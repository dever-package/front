package api

import (
	"context"

	"github.com/shemic/dever/server"

	operationlog "github.com/dever-package/front/service/operationlog"
	pageservice "github.com/dever-package/front/service/page"
	permissionservice "github.com/dever-package/front/service/permission"
	"github.com/dever-package/front/service/siteconfig"
)

type Main struct{}

func (Main) GetInfo(c *server.Context) error {
	return permissionservice.GetMainInfo(c)
}

func (Main) GetBootstrap(c *server.Context) error {
	mainInfo, err := permissionservice.LoadMainInfo(c, false)
	if err != nil {
		return c.Error(err)
	}

	mainPath := mainSchemaPath(c.Context())
	schema, err := pageservice.BuildInfo(c, mainPath)
	if err != nil {
		return c.Error(err)
	}

	return c.JSON(map[string]any{
		"main":   mainInfo,
		"path":   mainPath,
		"schema": schema,
	})
}

func (Main) PostSync(c *server.Context) error {
	err := permissionservice.SyncMainInfo(c)
	if err == nil {
		operationlog.Record(c, operationlog.Entry{
			Action:   "sync",
			PagePath: "front/auth/list",
			Message:  "同步权限",
		})
	}
	return err
}

func mainSchemaPath(ctx context.Context) string {
	if site, ok := siteconfig.FromContext(ctx); ok {
		return site.SystemPagePath("main")
	}
	return siteconfig.Site{API: siteconfig.DefaultAPI}.SystemPagePath("main")
}
