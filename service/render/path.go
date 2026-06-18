package render

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/component"
)

func cleanRelativePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	value = path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if value == "." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return ""
	}
	return value
}

func cleanAbsPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return path.Clean("/" + strings.Trim(value, "/"))
}

func componentTemplatePath(current component.Component, pageName, value string) string {
	return filepath.ToSlash(filepath.Join(componentResourceRoot(current, "template"), cleanRelativePath(pageName), cleanRelativePath(value)))
}

func componentAssetPath(current component.Component, pageName, value string) string {
	return filepath.ToSlash(filepath.Join(componentResourceRoot(current, "assets"), cleanRelativePath(pageName), cleanRelativePath(value)))
}

func componentAssetRefPath(current component.Component, rel string) string {
	return filepath.ToSlash(filepath.Join(componentResourceRoot(current, "assets"), cleanRelativePath(rel)))
}

func componentResourceRoot(current component.Component, name string) string {
	prefix := strings.Trim(filepath.ToSlash(current.PagePrefix), "/")
	if prefix == "" || prefix == "." {
		return name
	}
	dir := path.Dir(prefix)
	if dir == "." {
		return name
	}
	return path.Join(dir, name)
}

func assetBase(sitePath string) string {
	sitePath = strings.Trim(cleanAbsPath(sitePath), "/")
	if sitePath == "" {
		return "/assets"
	}
	return "/" + path.Join(sitePath, "assets")
}
