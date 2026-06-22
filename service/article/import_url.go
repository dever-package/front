package article

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shemic/dever/server"
	"golang.org/x/net/html/charset"

	"github.com/dever-package/front/service/remoteurl"
)

const (
	articleImportTimeout  = 30 * time.Second
	articleImportMaxBytes = 8 * 1024 * 1024
	defaultMaxAssets      = 80
	maxArticleAssets      = 120
)

var articleHTTPClient = remoteurl.NewHTTPClient(remoteurl.ClientOptions{
	Timeout:      articleImportTimeout,
	MaxRedirects: 5,
	ProxyEnvVars: []string{
		"FRONT_ARTICLE_IMPORT_URL_PROXY",
		"FRONT_UPLOAD_IMPORT_URL_PROXY",
	},
})

func ImportURL(c *server.Context) error {
	var input ImportURLInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}

	article, err := ImportURLContent(c.Context(), input)
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(article)
}

func ImportURLContent(ctx context.Context, input ImportURLInput) (ImportedArticle, error) {
	page, err := fetchArticlePage(ctx, input.URL)
	if err != nil {
		return ImportedArticle{}, err
	}
	article, err := parseArticlePage(page, normalizeMaxAssets(input.MaxImages))
	if err != nil {
		return ImportedArticle{}, err
	}
	if strings.TrimSpace(article.HTML) == "" {
		return ImportedArticle{}, fmt.Errorf("未解析到文章正文")
	}
	return article, nil
}

func fetchArticlePage(ctx context.Context, rawURL string) (fetchedArticlePage, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fetchedArticlePage{}, fmt.Errorf("文章链接不能为空")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return fetchedArticlePage{}, fmt.Errorf("文章链接无效")
	}
	if err := remoteurl.Validate(parsed); err != nil {
		return fetchedArticlePage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fetchedArticlePage{}, fmt.Errorf("创建文章抓取请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := articleHTTPClient.Do(req)
	if err != nil {
		return fetchedArticlePage{}, fmt.Errorf("抓取文章失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fetchedArticlePage{}, fmt.Errorf("抓取文章失败: status=%d", resp.StatusCode)
	}

	raw, err := readLimited(resp.Body, articleImportMaxBytes, "文章内容过大")
	if err != nil {
		return fetchedArticlePage{}, err
	}
	reader, err := charset.NewReader(bytes.NewReader(raw), resp.Header.Get("Content-Type"))
	if err != nil {
		return fetchedArticlePage{}, fmt.Errorf("识别文章编码失败: %w", err)
	}
	decoded, err := readLimited(reader, articleImportMaxBytes, "文章内容过大")
	if err != nil {
		return fetchedArticlePage{}, err
	}

	return fetchedArticlePage{
		URL:        resp.Request.URL.String(),
		SourceHTML: string(decoded),
	}, nil
}

func readLimited(reader io.Reader, maxBytes int64, tooLargeMessage string) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, errors.New(tooLargeMessage)
	}
	return raw, nil
}

func normalizeMaxAssets(value int) int {
	if value <= 0 {
		return defaultMaxAssets
	}
	if value > maxArticleAssets {
		return maxArticleAssets
	}
	return value
}
