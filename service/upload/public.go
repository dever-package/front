package upload

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"

	uploadprovider "my/package/front/service/upload/provider"
)

func init() {
	server.Auto(func(s server.Server) {
		s.Get("/upload/*", OpenPublicUpload)
	})
}

func OpenPublicUpload(c *server.Context) error {
	rawPath := strings.TrimSpace(c.Input("*"))
	localPath, err := uploadprovider.ResolveLocalPublicFilePath(rawPath)
	if err != nil {
		return c.Error("文件不存在", http.StatusNotFound)
	}
	if _, err := os.Stat(localPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.Error("文件不存在", http.StatusNotFound)
		}
		return c.Error(err)
	}

	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前上传环境不支持文件输出")
	}

	raw.Set("Cache-Control", "public, max-age=31536000")
	return raw.SendFile(localPath)
}
