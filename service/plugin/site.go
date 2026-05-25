package plugin

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
	"github.com/shemic/dever/server"
)

const (
	mountBasePath   = "/_admin/plugins"
	defaultEmbedDir = "front/dist"
	manifestFile    = "manifest.json"
)

type Options struct {
	Name     string
	DiskDir  string
	EmbedDir string
	FS       fs.FS
}

// Register 挂载 package 前端插件静态资源。
func Register(s server.Server, options Options) {
	if s == nil {
		return
	}

	plugin := normalizeOptions(options)
	if plugin.Name == "" {
		return
	}

	mountPath := mountBasePath + "/" + plugin.Name
	open := func(c *server.Context) error {
		return openFile(c, plugin)
	}
	s.Get(mountPath, open)
	s.Get(mountPath+"/*", open)
}

func normalizeOptions(options Options) Options {
	options.Name = cleanName(options.Name)
	if options.DiskDir == "" && options.Name != "" {
		options.DiskDir = filepath.Join("package", options.Name, "front", "dist")
	}
	if strings.TrimSpace(options.EmbedDir) == "" {
		options.EmbedDir = defaultEmbedDir
	}
	return options
}

func openFile(c *server.Context, options Options) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持静态文件输出", http.StatusInternalServerError)
	}

	rel := cleanAssetPath(c.Input("*"))
	if file, err := resolveDiskFile(options.DiskDir, rel); err == nil {
		setContentType(raw, rel)
		return raw.SendFile(file)
	} else if !errors.Is(err, os.ErrNotExist) {
		return c.Error(err, http.StatusNotFound)
	}

	content, err := readEmbeddedFile(options, rel)
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}
	setContentType(raw, rel)
	return raw.Send(content)
}

func resolveDiskFile(rootDir, rel string) (string, error) {
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

func readEmbeddedFile(options Options, rel string) ([]byte, error) {
	if options.FS == nil {
		return nil, os.ErrNotExist
	}

	content, err := fs.ReadFile(options.FS, path.Join(options.EmbedDir, filepath.ToSlash(rel)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return content, nil
}

func cleanName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/") {
		return ""
	}
	return cleaned
}

func cleanAssetPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return manifestFile
	}
	value = strings.TrimPrefix(path.Clean("/"+value), "/")
	if value == "." {
		return manifestFile
	}
	return value
}

func setContentType(raw *fiber.Ctx, rel string) {
	contentType := mime.TypeByExtension(path.Ext(filepath.ToSlash(rel)))
	if contentType == "" {
		return
	}
	raw.Set("Content-Type", contentType)
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
