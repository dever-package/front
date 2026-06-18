package render

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/component"
	"github.com/shemic/dever/server"

	"my/package/front/service/siteconfig"
)

const projectAssetRoot = "config/front/assets"

func RegisterAssets(s server.Server, site siteconfig.Site) {
	if s == nil {
		return
	}
	assetRoute := cleanAbsPath(path.Join(site.APIPrefix(), "assets", "*"))
	s.Get(assetRoute, func(c *server.Context) error {
		c.SetContext(siteconfig.WithSite(c.Context(), site))
		return OpenAsset(c, site)
	})
}

func OpenAsset(c *server.Context, site siteconfig.Site) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持静态资源输出", http.StatusInternalServerError)
	}

	rel := cleanRelativePath(c.Input("*"))
	if rel == "" || rel == "index.html" {
		return c.Error("资源不存在", http.StatusNotFound)
	}

	if served, err := openProjectAsset(raw, site, rel); served || err != nil {
		return err
	}
	content, servedRel, err := ReadComponentAsset(site.Page, rel)
	if err != nil {
		return c.Error("资源不存在", http.StatusNotFound)
	}

	raw.Set("Cache-Control", cacheHeader(servedRel))
	setContentType(raw, servedRel)
	return raw.Send(content)
}

func openProjectAsset(raw *fiber.Ctx, site siteconfig.Site, rel string) (bool, error) {
	root, err := filepath.Abs(filepath.Join(projectAssetRoot, site.Key))
	if err != nil {
		return false, err
	}
	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := ensureInside(root, file); err != nil {
		return false, err
	}
	info, err := os.Stat(file)
	if err == nil && !info.IsDir() {
		raw.Set("Cache-Control", "no-cache")
		setContentType(raw, rel)
		return true, raw.SendFile(file)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, nil
}

func ReadComponentAsset(pageName, rel string) ([]byte, string, error) {
	if cleanRelativePath(pageName) == "" || cleanRelativePath(rel) == "" {
		return nil, "", fs.ErrNotExist
	}
	for _, current := range component.Active() {
		if current.PageFS == nil {
			continue
		}
		fullPath := componentAssetPath(current, pageName, rel)
		content, err := fs.ReadFile(current.PageFS, fullPath)
		if err == nil {
			return content, rel, nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, "", err
		}
	}
	return nil, "", fs.ErrNotExist
}

func setContentType(raw *fiber.Ctx, rel string) {
	contentType := mime.TypeByExtension(path.Ext(filepath.ToSlash(rel)))
	if contentType == "" {
		return
	}
	raw.Set("Content-Type", contentType)
}

func cacheHeader(rel string) string {
	if strings.TrimSpace(rel) == "" {
		return "no-cache"
	}
	return "public, max-age=31536000, immutable"
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
