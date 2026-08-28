package douyin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dever-package/front/service/remoteurl"
	"github.com/dever-package/front/service/webcontent/provider"
)

const (
	pageTimeout     = 30 * time.Second
	maxPageBytes    = 8 * 1024 * 1024
	mobileUserAgent = "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/126.0 Mobile Safari/537.36"
)

var numericItemIDPattern = regexp.MustCompile(`^[0-9]{10,24}$`)

type Resolver struct {
	client *http.Client
}

func New() *Resolver {
	client := remoteurl.NewHTTPClient(remoteurl.ClientOptions{
		Timeout:      pageTimeout,
		MaxRedirects: 5,
		ProxyEnvVars: []string{
			"FRONT_WEBCONTENT_URL_PROXY",
			"FRONT_ARTICLE_IMPORT_URL_PROXY",
			"FRONT_UPLOAD_IMPORT_URL_PROXY",
		},
	})
	jar, err := cookiejar.New(nil)
	if err == nil {
		client.Jar = jar
	}
	return &Resolver{client: client}
}

func (*Resolver) Platform() string {
	return provider.PlatformDouyin
}

func (*Resolver) Match(parsedURL *url.URL) bool {
	if parsedURL == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsedURL.Hostname()))
	return host == "douyin.com" || strings.HasSuffix(host, ".douyin.com") ||
		host == "iesdouyin.com" || strings.HasSuffix(host, ".iesdouyin.com")
}

func (resolver *Resolver) Resolve(ctx context.Context, request provider.ResolveRequest) (provider.ResolvedContent, error) {
	if request.URL == nil || !resolver.Match(request.URL) {
		return provider.ResolvedContent{}, fmt.Errorf("抖音作品链接无效")
	}
	if resolver.client == nil {
		return provider.ResolvedContent{}, fmt.Errorf("抖音解析器未初始化")
	}

	itemID, err := resolver.resolveItemID(ctx, request.URL)
	if err != nil {
		return provider.ResolvedContent{}, err
	}
	canonicalURL := "https://www.douyin.com/video/" + itemID
	pageURLs := []string{
		"https://jingxuan.douyin.com/m/video/" + itemID,
		"https://www.iesdouyin.com/share/video/" + itemID + "/",
	}

	var lastErr error
	for _, pageURL := range pageURLs {
		sourceHTML, err := resolver.fetchPage(ctx, pageURL, request.Credential)
		if err != nil {
			lastErr = err
			continue
		}
		result, err := parsePage(sourceHTML, itemID, canonicalURL)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("未解析到公开视频地址")
	}
	return provider.ResolvedContent{}, fmt.Errorf("抖音作品解析失败，作品可能已失效或需要有效登录态: %w", lastErr)
}

func (resolver *Resolver) resolveItemID(ctx context.Context, sourceURL *url.URL) (string, error) {
	if itemID := itemIDFromURL(sourceURL); itemID != "" {
		return itemID, nil
	}

	req, err := resolver.newRequest(ctx, sourceURL.String(), "")
	if err != nil {
		return "", fmt.Errorf("创建抖音短链请求失败: %w", err)
	}
	resp, err := resolver.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("解析抖音短链失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.Request != nil {
		if itemID := itemIDFromURL(resp.Request.URL); itemID != "" {
			return itemID, nil
		}
	}
	body, readErr := readLimited(resp.Body, 1024*1024)
	if readErr == nil {
		if itemID := itemIDFromText(string(body)); itemID != "" {
			return itemID, nil
		}
	}
	return "", fmt.Errorf("抖音短链中未找到作品 ID")
}

func (resolver *Resolver) fetchPage(ctx context.Context, pageURL string, credential string) (string, error) {
	req, err := resolver.newRequest(ctx, pageURL, credential)
	if err != nil {
		return "", fmt.Errorf("创建抖音页面请求失败: %w", err)
	}
	resp, err := resolver.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("抓取抖音页面失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("抓取抖音页面失败: status=%d", resp.StatusCode)
	}
	body, err := readLimited(resp.Body, maxPageBytes)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (*Resolver) newRequest(ctx context.Context, rawURL string, credential string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mobileUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	if credential = strings.TrimSpace(credential); credential != "" {
		req.Header.Set("Cookie", credential)
	}
	return req, nil
}

func itemIDFromURL(parsedURL *url.URL) string {
	if parsedURL == nil {
		return ""
	}
	for _, key := range []string{"modal_id", "item_id", "item_ids"} {
		if itemID := normalizeItemID(parsedURL.Query().Get(key)); itemID != "" {
			return itemID
		}
	}
	segments := strings.FieldsFunc(parsedURL.EscapedPath(), func(current rune) bool {
		return current == '/'
	})
	for index := len(segments) - 1; index >= 0; index-- {
		segment, err := url.PathUnescape(segments[index])
		if err == nil {
			if itemID := normalizeItemID(segment); itemID != "" {
				return itemID
			}
		}
	}
	return ""
}

func itemIDFromText(source string) string {
	for _, marker := range []string{"/share/video/", "/video/", "/m/video/"} {
		offset := 0
		for {
			index := strings.Index(source[offset:], marker)
			if index < 0 {
				break
			}
			start := offset + index + len(marker)
			end := start
			for end < len(source) && source[end] >= '0' && source[end] <= '9' {
				end++
			}
			if itemID := normalizeItemID(source[start:end]); itemID != "" {
				return itemID
			}
			offset = start
		}
	}
	return ""
}

func normalizeItemID(value string) string {
	value = strings.TrimSpace(strings.Split(value, ",")[0])
	if numericItemIDPattern.MatchString(value) {
		return value
	}
	return ""
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取抖音页面失败: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("抖音页面内容过大")
	}
	return body, nil
}
