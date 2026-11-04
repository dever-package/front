package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/go-sdk/v7/auth"
	qiniuclient "github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"

	frontmodel "github.com/dever-package/front/model"
)

type qiniuDriver struct{}

// 保留双栈回退，限制失联地址等待；不改系统 DNS 或其他供应方的 Transport。
var qiniuHTTPClient = &http.Client{Timeout: 10 * time.Minute, Transport: &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second, FallbackDelay: 100 * time.Millisecond}).DialContext,
	TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
	MaxIdleConns: 32, MaxIdleConnsPerHost: 8, IdleConnTimeout: 90 * time.Second,
	ForceAttemptHTTP2: true,
}}

const qiniuVideoThumbnailOperation = "vframe/jpg/offset/0/w/640"

func (driver qiniuDriver) Save(ctx context.Context, input SaveInput) error {
	_, err := driver.SaveWithResult(ctx, input)
	return err
}

func (qiniuDriver) SaveWithResult(ctx context.Context, input SaveInput) (SaveResult, error) {
	storageConfig := input.Rule.Storage
	if strings.TrimSpace(storageConfig.AccessKey) == "" || strings.TrimSpace(storageConfig.SecretKey) == "" || strings.TrimSpace(storageConfig.Bucket) == "" {
		return SaveResult{}, fmt.Errorf("七牛云配置不完整")
	}
	candidates := resolveSaveCandidates(input)
	if len(candidates) == 0 {
		return SaveResult{}, fmt.Errorf("七牛云上传缺少文件路径")
	}

	for _, candidate := range candidates {
		err := saveQiniuCandidate(ctx, input, candidate.ObjectKey)
		if isQiniuErrorCode(err, 614) && input.PathMode == frontmodel.UploadStoragePathModeHash {
			// 请求可能已在七牛提交、仅回包丢失；必须核对内容，不能覆盖其他文件。
			if verifyErr := verifyQiniuSavedContent(ctx, input, candidate.ObjectKey); verifyErr != nil {
				return SaveResult{}, verifyErr
			}
			err = nil
		}
		if err != nil {
			if isQiniuErrorCode(err, 614) {
				if input.PathMode == frontmodel.UploadStoragePathModeHash {
					return SaveResult{}, fmt.Errorf("七牛云内容哈希路径已被其他文件占用")
				}
				continue
			}
			return SaveResult{}, fmt.Errorf("上传到七牛云失败: %w", err)
		}
		return SaveResult{ProviderKey: candidate.ObjectKey, StoredName: candidate.Name}, nil
	}
	return SaveResult{}, fmt.Errorf("七牛云目标目录中没有可用文件名")
}

func saveQiniuCandidate(ctx context.Context, input SaveInput, objectKey string) error {
	storageConfig := input.Rule.Storage

	mac := auth.New(strings.TrimSpace(storageConfig.AccessKey), strings.TrimSpace(storageConfig.SecretKey))
	policy := storage.PutPolicy{
		Scope:        fmt.Sprintf("%s:%s", strings.TrimSpace(storageConfig.Bucket), objectKey),
		InsertOnly:   1,
		FsizeLimit:   input.Size,
		MimeLimit:    strings.TrimSpace(input.Rule.MimeLimit),
		ForceSaveKey: false,
	}
	upToken := policy.UploadToken(mac)

	qiniuConfig := storage.NewConfig()
	qiniuConfig.UseHTTPS = true
	if host := strings.TrimSpace(storageConfig.UploadHost); host != "" {
		qiniuConfig.UpHost = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	}

	ret := storage.PutRet{}
	uploader := storage.NewFormUploaderEx(qiniuConfig, &qiniuclient.Client{Client: qiniuHTTPClient})
	if input.Progress != nil {
		input.Progress(0, input.Size)
	}
	var uploadErr error
	for attempt := 0; attempt < 2; attempt++ {
		uploadErr = uploader.PutFile(ctx, &ret, upToken, objectKey, input.LocalPath, &storage.PutExtra{MimeType: input.Mime, TryTimes: 1})
		if uploadErr == nil || !retryableQiniuUploadError(ctx, uploadErr) {
			break
		}
		if attempt == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	if uploadErr != nil {
		return uploadErr
	}
	if input.Progress != nil {
		input.Progress(input.Size, input.Size)
	}
	return nil
}

func retryableQiniuUploadError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var network net.Error
	var provider *qiniuclient.ErrorInfo
	return errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || (errors.As(err, &provider) && (provider.Code == 429 || provider.Code >= 500))
}

func verifyQiniuSavedContent(ctx context.Context, input SaveInput, key string) error {
	if input.Hash == "" || strings.TrimSpace(input.Rule.Storage.Domain) == "" {
		return fmt.Errorf("七牛云已有文件，缺少内容校验信息")
	}
	verifyContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(verifyContext, http.MethodGet, JoinPublicURL(input.Rule.Storage.Domain, key), nil)
	if err != nil {
		return err
	}
	response, err := qiniuHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("核对七牛云已有文件失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("核对七牛云已有文件失败: HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(response.Body, input.Size+1))
	if err != nil {
		return err
	}
	if size != input.Size || hex.EncodeToString(hash.Sum(nil))[:32] != input.Hash {
		return fmt.Errorf("七牛云内容哈希路径已被其他文件占用")
	}
	return nil
}

