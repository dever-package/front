package api

import (
	"github.com/shemic/dever/server"

	exportservice "my/package/front/service/export"
)

type Export struct{}

func (Export) PostTaskCreate(c *server.Context) error {
	return exportservice.CreateTask(c)
}

func (Export) GetTaskInfo(c *server.Context) error {
	return exportservice.GetTaskInfo(c)
}

func (Export) GetDownload(c *server.Context) error {
	return exportservice.DownloadTask(c)
}
