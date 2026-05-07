package embedpage

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/util"
	frontroot "my/package/front"
)

const cacheKey = "default"

var parentCache util.ConcurrentMap[string, map[string]map[string]struct{}]

func ClearCache() {
	parentCache.Delete(cacheKey)
}

func Paths() map[string]struct{} {
	parents := Parents()
	result := make(map[string]struct{}, len(parents))
	for childPath := range parents {
		result[childPath] = struct{}{}
	}
	return result
}

func HasPath(path string) bool {
	path = normalizePath(path)
	if path == "" {
		return false
	}

	_, ok := Parents()[path]
	return ok
}

func IsChild(parentPath, childPath string) bool {
	parentPath = normalizePath(parentPath)
	childPath = normalizePath(childPath)
	if parentPath == "" || childPath == "" {
		return false
	}

	parents := Parents()[childPath]
	_, ok := parents[parentPath]
	return ok
}

func FilterRows(rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}

	embeddedParents := Parents()
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
	if cached, ok := parentCache.Load(cacheKey); ok {
		return cached
	}

	result := map[string]map[string]struct{}{}
	_ = walkParents(result)
	parentCache.Store(cacheKey, result)
	return result
}

func walkParents(result map[string]map[string]struct{}) error {
	for _, root := range []string{"module", "package"} {
		if err := walkDiskParents(root, result); err != nil {
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
		routePath := trimPageFileExt(filepath.ToSlash(filepath.Join("front", relativePath)))
		collectParents(result, routePath, content)
		return nil
	})
}

func walkDiskParents(root string, result map[string]map[string]struct{}) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() || !isPageFileName(entry.Name()) {
			return nil
		}

		cleanPath := filepath.ToSlash(filepath.Clean(path))
		parts := strings.Split(cleanPath, "/")
		if len(parts) < 4 || parts[0] != root || parts[1] == "front" || parts[2] != "page" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		routePath := trimPageFileExt(strings.Join(append([]string{parts[1]}, parts[3:]...), "/"))
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
	return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonc")
}

func trimPageFileExt(path string) string {
	for _, ext := range []string{".jsonc", ".json"} {
		if strings.HasSuffix(path, ext) {
			return strings.TrimSuffix(path, ext)
		}
	}
	return path
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "\\", "/")
	return path
}
