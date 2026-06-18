package site

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/config"
	"github.com/shemic/dever/server"

	frontroot "my/package/front"
	renderservice "my/package/front/service/render"
	"my/package/front/service/siteconfig"
)

const (
	defaultDir               = "package/front/html"
	embedDir                 = "html"
	indexFile                = "index.html"
	siteAssetPrefix          = "assets/"
	siteAssetDir             = "config/front/assets"
	defaultBundledAssetScope = "assets/images"
	defaultPluginDevURL      = "http://127.0.0.1:5174"
)

type settings struct {
	enabled      bool
	dir          string
	pluginDev    bool
	pluginDevURL string
}

// Register 按配置把前端静态文件挂到后端路由。
func Register(s server.Server) {
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Errorf("读取前端静态站点配置失败: %w", err))
	}
	frontConfig, err := siteconfig.Load(nil)
	if err != nil {
		panic(fmt.Errorf("读取 front 站点配置失败: %w", err))
	}
	register(s, settingsFromConfig(cfg.FrontSite), frontConfig)
}

func register(s server.Server, siteSettings settings, frontConfig siteconfig.Config) {
	if !siteSettings.enabled || s == nil {
		return
	}

	registerPluginDevProxy(s, siteSettings)

	for _, site := range frontConfig.Sites {
		currentSite := site
		registerPluginAssets(s, currentSite, siteSettings)
		renderservice.RegisterSite(s, currentSite)
		open := func(c *server.Context) error {
			c.SetContext(siteconfig.WithSite(c.Context(), currentSite))
			return openFile(c, siteSettings, currentSite)
		}
		runtime := func(c *server.Context) error {
			c.SetContext(siteconfig.WithSite(c.Context(), currentSite))
			return writeRuntime(c, currentSite, siteSettings.pluginDev)
		}
		s.Get(currentSite.Path, open)
		s.Get(currentSite.Path+"/runtime.js", runtime)
		s.Get(currentSite.Path+"/*", open)
	}
}

func settingsFromConfig(cfg config.FrontSite) settings {
	enabled := true
	if cfg.Enabled != nil {
		enabled = *cfg.Enabled
	}
	return settings{
		enabled:      enabled,
		dir:          cleanDir(cfg.Dir),
		pluginDev:    siteconfig.PluginDevEnabled(cfg),
		pluginDevURL: frontPluginDevURL(cfg),
	}
}

func openFile(c *server.Context, site settings, currentSite siteconfig.Site) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持静态文件输出", http.StatusInternalServerError)
	}

	rel := cleanAssetPath(c.Input("*"))
	served, err := openSiteAsset(raw, currentSite, rel)
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}
	if served {
		return nil
	}

	file, cache, err := resolveDiskFile(site.dir, rel)
	if err == nil {
		raw.Set("Cache-Control", cache)
		if filepath.Base(file) == indexFile {
			content, err := os.ReadFile(file)
			if err != nil {
				return c.Error(err, http.StatusNotFound)
			}
			content, err = injectRuntime(content, currentSite, site.pluginDev)
			if err != nil {
				return c.Error(err, http.StatusInternalServerError)
			}
			setContentType(raw, indexFile)
			return raw.Send(content)
		}
		return raw.SendFile(file)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return c.Error(err, http.StatusNotFound)
	}

	content, servedRel, cache, err := resolveEmbeddedFile(rel)
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}
	raw.Set("Cache-Control", cache)
	setContentType(raw, servedRel)
	if servedRel == indexFile {
		content, err = injectRuntime(content, currentSite, site.pluginDev)
		if err != nil {
			return c.Error(err, http.StatusInternalServerError)
		}
	}
	return raw.Send(content)
}

func frontPluginDevURL(cfg config.FrontSite) string {
	if value := strings.TrimSpace(os.Getenv("DEVER_FRONT_PLUGIN_DEV_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(cfg.PluginDev.URL); value != "" {
		return strings.TrimRight(value, "/")
	}
	if cfg.PluginDev.Port > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d", cfg.PluginDev.Port)
	}
	if siteconfig.PluginDevEnabled(cfg) {
		return defaultPluginDevURL
	}
	return ""
}

