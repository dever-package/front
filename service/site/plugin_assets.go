package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"

	"my/package/front/service/siteconfig"
)

const (
	pluginMountDir       = "plugins"
	pluginSourceMountDir = "plugins-src"
	pluginDistDir        = "front/dist"
	pluginSourceDir      = "front/src"
	pluginManifest       = "manifest.json"
	pluginSourceEntry    = "plugin.ts"
	pluginDevRuntime     = "runtime.js"
)

type pluginManifestEntry struct {
	IsEntry bool     `json:"isEntry"`
	File    string   `json:"file"`
	Module  bool     `json:"module,omitempty"`
	CSS     []string `json:"css,omitempty"`
}

func registerPluginAssets(s server.Server, site siteconfig.Site, siteSettings settings) {
	mountPath := pluginMountPath(site)
	open := func(c *server.Context) error {
		c.SetContext(siteconfig.WithSite(c.Context(), site))
		return openPluginAsset(c)
	}
	s.Get(mountPath+"/*", open)

	if !siteSettings.pluginDev {
		return
	}

	sourceMountPath := pluginSourceMountPath(site)
	openSource := func(c *server.Context) error {
		c.SetContext(siteconfig.WithSite(c.Context(), site))
		return openSourcePluginAsset(c)
	}
	s.Get(sourceMountPath+"/*", openSource)
}

func openPluginAsset(c *server.Context) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持前端插件输出")
	}

	pluginName, rel, ok := splitPluginAssetPath(c.Input("*"))
	if !ok {
		return c.Error("前端插件路径不合法", 404)
	}

	for _, root := range pluginDiskRoots(pluginName) {
		file, err := resolvePluginDiskFile(root, rel)
		if err == nil {
			raw.Set("Cache-Control", "no-cache")
			setContentType(raw, rel)
			return raw.SendFile(file)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return c.Error(err, 404)
		}
	}

	return c.Error("前端插件不存在", 404)
}

func openSourcePluginAsset(c *server.Context) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持前端插件源码输出")
	}

	pluginName, rel, ok := splitPluginAssetPath(c.Input("*"))
	if !ok {
		return c.Error("前端插件源码路径不合法", 404)
	}

	sourceRoot, err := resolvePluginSourceRoot(pluginName)
	if err != nil {
		return c.Error("前端插件源码不存在", 404)
	}

	switch rel {
	case pluginManifest:
		return sendSourcePluginManifest(raw)
	case pluginDevRuntime:
		entry := filepath.Join(sourceRoot, pluginSourceEntry)
		runtime := fmt.Sprintf(
			"import plugin from %q;\nwindow.DeverFront?.registerPlugin(plugin);\n",
			viteFSURL(entry),
		)
		raw.Set("Cache-Control", "no-cache")
		raw.Set("Content-Type", "application/javascript; charset=utf-8")
		return raw.SendString(runtime)
	default:
		return c.Error("前端插件源码文件不存在", 404)
	}
}

func sendSourcePluginManifest(raw *fiber.Ctx) error {
	content, err := json.Marshal(map[string]pluginManifestEntry{
		pluginDevRuntime: {
			IsEntry: true,
			File:    pluginDevRuntime,
			Module:  true,
		},
	})
	if err != nil {
		return err
	}
	raw.Set("Cache-Control", "no-cache")
	raw.Set("Content-Type", "application/json; charset=utf-8")
	return raw.Send(content)
}

func runtimePluginURLs(site siteconfig.Site, pluginDev bool) []string {
	return uniqueRuntimePluginURLs(
		append(
			site.Setting.Runtime.Plugins,
			discoverRuntimePluginURLs(site, pluginDev)...,
		),
	)
}

