package embedpage

import (
	"strings"

	"github.com/shemic/dever/util"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	pagecontent "github.com/dever-package/front/service/internal/pagecontent"
	"github.com/dever-package/front/service/siteconfig"
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

	return hasParentPath(ParentsForPage(pageName), parentPath, childPath, map[string]struct{}{})
}

func hasParentPath(graph map[string]map[string]struct{}, parentPath, childPath string, visited map[string]struct{}) bool {
	if _, seen := visited[childPath]; seen {
		return false
	}
	visited[childPath] = struct{}{}

	parents := graph[childPath]
	if len(parents) == 0 {
		return false
	}
	if _, ok := parents[parentPath]; ok {
		return true
	}
	for currentParent := range parents {
		if hasParentPath(graph, parentPath, currentParent, visited) {
			return true
		}
	}
	return false
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

	parentIDs := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		parentID := util.ToUint64(row["parent_id"])
		if parentID > 0 {
			parentIDs[parentID] = struct{}{}
		}
	}

	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if _, hidden := embeddedParents[normalizePath(util.ToStringTrimmed(row["path"]))]; hidden && shouldHideEmbeddedRow(row, parentIDs) {
			continue
		}
		result = append(result, row)
	}
	return result
}

func shouldHideEmbeddedRow(row map[string]any, parentIDs map[uint64]struct{}) bool {
	if util.ToIntDefault(row["type"], 0) == 2 {
		return false
	}
	id := util.ToUint64(row["id"])
	if id == 0 {
		return true
	}
	_, hasChild := parentIDs[id]
	return !hasChild
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
	return pagecontent.WalkComponentPages(pageName, func(page pagecontent.ComponentPage) error {
		collectParents(result, page.Path, page.Content)
		return nil
	})
}

func collectParents(result map[string]map[string]struct{}, parentPath string, content []byte) {
	parentPath = normalizePath(parentPath)
	if parentPath == "" {
		return
	}

	var payload struct {
		Layout map[string]any              `json:"layout"`
		Nodes  map[string][]map[string]any `json:"nodes"`
	}
	if err := util.UnmarshalJSONC(content, &payload); err != nil {
		return
	}

	collectLayout(result, parentPath, payload.Layout)
	for _, items := range payload.Nodes {
		for _, item := range items {
			collectItem(result, parentPath, item)
		}
	}
}

func collectLayout(result map[string]map[string]struct{}, parentPath string, layout map[string]any) {
	if len(layout) == 0 {
		return
	}

	if childPath := normalizePath(util.ToStringTrimmed(layout["path"])); childPath != "" {
		addParent(result, parentPath, childPath)
	}

	children, _ := layout["children"].(map[string]any)
	for _, child := range children {
		childLayout, _ := child.(map[string]any)
		collectLayout(result, parentPath, childLayout)
	}
}

func collectItem(result map[string]map[string]struct{}, parentPath string, item map[string]any) {
	if childPath := embeddedRoute(item); childPath != "" {
		addParent(result, parentPath, childPath)
	}

	if children, ok := item["items"].([]any); ok {
		for _, child := range children {
			if childItem, ok := child.(map[string]any); ok {
				collectItem(result, parentPath, childItem)
			}
		}
	}
}

func addParent(result map[string]map[string]struct{}, parentPath, childPath string) {
	parentPath = normalizePath(parentPath)
	childPath = normalizePath(childPath)
	if parentPath == "" || childPath == "" || parentPath == childPath {
		return
	}
	if result[childPath] == nil {
		result[childPath] = map[string]struct{}{}
	}
	result[childPath][parentPath] = struct{}{}
}

func embeddedRoute(item map[string]any) string {
	meta, _ := item["meta"].(map[string]any)
	route := normalizePath(util.ToStringTrimmed(meta["pageRoute"]))
	if route == "" {
		return ""
	}

	if strings.TrimSpace(util.ToString(item["mode"])) == "form" {
		return route
	}

	switch strings.TrimSpace(util.ToString(item["type"])) {
	case "feedback-modal", "feedback-drawer":
		return route
	default:
		return ""
	}
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
