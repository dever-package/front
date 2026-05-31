package embedpage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/util"
	frontroot "my/package/front"
	frontpagepath "my/package/front/internal/pagepath"
	"my/package/front/service/siteconfig"
)

var parentCache util.ConcurrentMap[string, map[string]map[string]struct{}]

func ClearCache() {
	parentCache = util.ConcurrentMap[string, map[string]map[string]struct{}]{}
}

func Paths() map[string]struct{} {
	return PathsForPage(siteconfig.DefaultPage)
}

func PathsForSite(siteKey string) map[string]struct{} {
	return PathsForPage(pageForSite(siteKey))
}

func PathsForPage(pageName string) map[string]struct{} {
	parents := ParentsForPage(pageName)
	result := make(map[string]struct{}, len(parents))
	for childPath := range parents {
		result[childPath] = struct{}{}
	}
	return result
}

func HasPath(path string) bool {
	return HasPathForPage(siteconfig.DefaultPage, path)
}

func HasPathForSite(siteKey string, path string) bool {
	return HasPathForPage(pageForSite(siteKey), path)
}

func HasPathForPage(pageName string, path string) bool {
	path = normalizePath(path)
	if path == "" {
		return false
	}

	_, ok := ParentsForPage(pageName)[path]
	return ok
}

func IsChild(parentPath, childPath string) bool {
	return IsChildForPage(siteconfig.DefaultPage, parentPath, childPath)
}

func IsChildForSite(siteKey string, parentPath, childPath string) bool {
	return IsChildForPage(pageForSite(siteKey), parentPath, childPath)
}

func IsChildForPage(pageName string, parentPath, childPath string) bool {
	parentPath = normalizePath(parentPath)
	childPath = normalizePath(childPath)
	if parentPath == "" || childPath == "" {
		return false
	}

	parents := ParentsForPage(pageName)[childPath]
	_, ok := parents[parentPath]
	return ok
}

func FilterRows(rows []map[string]any) []map[string]any {
	return FilterRowsForPage(siteconfig.DefaultPage, rows)
}

func FilterRowsForSite(siteKey string, rows []map[string]any) []map[string]any {
	return FilterRowsForPage(pageForSite(siteKey), rows)
}

func FilterRowsForPage(pageName string, rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}

	embeddedParents := ParentsForPage(pageName)
	if len(embeddedParents) == 0 {
		return rows
	}

	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if _, hidden := embeddedParents[normalizePath(util.ToStringTrimmed(row["path"]))]; hidden {
			continue
		}
		result = append(result, row)
	}
	return result
}

func Parents() map[string]map[string]struct{} {
	return ParentsForPage(siteconfig.DefaultPage)
}

func ParentsForSite(siteKey string) map[string]map[string]struct{} {
	return ParentsForPage(pageForSite(siteKey))
}

func ParentsForPage(pageName string) map[string]map[string]struct{} {
	pageName = normalizePageName(pageName)
	if cached, ok := parentCache.Load(pageName); ok {
		return cached
	}

	result := map[string]map[string]struct{}{}
	_ = walkParents(result, pageName)
	parentCache.Store(pageName, result)
	return result
}

func walkParents(result map[string]map[string]struct{}, pageName string) error {
	pageNames := siteconfig.LoadPageNames()
	for _, root := range []string{"module", "package"} {
		if err := walkDiskParents(root, pageName, pageNames, result); err != nil {
			return err
		}
	}

	return fs.WalkDir(frontroot.PageFS, "page", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() || !isPageFileName(entry.Name()) {
			return nil
		}

		content, err := frontroot.PageFS.ReadFile(path)
		if err != nil {
			return err
		}

		relativePath := strings.TrimPrefix(filepath.ToSlash(path), "page/")
		relativeParts := frontpagepath.RelativePartsForPage(relativePath, pageName, pageNames)
		if len(relativeParts) == 0 {
			return nil
		}
		routePath := trimPageFileExt(filepath.ToSlash(filepath.Join(append([]string{"front"}, relativeParts...)...)))
		collectParents(result, routePath, content)
		return nil
	})
}

func walkDiskParents(root string, pageName string, pageNames map[string]struct{}, result map[string]map[string]struct{}) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() || !isPageFileName(entry.Name()) {
			return nil
		}
		if frontpagepath.DiskPageBelongsToOtherPage(root, path, pageName, pageNames) {
			return nil
		}

		moduleName, routePath, ok := frontpagepath.DiskPageRouteForPage(root, path, pageName)
		if !ok || moduleName == "front" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		collectParents(result, routePath, content)
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func collectParents(result map[string]map[string]struct{}, parentPath string, content []byte) {
	parentPath = normalizePath(parentPath)
	if parentPath == "" {
		return
	}

	var payload struct {
		Nodes map[string][]map[string]any `json:"nodes"`
	}
	if err := util.UnmarshalJSONC(content, &payload); err != nil {
		return
	}

	for _, items := range payload.Nodes {
		for _, item := range items {
			collectItem(result, parentPath, item)
		}
	}
}

func collectItem(result map[string]map[string]struct{}, parentPath string, item map[string]any) {
	if childPath := embeddedRoute(item); childPath != "" {
		if result[childPath] == nil {
			result[childPath] = map[string]struct{}{}
		}
		result[childPath][parentPath] = struct{}{}
	}

	if children, ok := item["items"].([]any); ok {
		for _, child := range children {
			if childItem, ok := child.(map[string]any); ok {
				collectItem(result, parentPath, childItem)
			}
		}
	}
}

func embeddedRoute(item map[string]any) string {
	if strings.TrimSpace(util.ToString(item["mode"])) != "form" {
		return ""
	}

	meta, _ := item["meta"].(map[string]any)
	return normalizePath(util.ToStringTrimmed(meta["pageRoute"]))
}

func isPageFileName(name string) bool {
	return frontpagepath.IsPageFileName(name)
}

func trimPageFileExt(path string) string {
	return frontpagepath.TrimPageFileExt(path)
}

func normalizePath(path string) string {
	return frontpagepath.NormalizePath(path)
}

func normalizePageName(pageName string) string {
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	if pageName == "" {
		return siteconfig.DefaultPage
	}
	return pageName
}

func pageForSite(siteKey string) string {
	cfg, err := siteconfig.Load(nil)
	if err != nil {
		return siteKey
	}
	site, ok := cfg.FindBySiteKey(siteKey)
	if !ok {
		return siteKey
	}
	return site.Page
}
