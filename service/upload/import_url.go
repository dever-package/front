package upload

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shemic/dever/server"

	"github.com/dever-package/front/service/upload/internal/transfer"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

const importURLTimeout = 10 * time.Minute

func ImportURLUpload(c *server.Context) error {
	var input uploadImportURLInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}
	release, err := acquireImportURLSlot()
	if err != nil {
		return c.Error(err)
	}
	defer release()

	fileRecord, err := importURLUploadWithProgress(c.Context(), input, nil)
	if err != nil {
		return c.Error(err)
	}
	logUploadFile(c, fileRecord.ID, input)
	return c.JSON(uploadrepo.BuildUploadFilePayload(fileRecord))
}

func downloadImportURLFile(
	ctx context.Context,
	input uploadImportURLInput,
	maxBytes int64,
	progress func(text string, progress int),
) (string, string, string, func(), error) {
	rawURL := strings.TrimSpace(input.URL)
	if rawURL == "" {
		return "", "", "", nil, fmt.Errorf("资源地址不能为空")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return "", "", "", nil, fmt.Errorf("资源地址无效")
	}
	if err := validateImportURL(parsed); err != nil {
		return "", "", "", nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, importURLTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("创建资源下载请求失败: %w", err)
	}
	resp, err := importURLHTTPClient.Do(req)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("下载资源失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", "", "", nil, fmt.Errorf("下载资源失败: status=%d", resp.StatusCode)
	}
	if maxBytes > 0 && resp.ContentLength > maxBytes {
		return "", "", "", nil, fmt.Errorf("文件大小超出限制")
	}
	notifyImportURLProgress(progress, "正在下载远程资源", 5)

	mimeType := normalizeImportURLMime(input.Mime, resp.Header.Get("Content-Type"))
	name := resolveImportURLName(input.Name, parsed, resp.Header.Get("Content-Disposition"), mimeType)
	file, err := os.CreateTemp("", "dever-import-url-*"+filepath.Ext(name))
	if err != nil {
		return "", "", "", nil, fmt.Errorf("创建导入临时文件失败: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}

	var reader io.Reader = resp.Body
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes+1)
	}
	reader = transfer.WrapReader(reader, resp.ContentLength, func(loaded int64, total int64) {
		if percent := transfer.Percent(loaded, total, 5, 45); percent >= 0 {
			notifyImportURLProgress(progress, "正在下载远程资源", percent)
		}
	})
	written, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		cleanup()
		return "", "", "", nil, fmt.Errorf("写入导入临时文件失败: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return "", "", "", nil, fmt.Errorf("关闭导入临时文件失败: %w", closeErr)
	}
	if maxBytes > 0 && written > maxBytes {
		cleanup()
		return "", "", "", nil, fmt.Errorf("文件大小超出限制")
	}

	return file.Name(), name, mimeType, cleanup, nil
}

func notifyImportURLProgress(progress func(text string, progress int), text string, percent int) {
	if progress == nil {
		return
	}
	progress(strings.TrimSpace(text), percent)
}

func normalizeImportURLMime(values ...string) string {
	for _, value := range values {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err == nil && strings.TrimSpace(mediaType) != "" {
			return strings.TrimSpace(mediaType)
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveImportURLName(rawName string, parsed *url.URL, disposition string, mimeType string) string {
	if name := strings.TrimSpace(rawName); name != "" {
		return ensureImportURLNameExt(filepath.Base(name), mimeType)
	}
	if _, params, err := mime.ParseMediaType(strings.TrimSpace(disposition)); err == nil {
		if name := strings.TrimSpace(params["filename"]); name != "" {
			return ensureImportURLNameExt(filepath.Base(name), mimeType)
		}
	}
	if parsed != nil {
		if name := strings.TrimSpace(filepath.Base(parsed.Path)); name != "" && name != "." && name != "/" {
			return ensureImportURLNameExt(name, mimeType)
		}
	}
	return ensureImportURLNameExt("generated-resource", mimeType)
}

func ensureImportURLNameExt(name string, mimeType string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "generated-resource"
	}
	if filepath.Ext(name) != "" {
		return name
	}
	extensions, err := mime.ExtensionsByType(strings.TrimSpace(mimeType))
	if err == nil && len(extensions) > 0 {
		return name + extensions[0]
	}
	return name
}
