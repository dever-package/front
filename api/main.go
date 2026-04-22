package api

import (
	"github.com/shemic/dever/server"

	permissionservice "github.com/dever-package/front/service/permission"
)

type Main struct{}

func (Main) GetInfo(c *server.Context) error {
	return permissionservice.GetMainInfo(c)
}

func (Main) PostSync(c *server.Context) error {
	return permissionservice.SyncMainInfo(c)
}
