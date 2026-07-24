package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	frontmodel "github.com/dever-package/front/model"
)

const (
	feishuAPIBase             = "https://open.feishu.cn/open-apis"
	feishuTokenRequestTimeout = 15 * time.Second
	feishuMetadataTimeout     = 30 * time.Second
	feishuResponseMaxBytes    = int64(1 << 20)
	feishuDriveRequestGap     = 220 * time.Millisecond
	feishuRetryAttempts       = 3
)

var (
	feishuHTTPClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("飞书云盘下载重定向次数过多")
			}
			return nil
		},
	}
	feishuAppStates    sync.Map
	feishuFolderStates sync.Map
)

type feishuClient struct {
	config      feishuConfig
	appState    *feishuAppState
	folderState *feishuFolderState
}

type feishuAppState struct {
	tokenMu      sync.Mutex
	token        string
	tokenExpires time.Time
	uploadMu     sync.Mutex
	rateMu       sync.Mutex
	nextRequest  time.Time
}

type feishuFolderState struct {
	folderMu      sync.Mutex
	foldersLoaded bool
	folderTokens  map[string]string
}

type feishuBaseResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (response *feishuBaseResponse) responseStatus() (int, string) {
	if response == nil {
		return 0, ""
	}
	return response.Code, response.Msg
}

type feishuResponsePayload interface {
	responseStatus() (int, string)
}

type feishuTenantTokenResponse struct {
	feishuBaseResponse
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int64  `json:"expire"`
}

type feishuAPIError struct {
	Operation  string
	HTTPStatus int
	Code       int
	Message    string
	Header     http.Header
}

func (err *feishuAPIError) Error() string {
	message := strings.TrimSpace(err.Message)
	if message == "" {
		message = "未知错误"
	}
	if err.Code != 0 {
		return fmt.Sprintf("飞书云盘%s失败（%d）：%s", err.Operation, err.Code, message)
	}
	if err.HTTPStatus != 0 {
		return fmt.Sprintf("飞书云盘%s失败（HTTP %d）：%s", err.Operation, err.HTTPStatus, message)
	}
	return fmt.Sprintf("飞书云盘%s失败：%s", err.Operation, message)
}

func newFeishuClient(storage frontmodel.UploadStorage) (*feishuClient, error) {
	config, err := resolveFeishuConfig(storage)
	if err != nil {
		return nil, err
	}
	appFingerprint := feishuAppFingerprint(config)
	appValue, _ := feishuAppStates.LoadOrStore(appFingerprint, &feishuAppState{})
	folderValue, _ := feishuFolderStates.LoadOrStore(feishuFolderFingerprint(appFingerprint, config), &feishuFolderState{
		folderTokens: make(map[string]string, 256),
	})
	return &feishuClient{
		config:      config,
		appState:    appValue.(*feishuAppState),
		folderState: folderValue.(*feishuFolderState),
	}, nil
}

