package site

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/config"
	"github.com/shemic/dever/server"
)

const (
	defaultPath = "/_admin"
	defaultDir  = "package/front/html"
	indexFile   = "index.html"
)

type settings struct {
	enabled bool
	path    string
	dir     string
}

// Register 按配置把前端静态文件挂到后端路由。
func Register(s server.Server) {
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Errorf("读取前端静态站点配置失败: %w", err))
	}
	register(s, settingsFromConfig(cfg.FrontSite))
}

func Allows(cfg config.FrontSite, requestPath string) bool {
	site := settingsFromConfig(cfg)
	if !site.enabled {
		return false
	}
	requestPath = cleanRequestPath(requestPath)
	return requestPath == site.path || strings.HasPrefix(requestPath, site.path+"/")
}

func register(s server.Server, site settings) {
	if !site.enabled || s == nil {
		return
	}

	open := func(c *server.Context) error {
		return openFile(c, site)
	}
	s.Get(site.path, open)
	s.Get(site.path+"/*", open)
}

func settingsFromConfig(cfg config.FrontSite) settings {
	enabled := true
	if cfg.Enabled != nil {
		enabled = *cfg.Enabled
	}
	return settings{
		enabled: enabled,
		path:    cleanMountPath(cfg.Path),
		dir:     cleanDir(cfg.Dir),
	}
}

func openFile(c *server.Context, site settings) error {
	file, cache, err := resolveFile(site.dir, c.Input("*"))
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}

	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持静态文件输出", http.StatusInternalServerError)
	}
	raw.Set("Cache-Control", cache)
	return raw.SendFile(file)
}

func resolveFile(rootDir, requestPath string) (string, string, error) {
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", "", err
	}

	rel := cleanAssetPath(requestPath)
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

func cleanDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultDir
	}
	return value
}

func cleanMountPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return defaultPath
	}
	value = "/" + strings.Trim(value, "/")
	return path.Clean(value)
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
	if name == indexFile || name == "config.js" {
		return "no-cache"
	}
	if strings.HasPrefix(filepath.ToSlash(rel), "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "public, max-age=86400"
}
