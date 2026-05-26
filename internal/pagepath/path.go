package pagepath

import (
	"path/filepath"
	"strings"
)

const DirName = "page"

var fileExtCandidates = []string{
	".jsonc",
	".json",
}

func FileExtCandidates() []string {
	return append([]string(nil), fileExtCandidates...)
}

func DiskPageRoute(root, diskPath string) (string, string, bool) {
	cleanPath := filepath.ToSlash(filepath.Clean(diskPath))
	parts := strings.Split(cleanPath, "/")
	if len(parts) < 4 || parts[0] != root {
		return "", "", false
	}

	moduleName := strings.TrimSpace(parts[1])
	if moduleName == "" {
		return "", "", false
	}

	var relativeParts []string
	switch {
	case len(parts) >= 5 && parts[2] == "front" && IsPageDir(parts[3]):
		relativeParts = parts[4:]
	case IsPageDir(parts[2]):
		relativeParts = parts[3:]
	default:
		return "", "", false
	}
	if len(relativeParts) == 0 {
		return "", "", false
	}

	routePath := TrimPageFileExt(strings.Join(append([]string{moduleName}, relativeParts...), "/"))
	routePath = NormalizePath(routePath)
	if routePath == "" {
		return "", "", false
	}
	return moduleName, routePath, true
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
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "\\", "/")
	return path
}
