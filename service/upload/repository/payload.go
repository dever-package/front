package repository

import (
	"net/url"
	"strconv"
	"strings"

	"my/package/front/service/siteconfig"
	uploadprovider "my/package/front/service/upload/provider"
)

func BuildUploadFilePayload(file UploadFile) map[string]any {
	openURL := buildUploadOpenURL(file.ID)
	publicURL := resolveUploadFilePublicURL(file)
	if strings.TrimSpace(publicURL) == "" {
		publicURL = openURL
	}

	return map[string]any{
		"id":          file.ID,
		"rule_id":     file.RuleID,
		"kind":        file.Kind,
		"biz_id":      file.BizID,
		"biz_key":     file.BizKey,
		"biz_name":    file.BizName,
		"category_id": file.CategoryID,
		"name":        file.Name,
		"ext":         file.Ext,
		"mime":        file.Mime,
		"size":        file.Size,
		"hash":        file.Hash,
		"path":        file.Path,
		"created_at":  file.CreatedAt,
		"url":         publicURL,
		"download":    openURL,
		"thumbnail":   publicURL,
		"open_url":    openURL,
	}
}

func buildUploadOpenURL(fileID uint64) string {
	query := url.Values{}
	query.Set("id", strconv.FormatUint(fileID, 10))
	return siteconfig.FrontRuntimeAPIURL("upload/open", query)
}

func resolveUploadFilePublicURL(file UploadFile) string {
	driver, err := uploadprovider.Resolve(strings.TrimSpace(file.Storage.Type))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(driver.ResolvePublicURL(uploadprovider.File{
		Path:    file.Path,
		Storage: file.Storage,
	}))
}
