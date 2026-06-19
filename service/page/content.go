package page

import (
	"context"

	pagecontent "github.com/dever-package/front/service/internal/pagecontent"
)

type ContentSignature = pagecontent.ContentSignature
type ComponentPage = pagecontent.ComponentPage

func Signature(content []byte) ContentSignature {
	return pagecontent.Signature(content)
}

func ReadContent(pathValue string) ([]byte, error) {
	return pagecontent.ReadContent(pathValue)
}

func ReadContentForContext(ctx context.Context, pathValue string) ([]byte, error) {
	return pagecontent.ReadContentForContext(ctx, pathValue)
}

func ReadContentForSite(siteKey string, pathValue string) ([]byte, error) {
	return pagecontent.ReadContentForSite(siteKey, pathValue)
}

func ReadContentForPage(pageName string, pathValue string) ([]byte, error) {
	return pagecontent.ReadContentForPage(pageName, pathValue)
}

func WalkComponentPages(pageName string, visit func(ComponentPage) error) error {
	return pagecontent.WalkComponentPages(pageName, visit)
}

func ListComponentPages(pageName string) ([]ComponentPage, error) {
	return pagecontent.ListComponentPages(pageName)
}

func ClearContentCache() {
	pagecontent.ClearContentCache()
}
