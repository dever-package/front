package permission

import (
	"sort"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"
)

type menuNode struct {
	ID       uint64
	Key      string
	Name     string
	Icon     string
	Path     string
	ParentID uint64
	Sort     int
	Type     int
}

func GetMainInfo(c *server.Context) error {
	snapshot, err := loadAccessSnapshot(c.Context())
	if err != nil {
		return c.Error(err)
	}

	menu := buildMenu(snapshot)
	configMeta, _ := loadConfigMeta()
	entry := resolveMainEntry(configMeta.Entry, menu)

	return c.JSON(map[string]any{
		"menu":  menu,
		"entry": entry,
	})
}

func SyncMainInfo(c *server.Context) error {
	if err := ForceBootstrap(c.Context()); err != nil {
		return c.Error(err)
	}

	return c.JSON(BuildAuthTablePayload(c.Context()))
}

func buildMenu(snapshot *accessSnapshot) []map[string]any {
	if snapshot == nil || len(snapshot.auth.rows) == 0 {
		return []map[string]any{}
	}

	children := map[uint64][]menuNode{}
	for _, row := range snapshot.auth.rows {
		if !visibleAuthRow(snapshot, row) {
			continue
		}
		node := menuNodeFromRow(row)
		children[node.ParentID] = append(children[node.ParentID], node)
	}

	for key := range children {
		sort.SliceStable(children[key], func(i, j int) bool {
			if children[key][i].Sort != children[key][j].Sort {
				return children[key][i].Sort < children[key][j].Sort
			}
			return children[key][i].ID < children[key][j].ID
		})
	}

	return buildMenuChildren(children, 0)
}

func menuNodeFromRow(row map[string]any) menuNode {
	return menuNode{
		ID:       authRowID(row),
		Key:      authRowKey(row),
		Name:     util.ToStringTrimmed(row["name"]),
		Icon:     util.ToStringTrimmed(row["icon"]),
		Path:     authRowPath(row),
		ParentID: util.ToUint64(row["parent_id"]),
		Sort:     util.ToIntDefault(row["sort"], 0),
		Type:     util.ToIntDefault(row["type"], 0),
	}
}

func visibleAuthRow(snapshot *accessSnapshot, row map[string]any) bool {
	if snapshot == nil {
		return false
	}
	if len(snapshot.allowed) == 0 {
		return true
	}
	_, ok := snapshot.allowed[authRowID(row)]
	return ok
}

func buildMenuChildren(children map[uint64][]menuNode, parentID uint64) []map[string]any {
	items := make([]map[string]any, 0, len(children[parentID]))
	for _, node := range children[parentID] {
		if node.Type != 1 {
			continue
		}
		item := buildMenuItem(children, node)
		items = append(items, item)
	}
	return items
}

func buildMenuItem(children map[uint64][]menuNode, node menuNode) map[string]any {
	childItems := make([]map[string]any, 0, len(children[node.ID]))
	activePaths := make([]string, 0, 4)
	pathValue := menuSelectablePath(node.Path)

	for _, child := range children[node.ID] {
		if child.Type == 1 {
			childItems = append(childItems, buildMenuItem(children, child))
			continue
		}
		activePaths = appendUniqueStrings(
			activePaths,
			collectHiddenMenuPaths(children, child, pathValue)...,
		)
	}

	item := map[string]any{
		"key":  node.Key,
		"name": node.Name,
		"path": node.Path,
		"icon": node.Icon,
	}
	if len(childItems) > 0 {
		item["children"] = childItems
	} else if len(activePaths) > 0 {
		item["active_paths"] = activePaths
	}
	return item
}

func collectHiddenMenuPaths(children map[uint64][]menuNode, node menuNode, ownerPath string) []string {
	paths := make([]string, 0, 4)
	if pathValue := menuSelectablePath(node.Path); pathValue != "" && normalizeMenuPath(pathValue) != normalizeMenuPath(ownerPath) {
		paths = append(paths, pathValue)
	}

	for _, child := range children[node.ID] {
		paths = appendUniqueStrings(paths, collectHiddenMenuPaths(children, child, ownerPath)...)
	}
	return paths
}

func menuSelectablePath(pathValue string) string {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" || !strings.Contains(pathValue, "/") {
		return ""
	}
	return pathValue
}

func appendUniqueStrings(base []string, values ...string) []string {
	if len(values) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, item := range base {
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}
	for _, item := range values {
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		base = append(base, item)
	}
	return base
}

func resolveMainEntry(configEntry string, menu []map[string]any) string {
	entry := strings.TrimSpace(configEntry)
	if entry != "" && menuHasPath(menu, entry) {
		return entry
	}

	return defaultEntryFromMenu(menu)
}

func menuHasPath(menu []map[string]any, pathValue string) bool {
	target := normalizeMenuPath(pathValue)
	if target == "" {
		return false
	}

	for _, item := range menu {
		if normalizeMenuPath(util.ToString(item["path"])) == target {
			return true
		}

		children, ok := item["children"].([]map[string]any)
		if ok && menuHasPath(children, target) {
			return true
		}
	}

	return false
}

func normalizeMenuPath(pathValue string) string {
	return strings.Trim(strings.TrimSpace(pathValue), "/")
}

func defaultEntryFromMenu(menu []map[string]any) string {
	for _, item := range menu {
		pathValue := strings.TrimSpace(util.ToString(item["path"]))
		if pathValue != "" && strings.Contains(pathValue, "/") {
			return pathValue
		}

		children, ok := item["children"].([]map[string]any)
		if !ok || len(children) == 0 {
			continue
		}

		entry := defaultEntryFromMenu(children)
		if entry != "" {
			return entry
		}
	}

	return ""
}
