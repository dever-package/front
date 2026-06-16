package pagepath

import (
	"path/filepath"
	"strings"
)

const (
	DirName         = "page"
	defaultPageName = "admin"
)

var fileExtCandidates = []string{
	".jsonc",
	".json",
}

func FileExtCandidates() []string {
	return append([]string(nil), fileExtCandidates...)
}

func DiskPageRoute(root, diskPath string) (string, string, bool) {
	return DiskPageRouteForPage(root, diskPath, "")
}

func DiskPageRouteForPage(root, diskPath, pageName string) (string, string, bool) {
	moduleName, relativeParts, ok := DiskPageRelativeParts(root, diskPath)
	if !ok {
		return "", "", false
	}
	if len(relativeParts) == 0 {
		return "", "", false
	}
	hadPagePrefix := pageName != "" && len(relativeParts) > 0 && relativeParts[0] == pageName
	relativeParts = stripPagePathPrefix(relativeParts, pageName)
	if len(relativeParts) == 0 {
		return "", "", false
	}
	if pageName != "" && pageName != defaultPageName && !hadPagePrefix {
		return "", "", false
	}
	if pageName != "" && !hadPagePrefix && strings.HasPrefix(relativeParts[0], "_") {
		return "", "", false
	}

	routePath := TrimPageFileExt(strings.Join(append([]string{moduleName}, relativeParts...), "/"))
	routePath = NormalizePath(routePath)
	if routePath == "" {
		return "", "", false
	}
	return moduleName, routePath, true
}

func DiskPageRelativeParts(root, diskPath string) (string, []string, bool) {
	cleanPath := filepath.ToSlash(filepath.Clean(diskPath))
	parts := strings.Split(cleanPath, "/")
	if len(parts) < 4 || parts[0] != root {
		return "", nil, false
	}

	moduleName := strings.TrimSpace(parts[1])
	if moduleName == "" {
		return "", nil, false
	}

	switch {
	case len(parts) >= 5 && parts[2] == "front" && IsPageDir(parts[3]):
		return moduleName, parts[4:], true
	case IsPageDir(parts[2]):
		return moduleName, parts[3:], true
	default:
		return "", nil, false
	}
}

func DiskPageBelongsToOtherPage(root, diskPath, pageName string, pageNames map[string]struct{}) bool {
	_, relativeParts, ok := DiskPageRelativeParts(root, diskPath)
	if !ok || len(relativeParts) == 0 {
		return false
	}
	return IsOtherPageName(relativeParts[0], pageName, pageNames)
}

func RelativePartsForPage(relativePath, pageName string, pageNames map[string]struct{}) []string {
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	if pageName == "" {
		pageName = defaultPageName
	}

	parts := strings.Split(filepath.ToSlash(relativePath), "/")
	hadPagePrefix := len(parts) > 0 && parts[0] == pageName
	if len(parts) > 0 && !hadPagePrefix && IsOtherPageName(parts[0], pageName, pageNames) {
		return nil
	}

	parts = StripPagePathPrefix(parts, pageName)
	if len(parts) == 0 {
		return nil
	}
	if pageName != defaultPageName && !hadPagePrefix {
		return nil
	}
	if !hadPagePrefix && strings.HasPrefix(parts[0], "_") {
		return nil
	}
	return parts
}

func IsOtherPageName(candidate, pageName string, pageNames map[string]struct{}) bool {
	candidate = strings.Trim(strings.TrimSpace(candidate), "/")
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	if candidate == "" || candidate == pageName {
		return false
	}
	_, ok := pageNames[candidate]
	return ok
}

func StripSitePathPrefix(parts []string, siteKey string) []string {
	return StripPagePathPrefix(parts, siteKey)
}

func StripPagePathPrefix(parts []string, pageName string) []string {
	return stripPagePathPrefix(parts, pageName)
}

func stripPagePathPrefix(parts []string, pageName string) []string {
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	if pageName == "" || len(parts) == 0 || parts[0] != pageName {
		return parts
	}
	return parts[1:]
}

func IsPageDir(name string) bool {
	return name == DirName
}

func IsPageFileName(name string) bool {
	for _, ext := range fileExtCandidates {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func TrimPageFileExt(path string) string {
	for _, ext := range fileExtCandidates {
		if strings.HasSuffix(path, ext) {
			return strings.TrimSuffix(path, ext)
		}
	}
	return path
}

func PageFilePriority(name string) int {
	for index, ext := range fileExtCandidates {
		if strings.HasSuffix(name, ext) {
			return index
		}
	}
	return len(fileExtCandidates)
}

func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if index := strings.IndexAny(path, "?#"); index >= 0 {
		path = path[:index]
	}
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "\\", "/")
	return path
}