func discoverRuntimePluginURLs(site siteconfig.Site, pluginDev bool) []string {
	sourceNames := []string{}
	if pluginDev {
		sourceNames = discoverSourcePluginNames()
	}
	distNames := discoverDistPluginNames()

	urls := make([]string, 0, len(sourceNames)+len(distNames))
	seen := map[string]struct{}{}
	for _, name := range sourceNames {
		seen[name] = struct{}{}
		urls = append(urls, pluginSourceManifestURL(site, name))
	}
	for _, name := range distNames {
		if _, ok := seen[name]; ok {
			continue
		}
		urls = append(urls, pluginManifestURL(site, name))
	}
	return urls
}

func discoverDistPluginNames() []string {
	return discoverPluginNamesWithFile(filepath.Join(pluginDistDir, pluginManifest))
}

func discoverSourcePluginNames() []string {
	return discoverPluginNamesWithFile(filepath.Join(pluginSourceDir, pluginSourceEntry))
}

func discoverPluginNamesWithFile(relativeFile string) []string {
	names := map[string]struct{}{}
	for _, root := range pluginParentRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := cleanPluginName(entry.Name())
			if name == "" {
				continue
			}
			filePath := filepath.Join(root, name, relativeFile)
			if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
				names[name] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func uniqueRuntimePluginURLs(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		urls = append(urls, item)
	}
	return urls
}

func pluginMountPath(site siteconfig.Site) string {
	return cleanRequestPath(path.Join(site.Path, pluginMountDir))
}

func pluginSourceMountPath(site siteconfig.Site) string {
	return cleanRequestPath(path.Join(site.Path, pluginSourceMountDir))
}

func pluginManifestURL(site siteconfig.Site, pluginName string) string {
	return cleanRequestPath(path.Join(site.Path, pluginMountDir, pluginName, pluginManifest))
}

func pluginSourceManifestURL(site siteconfig.Site, pluginName string) string {
	return cleanRequestPath(path.Join(site.Path, pluginSourceMountDir, pluginName, pluginManifest))
}

func pluginDiskRoots(pluginName string) []string {
	return pluginFrontRoots(pluginName, pluginDistDir)
}

func pluginSourceRoots(pluginName string) []string {
	return pluginFrontRoots(pluginName, pluginSourceDir)
}

func pluginFrontRoots(pluginName string, subDir string) []string {
	roots := pluginParentRoots()
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		result = append(result, filepath.Join(root, pluginName, subDir))
	}
	return result
}

func pluginParentRoots() []string {
	return uniquePaths([]string{
		"module",
		filepath.Join("backend", "module"),
		"package",
		filepath.Join("backend", "package"),
	})
}

func splitPluginAssetPath(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(path.Clean("/"+value), "/")
	if value == "." || value == "" {
		return "", "", false
	}

	parts := strings.SplitN(value, "/", 2)
	pluginName := cleanPluginName(parts[0])
	if pluginName == "" {
		return "", "", false
	}

	rel := pluginManifest
	if len(parts) > 1 {
		rel = cleanPluginAssetRel(parts[1])
	}
	if rel == "" {
		return "", "", false
	}
	return pluginName, rel, true
}

func cleanPluginName(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "/") {
		return ""
	}
	return cleaned
}

func cleanPluginAssetRel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return pluginManifest
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+value), "/")
	if cleaned == "." {
		return pluginManifest
	}
	return cleaned
}

func resolvePluginDiskFile(rootDir, rel string) (string, error) {
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}

	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := ensureInside(root, file); err != nil {
		return "", err
	}

	info, err := os.Stat(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	if info.IsDir() {
		return "", os.ErrNotExist
	}
	return file, nil
}

func resolvePluginSourceRoot(pluginName string) (string, error) {
	for _, root := range pluginSourceRoots(pluginName) {
		entry := filepath.Join(root, pluginSourceEntry)
		info, err := os.Stat(entry)
		if err == nil && !info.IsDir() {
			return root, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", os.ErrNotExist
}

func viteFSURL(file string) string {
	absolute, err := filepath.Abs(file)
	if err != nil {
		absolute = file
	}
	return "/@fs/" + filepath.ToSlash(absolute)
}

func uniquePaths(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = filepath.Clean(strings.TrimSpace(item))
		if item == "" || item == "." {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
