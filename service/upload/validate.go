package upload

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

func validateUploadInit(rule resolvedUploadRule, input uploadInitInput) error {
	if rule.ID == 0 {
		return fmt.Errorf("上传规则不存在")
	}
	if rule.Status != 1 {
		return fmt.Errorf("上传规则已停用")
	}
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("文件名不能为空")
	}
	if input.Size <= 0 {
		return fmt.Errorf("文件大小无效")
	}
	if maxSize := uploadRuleMaxSizeBytes(rule); maxSize > 0 && input.Size > maxSize {
		return fmt.Errorf("文件大小超出限制")
	}
	if err := validateUploadAccept(rule.Accept, input.Name, input.Mime); err != nil {
		return err
	}
	if err := validateUploadActiveContent(rule.Accept, input.Name, input.Mime); err != nil {
		return err
	}
	if strings.EqualFold(rule.Transport, "direct") && strings.EqualFold(resolveUploadStorageProvider(rule.Storage), "local") {
		return fmt.Errorf("本地上传不支持前端直传")
	}
	if strings.EqualFold(rule.Transport, "direct") && normalizeUploadHash(input.Hash) == "" {
		return fmt.Errorf("当前上传规则要求提供文件标识")
	}
	return nil
}

func validateUploadStoredFile(rule resolvedUploadRule, fileName, mimeType string) error {
	if err := validateUploadAccept(rule.Accept, fileName, mimeType); err != nil {
		return err
	}
	return validateUploadActiveContent(rule.Accept, fileName, mimeType)
}

func validateUploadAccept(accept, fileName, mimeType string) error {
	accept = strings.TrimSpace(accept)
	if accept == "" || accept == "*" || accept == "*/*" {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	mimeType = normalizeUploadMimeType(mimeType)
	for _, token := range splitUploadAccept(accept) {
		switch {
		case token == "*" || token == "*/*":
			return nil
		case strings.HasPrefix(token, ".") && token == ext:
			return nil
		case strings.HasSuffix(token, "/*") && strings.HasPrefix(mimeType, strings.TrimSuffix(token, "*")):
			return nil
		case strings.Contains(token, "/") && token == mimeType:
			return nil
		}
	}
	return fmt.Errorf("文件类型不被允许")
}

func validateUploadActiveContent(accept, fileName, mimeType string) error {
	if !isActiveUploadContent(fileName, mimeType) {
		return nil
	}
	if acceptExplicitlyAllowsActiveContent(accept, fileName, mimeType) {
		return nil
	}
	return fmt.Errorf("文件类型不被允许")
}

func isActiveUploadContent(fileName, mimeType string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch ext {
	case ".svg", ".html", ".htm", ".xhtml", ".xml":
		return true
	}

	switch normalizeUploadMimeType(mimeType) {
	case "image/svg+xml", "text/html", "application/xhtml+xml", "application/xml", "text/xml":
		return true
	default:
		return false
	}
}

func acceptExplicitlyAllowsActiveContent(accept, fileName, mimeType string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	mimeType = normalizeUploadMimeType(mimeType)
	for _, token := range splitUploadAccept(accept) {
		if (ext != "" && token == ext) || (mimeType != "" && token == mimeType) {
			return true
		}
	}
	return false
}

func detectUploadFileMime(localPath, fileName, fallback string) (string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("读取上传文件失败: %w", err)
	}
	defer file.Close()

	header := make([]byte, 512)
	n, _ := file.Read(header)
	return detectUploadMimeFromHeader(header[:n], fileName, fallback), nil
}

func detectUploadMimeFromHeader(header []byte, fileName, fallback string) string {
	detected := normalizeUploadMimeType(http.DetectContentType(header))
	if detected != "" && detected != "application/octet-stream" {
		return detected
	}
	if fallback = normalizeUploadMimeType(fallback); fallback != "" {
		return fallback
	}
	return normalizeUploadMimeType(mime.TypeByExtension(filepath.Ext(fileName)))
}

func normalizeUploadMimeType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(value); err == nil {
		value = mediaType
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func splitUploadAccept(accept string) []string {
	return uploadrepo.SplitAccept(accept)
}

func mergeUploadAcceptTypes(items []resolvedUploadAcceptType) string {
	return uploadrepo.MergeAcceptTypes(items)
}

func collectUploadAcceptTypeIDs(items []resolvedUploadAcceptType) []uint64 {
	return uploadrepo.CollectAcceptTypeIDs(items)
}
