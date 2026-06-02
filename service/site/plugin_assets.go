package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

type runtimePluginDescriptor struct {
	Name     string   `json:"name"`
	Manifest string   `json:"manifest,omitempty"`
	Entry    string   `json:"entry,omitempty"`
	CSS      []string `json:"css,omitempty"`
	Nodes    []string `json:"nodes,omitempty"`
	Depends  []string `json:"depends,omitempty"`
	Module   bool     `json:"module,omitempty"`
}

type pluginSourceMetadata struct {
	Name    string   `json:"name"`
	Nodes   []string `json:"nodes,omitempty"`
	Depends []string `json:"depends,omitempty"`
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
		return sendSourcePluginManifest(raw, pluginName, sourceRoot)
	case pluginDevRuntime:
		entry := filepath.Join(sourceRoot, pluginSourceEntry)
		runtime := fmt.Sprintf(
			"import plugin from %q;\nwindow.DeverFront?.registerPlugin(plugin);\n",
			versionedViteSourceURL(entry),
		)
		raw.Set("Cache-Control", "no-store")
		raw.Set("Content-Type", "application/javascript; charset=utf-8")
		return raw.SendString(runtime)
	default:
		return c.Error("前端插件源码文件不存在", 404)
	}
}

func sendSourcePluginManifest(raw *fiber.Ctx, pluginName string, sourceRoot string) error {
	metadata := readPluginSourceMetadata(pluginName, filepath.Join(sourceRoot, pluginSourceEntry))
	content, err := json.Marshal(map[string]interface{}{
		"__plugin": metadata,
		pluginDevRuntime: pluginManifestEntry{
			IsEntry: true,
			File:    pluginDevRuntime,
			Module:  true,
		},
	})
	if err != nil {
		return err
	}
	raw.Set("Cache-Control", "no-store")
	raw.Set("Content-Type", "application/json; charset=utf-8")
	return raw.Send(content)
}

func runtimePluginDescriptors(site siteconfig.Site, pluginDev bool) []runtimePluginDescriptor {
	descriptors := make([]runtimePluginDescriptor, 0)
	for _, item := range site.Setting.Runtime.Plugins {
		if descriptor := configuredRuntimePluginDescriptor(item); descriptor.Name != "" || descriptor.Manifest != "" {
			descriptors = append(descriptors, descriptor)
		}
	}
	return uniqueRuntimePluginDescriptors(append(descriptors, discoverRuntimePluginDescriptors(site, pluginDev)...))
}

func discoverRuntimePluginDescriptors(site siteconfig.Site, pluginDev bool) []runtimePluginDescriptor {
	sourceNames := []string{}
	if pluginDev {
		sourceNames = discoverSourcePluginNames()
	}
	distNames := discoverDistPluginNames()

	descriptors := make([]runtimePluginDescriptor, 0, len(sourceNames)+len(distNames))
	seen := map[string]struct{}{}
	for _, name := range sourceNames {
		seen[name] = struct{}{}
		descriptors = append(descriptors, sourceRuntimePluginDescriptor(site, name))
	}
	for _, name := range distNames {
		if _, ok := seen[name]; ok {
			continue
		}
		descriptors = append(descriptors, distRuntimePluginDescriptor(site, name))
	}
	return descriptors
}

func configuredRuntimePluginDescriptor(value string) runtimePluginDescriptor {
	manifest := strings.TrimSpace(value)
	if manifest == "" {
		return runtimePluginDescriptor{}
	}
	return runtimePluginDescriptor{
		Name:     runtimePluginNameFromURL(manifest),
		Manifest: manifest,
	}
}

func sourceRuntimePluginDescriptor(site siteconfig.Site, pluginName string) runtimePluginDescriptor {
	metadata := pluginSourceMetadata{Name: pluginName}
	if sourceRoot, err := resolvePluginSourceRoot(pluginName); err == nil {
		metadata = readPluginSourceMetadata(pluginName, filepath.Join(sourceRoot, pluginSourceEntry))
	}

	return runtimePluginDescriptor{
		Name:     metadata.Name,
		Manifest: pluginSourceManifestURL(site, pluginName),
		Entry:    pluginSourceRuntimeURL(site, pluginName),
		Nodes:    metadata.Nodes,
		Depends:  metadata.Depends,
		Module:   true,
	}
}

func distRuntimePluginDescriptor(site siteconfig.Site, pluginName string) runtimePluginDescriptor {
	metadata := pluginSourceMetadata{Name: pluginName}
	if sourceRoot, err := resolvePluginSourceRoot(pluginName); err == nil {
		metadata = readPluginSourceMetadata(pluginName, filepath.Join(sourceRoot, pluginSourceEntry))
	}

	return runtimePluginDescriptor{
		Name:     metadata.Name,
		Manifest: pluginManifestURL(site, pluginName),
		Nodes:    metadata.Nodes,
		Depends:  metadata.Depends,
	}
}

func readPluginSourceMetadata(defaultName string, entryFile string) pluginSourceMetadata {
	content, err := os.ReadFile(entryFile)
	if err != nil {
		return pluginSourceMetadata{Name: defaultName}
	}
	return extractPluginSourceMetadata(defaultName, string(content))
}

