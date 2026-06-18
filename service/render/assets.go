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
const configAssetPrefix = "config/assets/"

func RegisterAssets(s server.Server, site siteconfig.Site) {
	if s == nil {
		return
	}
	assetRoute := cleanAbsPath(path.Join(site.Path, "assets", "*"))
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

	if err := SendAssetRef(raw, rel); err != nil {
		return c.Error("资源不存在", http.StatusNotFound)
	}
	return nil
}

func SendAssetRef(raw *fiber.Ctx, ref string) error {
	asset, err := ReadAssetRef(ref)
	if err != nil {
		return err
	}
	raw.Set("Cache-Control", asset.CacheControl)
	setContentType(raw, asset.ServedRel)
	if asset.FilePath != "" {
		return raw.SendFile(asset.FilePath)
	}
	return raw.Send(asset.Content)
}

type AssetResult struct {
	Content      []byte
	FilePath     string
	ServedRel    string
	CacheControl string
}

func ReadAssetRef(ref string) (AssetResult, error) {
	ref = cleanRelativePath(ref)
	if ref == "" {
		return AssetResult{}, fs.ErrNotExist
	}
	if strings.HasPrefix(ref, configAssetPrefix) {
		return readProjectAssetRef(strings.TrimPrefix(ref, configAssetPrefix))
	}
	componentName, rel, ok := splitComponentAssetRef(ref)
	if !ok {
		return AssetResult{}, fs.ErrNotExist
	}
	return readComponentAssetRef(componentName, rel)
}

func readProjectAssetRef(rel string) (AssetResult, error) {
	rel = cleanRelativePath(rel)
	if rel == "" {
		return AssetResult{}, fs.ErrNotExist
	}
	root, err := filepath.Abs(projectAssetRoot)
	if err != nil {
		return AssetResult{}, err
	}
	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := ensureInside(root, file); err != nil {
		return AssetResult{}, err
	}
	info, err := os.Stat(file)
	if err == nil && !info.IsDir() {
		return AssetResult{
			FilePath:     file,
			ServedRel:    rel,
			CacheControl: "no-cache",
		}, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return AssetResult{}, err
	}
	return AssetResult{}, fs.ErrNotExist
}

func splitComponentAssetRef(ref string) (string, string, bool) {
	parts := strings.SplitN(cleanRelativePath(ref), "/", 3)
	if len(parts) != 3 || parts[1] != "assets" {
		return "", "", false
	}
	componentName := strings.TrimSpace(parts[0])
	rel := cleanRelativePath(parts[2])
	return componentName, rel, componentName != "" && rel != ""
}

func readComponentAssetRef(componentName, rel string) (AssetResult, error) {
	current, ok := component.Find(componentName)
	if !ok || current.PageFS == nil {
		return AssetResult{}, fs.ErrNotExist
	}
	content, servedRel, err := ReadComponentAsset(current, rel)
	if err != nil {
		return AssetResult{}, err
	}
	return AssetResult{
		Content:      content,
		ServedRel:    servedRel,
		CacheControl: cacheHeader(servedRel),
	}, nil
}

func ReadComponentAsset(current component.Component, rel string) ([]byte, string, error) {
	if current.PageFS == nil || cleanRelativePath(rel) == "" {
		return nil, "", fs.ErrNotExist
	}
	fullPath := componentAssetRefPath(current, rel)
	content, err := fs.ReadFile(current.PageFS, fullPath)
	if err == nil {
		return content, rel, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
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