func openSiteAsset(raw *fiber.Ctx, site siteconfig.Site, rel string) (bool, error) {
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, siteAssetPrefix) {
		return false, nil
	}

	assetRel := cleanAssetPath(strings.TrimPrefix(rel, siteAssetPrefix))
	if assetRel == indexFile {
		return false, nil
	}

	root, err := filepath.Abs(filepath.Join(siteAssetDir, site.Key))
	if err != nil {
		return false, err
	}
	file := filepath.Join(root, filepath.FromSlash(assetRel))
	if err := ensureInside(root, file); err != nil {
		return false, err
	}
	info, err := os.Stat(file)
	if err == nil && !info.IsDir() {
		raw.Set("Cache-Control", "no-cache")
		setContentType(raw, assetRel)
		return true, raw.SendFile(file)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if content, servedRel, err := renderservice.ReadComponentAsset(site.Page, assetRel); err == nil {
		raw.Set("Cache-Control", cacheHeader(servedRel))
		setContentType(raw, servedRel)
		return true, raw.Send(content)
	}

	return openBundledSiteAsset(raw, assetRel)
}

func openBundledSiteAsset(raw *fiber.Ctx, assetRel string) (bool, error) {
	bundledRel := filepath.ToSlash(filepath.Join(defaultBundledAssetScope, filepath.Base(assetRel)))
	content, servedRel, err := readEmbeddedFile(bundledRel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	raw.Set("Cache-Control", cacheHeader(servedRel))
	setContentType(raw, servedRel)
	return true, raw.Send(content)
}

func resolveDiskFile(rootDir, rel string) (string, string, error) {
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", "", err
	}

	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := ensureInside(root, file); err != nil {
		return "", "", err
	}

	info, err := os.Stat(file)
	if err == nil && !info.IsDir() {
		return file, cacheHeader(rel), nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	if isAssetRequest(rel) {
		return "", "", os.ErrNotExist
	}

	index := filepath.Join(root, indexFile)
	if err := ensureInside(root, index); err != nil {
		return "", "", err
	}
	if info, err := os.Stat(index); err != nil || info.IsDir() {
		return "", "", os.ErrNotExist
	}
	return index, cacheHeader(indexFile), nil
}

func resolveEmbeddedFile(rel string) ([]byte, string, string, error) {
	content, servedRel, err := readEmbeddedFile(rel)
	if err == nil {
		return content, servedRel, cacheHeader(servedRel), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, "", "", err
	}
	if isAssetRequest(rel) {
		return nil, "", "", os.ErrNotExist
	}

	content, servedRel, err = readEmbeddedFile(indexFile)
	if err != nil {
		return nil, "", "", err
	}
	return content, servedRel, cacheHeader(indexFile), nil
}

func readEmbeddedFile(rel string) ([]byte, string, error) {
	servedRel := cleanAssetPath(rel)
	embeddedPath := path.Join(embedDir, filepath.ToSlash(servedRel))
	content, err := frontroot.SiteFS.ReadFile(embeddedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}
	return content, servedRel, nil
}

func setContentType(raw *fiber.Ctx, rel string) {
	contentType := mime.TypeByExtension(path.Ext(filepath.ToSlash(rel)))
	if contentType == "" {
		return
	}
	raw.Set("Content-Type", contentType)
}

func cleanDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultDir
	}
	return value
}

func cleanRequestPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	return path.Clean(value)
}

func cleanAssetPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return indexFile
	}
	value = strings.TrimPrefix(path.Clean("/"+value), "/")
	if value == "." {
		return indexFile
	}
	return value
}

func ensureInside(root, file string) error {
	root = filepath.Clean(root)
	file = filepath.Clean(file)
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return nil
	}
	return os.ErrPermission
}

func isAssetRequest(rel string) bool {
	if rel == indexFile {
		return false
	}
	return filepath.Ext(rel) != ""
}

func cacheHeader(rel string) string {
	name := path.Base(filepath.ToSlash(rel))
	if name == indexFile {
		return "no-cache"
	}
	if strings.HasPrefix(filepath.ToSlash(rel), "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=86400"
}
