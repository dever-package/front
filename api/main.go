package api

import (
	"github.com/shemic/dever/server"

	operationlog "my/package/front/service/operationlog"
	permissionservice "my/package/front/service/permission"
)

type Main struct{}

func (Main) GetInfo(c *server.Context) error {
	return permissionservice.GetMainInfo(c)
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
