package repository

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dever-package/front/service/siteconfig"
	"github.com/dever-package/front/service/upload/openurl"
	uploadprovider "github.com/dever-package/front/service/upload/provider"
)

type UploadFileURLs struct {
	URL       string
	Thumbnail string
	OpenURL   string
	Download  string
}

func BuildUploadFilePayload(file UploadFile) map[string]any {
	urls := ResolveUploadFileURLs(file)

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
		"url":         urls.URL,
		"download":    urls.Download,
		"thumbnail":   urls.Thumbnail,
		"open_url":    urls.OpenURL,
	}
}

func ResolveUploadFileURLs(file UploadFile) UploadFileURLs {
	downloadURL := buildUploadOpenURL(file.ID)
	providerURL, providerThumbnailURL, useSignedPublicOpen := resolveUploadFileURLs(file)
	publicURL, openTargetURL := openurl.ResolveAssetURLs(
		file.ID,
		providerURL,
		downloadURL,
		useSignedPublicOpen,
	)
	thumbnailURL := publicURL
	if uploadprovider.IsVideoFile(file.Kind, file.Mime, file.Ext) {
		thumbnailURL = providerThumbnailURL
	}
	return UploadFileURLs{
		URL:       publicURL,
		Thumbnail: thumbnailURL,
		OpenURL:   openTargetURL,
		Download:  downloadURL,
	}
}

func buildUploadOpenURL(fileID uint64) string {
	if fileID == 0 {
		return ""
	}
	query := url.Values{}
	query.Set("id", strconv.FormatUint(fileID, 10))
	return siteconfig.FrontRuntimeAPIURL("upload/open", query)
}

func resolveUploadFileURLs(file UploadFile) (string, string, bool) {
	driver, err := uploadprovider.Resolve(strings.TrimSpace(file.Storage.Type))
	if err != nil {
		return "", "", false
	}
	providerFile := uploadprovider.File{
		Path:        file.Path,
		ProviderKey: file.ProviderKey,
		Storage:     file.Storage,
	}
	publicURL := strings.TrimSpace(driver.ResolvePublicURL(providerFile))
	thumbnailURL := uploadprovider.ResolveThumbnailURL(driver, providerFile)
	return publicURL, thumbnailURL, uploadprovider.UsesSignedPublicOpen(driver)
}
