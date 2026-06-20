package importer

import (
	"fmt"
	"strings"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

func ensureImportFileAccess(pagePath string, config importConfig, fileRecord uploadrepo.UploadFile, fileHash string) error {
	expectedBizKey := buildImportFileBizKey(pagePath, config.Key)
	if expectedBizKey == "" {
		return fmt.Errorf("导入文件访问上下文无效")
	}
	if uploadrepo.NormalizeBizKey(fileRecord.BizKey) != expectedBizKey {
		return fmt.Errorf("导入文件与当前导入配置不匹配")
	}

	expectedHash := uploadrepo.NormalizeHash(fileHash)
	if expectedHash == "" || uploadrepo.NormalizeHash(fileRecord.Hash) != expectedHash {
		return fmt.Errorf("导入文件校验失败，请重新上传")
	}
	return nil
}

func buildImportFileBizKey(pagePath, importKey string) string {
	pageSegment := normalizeImportFileBizSegment(frontpagepath.NormalizePath(pagePath))
	keySegment := normalizeImportFileBizSegment(importKey)
	if pageSegment == "" || keySegment == "" {
		return ""
	}
	return uploadrepo.NormalizeBizKey(pageSegment + ".import." + keySegment)
}

func normalizeImportFileBizSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(value, "/.")
	value = strings.ReplaceAll(value, "/", ".")
	for strings.Contains(value, "..") {
		value = strings.ReplaceAll(value, "..", ".")
	}
	return strings.Trim(value, ".")
}
