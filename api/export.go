package api

import (
	"github.com/shemic/dever/server"

	exportservice "github.com/dever-package/front/service/export"
	operationlog "github.com/dever-package/front/service/operationlog"
)

type Export struct{}

func (Export) PostTaskCreate(c *server.Context) error {
	pagePath := c.Input("path")
	exportKey := c.Input("exportKey")
	err := exportservice.CreateTask(c)
	if err == nil {
		operationlog.Record(c, operationlog.Entry{
			Action:   "export",
			PagePath: pagePath,
			TargetID: exportKey,
			Message:  "创建导出任务",
			Payload: map[string]any{
				"path":      pagePath,
				"table_id":  c.Input("tableId"),
				"exportKey": exportKey,
			},
		})
	}
	return err
}

func (Export) GetTaskInfo(c *server.Context) error {
	return exportservice.GetTaskInfo(c)
}

func (Export) GetDownload(c *server.Context) error {
	return exportservice.DownloadTask(c)
}
