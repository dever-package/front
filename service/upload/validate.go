package upload

import (
	"fmt"
	"path/filepath"
	"strings"

	uploadrepo "my/package/front/service/upload/repository"
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
	if strings.EqualFold(rule.Transport, "direct") && strings.EqualFold(resolveUploadStorageProvider(rule.Storage), "local") {
		return fmt.Errorf("本地上传不支持前端直传")
	}
	if strings.EqualFold(rule.Transport, "direct") && normalizeUploadHash(input.Hash) == "" {
		return fmt.Errorf("当前上传规则要求提供文件标识")
	}
	return nil
}

func validateUploadAccept(accept, fileName, mimeType string) error {
	accept = strings.TrimSpace(accept)
	if accept == "" || accept == "*" || accept == "*/*" {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
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

func splitUploadAccept(accept string) []string {
	return uploadrepo.SplitAccept(accept)
}

func mergeUploadAcceptTypes(items []resolvedUploadAcceptType) string {
	return uploadrepo.MergeAcceptTypes(items)
}

func collectUploadAcceptTypeIDs(items []resolvedUploadAcceptType) []uint64 {
	return uploadrepo.CollectAcceptTypeIDs(items)
}
