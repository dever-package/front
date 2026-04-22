package provider

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const localUploadPublicPrefix = "/upload"

func ResolveLocalObjectPath(objectKey string) string {
	normalizedKey := normalizeLocalObjectKey(objectKey)
	if normalizedKey == "" {
		return resolveUploadDataRoot()
	}
	return filepath.Join(resolveUploadDataRoot(), filepath.FromSlash(normalizedKey))
}

func ResolveLocalPublicURL(storageDomain, objectKey string) string {
	if url := JoinPublicURL(storageDomain, objectKey); strings.TrimSpace(url) != "" {
		return url
	}
	return LocalPublicPath(objectKey)
}

func LocalPublicPath(objectKey string) string {
	normalizedKey := normalizeLocalObjectKey(objectKey)
	if normalizedKey == "" {
		return ""
	}
	normalizedKey = strings.TrimPrefix(normalizedKey, "upload/")
	normalizedKey = strings.TrimSpace(normalizedKey)
	if normalizedKey == "" {
		return ""
	}
	return path.Join(localUploadPublicPrefix, normalizedKey)
}

func ResolveLocalPublicFilePath(publicPath string) (string, error) {
	rootDir := filepath.Join(resolveUploadDataRoot(), "upload")
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("解析上传目录失败: %w", err)
	}

	normalizedPath := path.Clean("/" + strings.TrimSpace(publicPath))
	relativePath := strings.TrimPrefix(normalizedPath, "/")
	if relativePath == "" || relativePath == "." {
		return "", fmt.Errorf("上传资源路径不能为空")
	}

	localPath := filepath.Join(rootAbs, filepath.FromSlash(relativePath))
	localAbs, err := filepath.Abs(localPath)
	if err != nil {
		return "", fmt.Errorf("解析上传资源路径失败: %w", err)
	}
	if localAbs != rootAbs && !strings.HasPrefix(localAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("上传资源路径无效")
	}
	return localAbs, nil
}

func normalizeLocalObjectKey(objectKey string) string {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(objectKey)))
	cleaned = strings.TrimLeft(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}