func extractPluginSourceMetadata(defaultName string, content string) pluginSourceMetadata {
	metadata := pluginSourceMetadata{
		Name: strings.TrimSpace(defaultName),
	}

	if name := extractStringProperty(content, "name"); name != "" {
		metadata.Name = name
	}
	if nodesBlock := extractPropertyBlock(content, "nodes", '{', '}'); nodesBlock != "" {
		metadata.Nodes = extractObjectStringKeys(nodesBlock)
	}
	if dependsBlock := extractPropertyBlock(content, "depends", '[', ']'); dependsBlock != "" {
		metadata.Depends = extractStringLiterals(dependsBlock)
	}

	return metadata
}

func extractStringProperty(content string, key string) string {
	re := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(key) + `\s*:\s*` + stringLiteralPattern())
	match := re.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func extractPropertyBlock(content string, key string, open byte, close byte) string {
	re := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(key) + `\s*:`)
	matches := re.FindAllStringIndex(content, -1)
	for _, match := range matches {
		index := match[1]
		for index < len(content) && isSpace(content[index]) {
			index++
		}
		if index >= len(content) || content[index] != open {
			continue
		}
		return matchDelimitedBlock(content, index, open, close)
	}
	return ""
}

func matchDelimitedBlock(content string, start int, open byte, close byte) string {
	depth := 0
	inString := byte(0)
	escaped := false
	for index := start; index < len(content); index++ {
		current := content[index]
		if inString != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == inString {
				inString = 0
			}
			continue
		}

		switch current {
		case '"', '\'', '`':
			inString = current
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return content[start : index+1]
			}
		}
	}
	return ""
}

func extractObjectStringKeys(block string) []string {
	re := regexp.MustCompile(stringLiteralPattern() + `\s*:`)
	matches := re.FindAllStringSubmatch(block, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			values = append(values, strings.TrimSpace(match[1]))
		}
	}
	return uniqueStrings(values)
}

func extractStringLiterals(block string) []string {
	re := regexp.MustCompile(stringLiteralPattern())
	matches := re.FindAllStringSubmatch(block, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			values = append(values, strings.TrimSpace(match[1]))
		}
	}
	return uniqueStrings(values)
}

func isSpace(value byte) bool {
	return value == ' ' || value == '\n' || value == '\r' || value == '\t'
}

func stringLiteralPattern() string {
	return "[\"'`]([^\"'`]+)[\"'`]"
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

func uniqueRuntimePluginDescriptors(items []runtimePluginDescriptor) []runtimePluginDescriptor {
	if len(items) == 0 {
		return []runtimePluginDescriptor{}
	}

	indexes := map[string]int{}
	result := make([]runtimePluginDescriptor, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		item.Manifest = strings.TrimSpace(item.Manifest)
		item.Entry = strings.TrimSpace(item.Entry)
		item.Nodes = uniqueStrings(item.Nodes)
		item.Depends = uniqueStrings(item.Depends)
		if item.Name == "" && item.Manifest == "" && item.Entry == "" {
			continue
		}

		key := runtimePluginDescriptorKey(item)
		if index, ok := indexes[key]; ok {
			if shouldReplaceRuntimePluginDescriptor(result[index], item) {
				result[index] = item
			}
			continue
		}

		indexes[key] = len(result)
		result = append(result, item)
	}
	return result
}

func runtimePluginDescriptorKey(item runtimePluginDescriptor) string {
	if item.Name != "" {
		return "name:" + item.Name
	}
	if item.Manifest != "" {
		return "manifest:" + item.Manifest
	}
	return "entry:" + item.Entry
}

func shouldReplaceRuntimePluginDescriptor(current runtimePluginDescriptor, next runtimePluginDescriptor) bool {
	if len(current.Nodes) == 0 && len(next.Nodes) > 0 {
		return true
	}
	if current.Entry == "" && next.Entry != "" {
		return true
	}
	return false
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
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

func pluginSourceRuntimeURL(site siteconfig.Site, pluginName string) string {
	return cleanRequestPath(path.Join(site.Path, pluginSourceMountDir, pluginName, pluginDevRuntime))
}

func runtimePluginNameFromURL(value string) string {
	cleaned := strings.TrimSpace(strings.Split(strings.Split(value, "?")[0], "#")[0])
	if cleaned == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	for index, part := range parts {
		if part == pluginManifest && index > 0 {
			return cleanPluginName(parts[index-1])
		}
		if part == pluginDevRuntime && index > 0 {
			return cleanPluginName(parts[index-1])
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return cleanPluginName(parts[len(parts)-1])
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

func viteSourceURL(file string) string {
	file = strings.TrimSpace(file)
	if file == "" || filepath.IsAbs(file) {
		return viteFSURL(file)
	}

	cleaned := filepath.ToSlash(filepath.Clean(file))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return viteFSURL(file)
	}
	return "/" + cleaned
}

func versionedViteSourceURL(file string) string {
	url := viteSourceURL(file)
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		return url
	}

	separator := "?"
	if strings.Contains(url, "?") {
		separator = "&"
	}
	return url + separator + "v=" + strconv.FormatInt(info.ModTime().UnixNano(), 36)
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
