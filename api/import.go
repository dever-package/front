package api

import (
	"github.com/shemic/dever/server"

	importerservice "github.com/dever-package/front/service/importer"
)

type Import struct{}

func (Import) PostAnalyze(c *server.Context) error {
	return importerservice.Analyze(c)
}

func (Import) PostTaskCreate(c *server.Context) error {
	return importerservice.CreateTask(c)
}

func (Import) GetTaskInfo(c *server.Context) error {
	return importerservice.GetTaskInfo(c)
}