func (qiniuDriver) InitDirect(ctx context.Context, rule Rule, session Session) (*DirectInitResult, error) {
	storageConfig := rule.Storage
	if strings.TrimSpace(storageConfig.AccessKey) == "" || strings.TrimSpace(storageConfig.SecretKey) == "" || strings.TrimSpace(storageConfig.Bucket) == "" {
		return nil, fmt.Errorf("七牛云配置不完整")
	}
	candidates := resolveSaveCandidates(SaveInput{
		ObjectKey:           session.ObjectKey,
		ObjectKeyCandidates: session.ObjectKeyCandidates,
		NameCandidates:      session.NameCandidates,
	})
	if len(candidates) == 0 {
		return nil, fmt.Errorf("七牛直传缺少文件标识")
	}

	mac := auth.New(strings.TrimSpace(storageConfig.AccessKey), strings.TrimSpace(storageConfig.SecretKey))
	candidate, err := resolveQiniuDirectCandidate(ctx, storageConfig, mac, session.PathMode, candidates)
	if err != nil {
		return nil, err
	}
	policy := storage.PutPolicy{
		Scope:      fmt.Sprintf("%s:%s", strings.TrimSpace(storageConfig.Bucket), candidate.ObjectKey),
		InsertOnly: 1,
		FsizeLimit: rule.MaxSizeBytes,
		MimeLimit:  strings.TrimSpace(rule.MimeLimit),
	}
	if storageConfig.TokenTTL > 0 {
		policy.Expires = uint64(storageConfig.TokenTTL)
	}
	upToken := policy.UploadToken(mac)

	uploadURL := strings.TrimSpace(storageConfig.UploadHost)
	if uploadURL == "" {
		uploadURL = "https://upload.qiniup.com"
	} else if !strings.HasPrefix(uploadURL, "http://") && !strings.HasPrefix(uploadURL, "https://") {
		uploadURL = "https://" + uploadURL
	}

	return &DirectInitResult{
		Method:      "post",
		UploadURL:   uploadURL,
		ProviderKey: candidate.ObjectKey,
		StoredName:  candidate.Name,
		Fields: map[string]string{
			"token": upToken,
			"key":   candidate.ObjectKey,
		},
	}, nil
}

func resolveQiniuDirectCandidate(
	ctx context.Context,
	storageConfig frontmodel.UploadStorage,
	mac *auth.Credentials,
	pathMode string,
	candidates []saveCandidate,
) (saveCandidate, error) {
	if pathMode != frontmodel.UploadStoragePathModeReadable {
		return candidates[0], nil
	}

	config := storage.NewConfig()
	config.UseHTTPS = true
	bucketManager := storage.NewBucketManager(mac, config)
	for _, candidate := range candidates {
		_, err := bucketManager.Stat(strings.TrimSpace(storageConfig.Bucket), candidate.ObjectKey)
		switch {
		case err == nil:
			continue
		case isQiniuErrorCode(err, 612):
			return candidate, nil
		default:
			return saveCandidate{}, fmt.Errorf("检查七牛云目标文件失败: %w", err)
		}
	}
	return saveCandidate{}, fmt.Errorf("七牛云目标目录中没有可用文件名")
}

func isQiniuErrorCode(err error, code int) bool {
	var qiniuErr *qiniuclient.ErrorInfo
	return errors.As(err, &qiniuErr) && qiniuErr.Code == code
}

func (qiniuDriver) ResolveOpen(_ context.Context, file File) (*OpenTarget, error) {
	domain := strings.TrimSpace(file.Storage.Domain)
	if domain == "" {
		return nil, fmt.Errorf("七牛云访问域名未配置")
	}
	return &OpenTarget{
		Redirect: JoinPublicURL(domain, ResolveFileProviderKey(file)),
	}, nil
}

func (qiniuDriver) ResolvePublicURL(file File) string {
	return JoinPublicURL(file.Storage.Domain, ResolveFileProviderKey(file))
}

func (driver qiniuDriver) ResolveThumbnailURL(file File) string {
	return appendQiniuURLImageOperation(
		driver.ResolvePublicURL(file),
		qiniuVideoThumbnailOperation,
	)
}

func appendQiniuURLImageOperation(rawURL, operation string) string {
	value := strings.TrimSpace(rawURL)
	operation = strings.TrimSpace(operation)
	if value == "" || operation == "" || strings.Contains(value, "vframe/") {
		return value
	}

	fragment := ""
	if index := strings.Index(value, "#"); index >= 0 {
		fragment = value[index:]
		value = value[:index]
	}
	separator := "?"
	if strings.Contains(value, "?") {
		separator = "&"
	}
	return value + separator + operation + fragment
}
