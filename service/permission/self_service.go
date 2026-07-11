package permission

import (
	"context"
	"strings"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	authctx "github.com/dever-package/front/service/internal/authctx"
	"github.com/dever-package/front/service/siteconfig"
)

var selfServicePageNames = []string{
	"account/profile",
}

func canAccessSelfServicePage(ctx context.Context, pathValue string) bool {
	return authctx.OptionalUID(ctx) > 0 && isSelfServicePagePath(ctx, pathValue)
}

func isSelfServicePagePath(ctx context.Context, pathValue string) bool {
	site, ok := siteconfig.FromContext(ctx)
	if !ok {
		site = siteconfig.Site{API: siteconfig.DefaultAPI}
	}
	return isSelfServicePagePathForSite(site, pathValue)
}

func isSelfServicePagePathForSite(site siteconfig.Site, pathValue string) bool {
	pathValue = frontpagepath.NormalizePath(pathValue)
	if pathValue == "" {
		return false
	}

	for _, pageName := range selfServicePageNames {
		for _, candidate := range selfServicePagePathCandidates(site, pageName) {
			if matchesSelfServicePagePath(pathValue, candidate) {
				return true
			}
		}
	}
	return false
}

func filterSelfServiceAuthRowsForSite(site siteconfig.Site, rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}

	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if isSelfServiceAuthRowForSite(site, row) {
			continue
		}
		result = append(result, row)
	}
	return result
}

func isSelfServiceAuthRowForSite(site siteconfig.Site, row map[string]any) bool {
	if len(row) == 0 {
		return false
	}
	return isSelfServicePagePathForSite(site, authRowKey(row)) ||
		isSelfServicePagePathForSite(site, authRowPath(row))
}

func selfServicePagePathCandidates(site siteconfig.Site, pageName string) []string {
	candidates := []string{pageName}
	if pathValue := site.SystemPagePath(pageName); pathValue != "" {
		candidates = append(candidates, pathValue)
	}

	defaultSite := siteconfig.Site{API: siteconfig.DefaultAPI}
	if pathValue := defaultSite.SystemPagePath(pageName); pathValue != "" {
		candidates = append(candidates, pathValue)
	}
	return candidates
}

func matchesSelfServicePagePath(pathValue string, selfServicePath string) bool {
	pathValue = frontpagepath.NormalizePath(pathValue)
	selfServicePath = frontpagepath.NormalizePath(selfServicePath)
	if pathValue == "" || selfServicePath == "" {
		return false
	}
	return pathValue == selfServicePath || strings.HasPrefix(pathValue, selfServicePath+"/")
}
