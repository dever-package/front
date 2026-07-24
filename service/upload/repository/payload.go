package repository

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dever-package/front/service/siteconfig"
	"github.com/dever-package/front/service/upload/openurl"
	uploadprovider "github.com/dever-package/front/service/upload/provider"
)

func BuildUploadFilePayload(file UploadFile) map[string]any {
	openURL := buildUploadOpenURL(file.ID)
	providerURL, useSignedPublicOpen := resolveUploadFilePublicURL(file)
	publicURL, openTargetURL := openurl.ResolveAssetURLs(
		file.ID,
		providerURL,
		openURL,
		useSignedPublicOpen,
	)

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
		"open_url":    openTargetURL,
	}
}

func buildUploadOpenURL(fileID uint64) string {
	query := url.Values{}
	query.Set("id", strconv.FormatUint(fileID, 10))
	return siteconfig.FrontRuntimeAPIURL("upload/open", query)
}

func resolveUploadFilePublicURL(file UploadFile) (string, bool) {
	driver, err := uploadprovider.Resolve(strings.TrimSpace(file.Storage.Type))
	if err != nil {
		return "", false
	}
	publicURL := strings.TrimSpace(driver.ResolvePublicURL(uploadprovider.File{
		Path:    file.Path,
		Storage: file.Storage,
	}))
	return publicURL, uploadprovider.UsesSignedPublicOpen(driver)
}
