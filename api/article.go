package api

import (
	"github.com/shemic/dever/server"

	articleservice "github.com/dever-package/front/service/article"
)

type Article struct{}

func (Article) PostImportUrl(c *server.Context) error {
	return articleservice.ImportURL(c)
}