func feishuAppFingerprint(config feishuConfig) string {
	raw := config.AppID + "\x00" + config.AppSecret
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func feishuFolderFingerprint(appFingerprint string, config feishuConfig) string {
	raw := appFingerprint + "\x00" + config.RootFolderToken
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (client *feishuClient) tenantAccessToken(ctx context.Context) (string, error) {
	client.appState.tokenMu.Lock()
	defer client.appState.tokenMu.Unlock()

	if token := strings.TrimSpace(client.appState.token); token != "" && time.Now().Add(5*time.Minute).Before(client.appState.tokenExpires) {
		return token, nil
	}

	var response feishuTenantTokenResponse
	err := withFeishuRetry(ctx, func() error {
		requestCtx, cancel := context.WithTimeout(nonNilContext(ctx), feishuTokenRequestTimeout)
		defer cancel()
		if err := doFeishuJSONRequest(requestCtx, http.MethodPost, feishuAPIBase+"/auth/v3/tenant_access_token/internal", "", map[string]any{
			"app_id":     client.config.AppID,
			"app_secret": client.config.AppSecret,
		}, &response, "认证"); err != nil {
			return err
		}
		return feishuResponseError("认证", &response)
	})
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(response.TenantAccessToken)
	if token == "" || response.Expire <= 0 {
		return "", fmt.Errorf("飞书云盘认证失败：未返回有效访问令牌")
	}
	client.appState.token = token
	client.appState.tokenExpires = time.Now().Add(time.Duration(response.Expire) * time.Second)
	return token, nil
}

func (client *feishuClient) doDriveJSON(ctx context.Context, method, path, accessToken string, body any, response feishuResponsePayload, operation string) error {
	execute := func() error {
		if err := client.waitDriveSlot(ctx); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(nonNilContext(ctx), feishuMetadataTimeout)
		defer cancel()
		if err := doFeishuJSONRequest(requestCtx, method, feishuAPIBase+path, accessToken, body, response, operation); err != nil {
			return err
		}
		return feishuResponseError(operation, response)
	}
	if method == http.MethodGet || method == http.MethodHead {
		return withFeishuRetry(ctx, execute)
	}
	return execute()
}

func (client *feishuClient) waitDriveSlot(ctx context.Context) error {
	client.appState.rateMu.Lock()
	now := time.Now()
	requestAt := now
	if client.appState.nextRequest.After(requestAt) {
		requestAt = client.appState.nextRequest
	}
	client.appState.nextRequest = requestAt.Add(feishuDriveRequestGap)
	client.appState.rateMu.Unlock()

	wait := time.Until(requestAt)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-nonNilContext(ctx).Done():
		return nonNilContext(ctx).Err()
	case <-timer.C:
		return nil
	}
}

func doFeishuJSONRequest(ctx context.Context, method, endpoint, accessToken string, body any, response any, operation string) error {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("构造飞书云盘%s请求失败", operation)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("构造飞书云盘%s请求失败", operation)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if accessToken = strings.TrimSpace(accessToken); accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return executeFeishuJSONRequest(request, response, operation)
}

func executeFeishuJSONRequest(request *http.Request, response any, operation string) error {
	result, err := feishuHTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("飞书云盘%s请求失败：%w", operation, err)
	}
	defer result.Body.Close()
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		return decodeFeishuHTTPError(result, operation)
	}
	raw, err := io.ReadAll(io.LimitReader(result.Body, feishuResponseMaxBytes+1))
	if err != nil {
		return fmt.Errorf("读取飞书云盘%s响应失败", operation)
	}
	if int64(len(raw)) > feishuResponseMaxBytes {
		return fmt.Errorf("飞书云盘%s响应内容过大", operation)
	}
	if err = json.Unmarshal(raw, response); err != nil {
		return fmt.Errorf("解析飞书云盘%s响应失败", operation)
	}
	return nil
}

func decodeFeishuHTTPError(response *http.Response, operation string) error {
	raw, _ := io.ReadAll(io.LimitReader(response.Body, feishuResponseMaxBytes))
	payload := feishuBaseResponse{}
	_ = json.Unmarshal(raw, &payload)
	message := normalizeFeishuMessage(payload.Msg)
	if message == "未知错误" && response.Status != "" {
		message = response.Status
	}
	return &feishuAPIError{
		Operation:  operation,
		HTTPStatus: response.StatusCode,
		Code:       payload.Code,
		Message:    message,
		Header:     cloneFeishuErrorHeader(response.Header),
	}
}

func cloneFeishuErrorHeader(source http.Header) http.Header {
	result := make(http.Header)
	for _, name := range []string{"Content-Range", "Retry-After"} {
		if value := strings.TrimSpace(source.Get(name)); value != "" {
			result.Set(name, value)
		}
	}
	return result
}

func feishuResponseError(operation string, response feishuResponsePayload) error {
	code, message := response.responseStatus()
	if code == 0 {
		return nil
	}
	return &feishuAPIError{
		Operation: operation,
		Code:      code,
		Message:   normalizeFeishuMessage(message),
	}
}

func withFeishuRetry(ctx context.Context, operation func() error) error {
	delays := []time.Duration{250 * time.Millisecond, 600 * time.Millisecond}
	var lastErr error
	for attempt := 0; attempt < feishuRetryAttempts; attempt++ {
		if err := nonNilContext(ctx).Err(); err != nil {
			return err
		}
		lastErr = operation()
		if lastErr == nil || !isRetryableFeishuError(lastErr) || attempt == feishuRetryAttempts-1 {
			return lastErr
		}
		timer := time.NewTimer(delays[attempt])
		select {
		case <-nonNilContext(ctx).Done():
			timer.Stop()
			return nonNilContext(ctx).Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func isRetryableFeishuError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *feishuAPIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus == http.StatusTooManyRequests || apiErr.HTTPStatus >= http.StatusInternalServerError || apiErr.Code == 1061001 || apiErr.Code == 1061006 || apiErr.Code == 1061045
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func normalizeFeishuMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "未知错误"
	}
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:160]) + "..."
	}
	return value
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
