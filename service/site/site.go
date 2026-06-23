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

	frontroot "github.com/dever-package/front"
	renderservice "github.com/dever-package/front/service/render"
	"github.com/dever-package/front/service/siteconfig"
)

const (
	defaultDir          = "package/front/front/html"
	embedDir            = "front/html"
	indexFile           = "index.html"
	siteAssetPrefix     = "assets/"
	defaultPluginDevURL = "http://127.0.0.1:5174"
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
		registerPluginAssets(s, currentSite, siteSettings, frontConfig)
		renderservice.RegisterSite(s, currentSite)
		open := func(c *server.Context) error {
			if isHostBoundLegacySitePath(frontConfig, c) {
				return c.Error("资源不存在", http.StatusNotFound)
			}
			c.SetContext(siteconfig.WithSite(c.Context(), currentSite))
			return openFile(c, siteSettings, currentSite)
		}
		runtime := func(c *server.Context) error {
			if isHostBoundLegacySitePath(frontConfig, c) {
				return c.Error("资源不存在", http.StatusNotFound)
			}
			c.SetContext(siteconfig.WithSite(c.Context(), currentSite))
			return writeRuntime(c, currentSite, siteSettings.pluginDev)
		}
		s.Get(currentSite.Path, open)
		s.Get(currentSite.Path+"/runtime.js", runtime)
		s.Get(currentSite.Path+"/*", open)
	}
	registerHostBoundSites(s, siteSettings, frontConfig)
}

func registerHostBoundSites(s server.Server, siteSettings settings, frontConfig siteconfig.Config) {
	if !frontConfig.HasHostBindings() {
		return
	}
	registerHostBoundPluginAssets(s, siteSettings, frontConfig)
	open := func(c *server.Context) error {
		currentSite, ok := requestHostBoundSite(frontConfig, c)
		if !ok {
			return c.Error("资源不存在", http.StatusNotFound)
		}
		c.SetContext(siteconfig.WithSite(c.Context(), currentSite))
		return openFile(c, siteSettings, currentSite)
	}
	runtime := func(c *server.Context) error {
		currentSite, ok := requestHostBoundSite(frontConfig, c)
		if !ok {
			return c.Error("资源不存在", http.StatusNotFound)
		}
		c.SetContext(siteconfig.WithSite(c.Context(), currentSite))
		return writeRuntime(c, currentSite, siteSettings.pluginDev)
	}
	s.Get("/", open)
	s.Get("/runtime.js", runtime)
	s.Get("/assets/*", open)
	s.Get("/*", open)
}

func requestHostBoundSite(frontConfig siteconfig.Config, c *server.Context) (siteconfig.Site, bool) {
	if c == nil {
		return siteconfig.Site{}, false
	}
	return frontConfig.FindByHost(siteconfig.RequestHost(c.Header("X-Forwarded-Host"), c.Header("Host")))
}

func isHostBoundLegacySitePath(frontConfig siteconfig.Config, c *server.Context) bool {
	if c == nil {
		return false
	}
	if _, ok := requestHostBoundSite(frontConfig, c); !ok {
		return false
	}
	if _, ok := frontConfig.FindByAPIRequestPath(c.Path()); ok {
		return false
	}
	site, ok := frontConfig.FindBySitePath(c.Path())
	return ok && cleanRequestPath(site.Path) != "/"
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
	requestPath := cleanRequestPath(c.Path())
	if strings.HasPrefix(requestPath, "/"+siteAssetPrefix) && !strings.HasPrefix(rel, siteAssetPrefix) {
		rel = cleanAssetPath(requestPath)
	}
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

	if err := renderservice.SendAssetRef(raw, assetRel); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
