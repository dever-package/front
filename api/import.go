package api

import (
	"github.com/shemic/dever/server"

	importerservice "github.com/dever-package/front/service/importer"
	operationlog "github.com/dever-package/front/service/operationlog"
)

type Import struct{}

func (Import) PostAnalyze(c *server.Context) error {
	pagePath := c.Input("path")
	importKey := c.Input("importKey")
	err := importerservice.Analyze(c)
	if err == nil {
		recordImportOperation(c, pagePath, importKey, "分析导入文件")
	}
	return err
}

func (Import) PostTaskCreate(c *server.Context) error {
	pagePath := c.Input("path")
	importKey := c.Input("importKey")
	err := importerservice.CreateTask(c)
	if err == nil {
		recordImportOperation(c, pagePath, importKey, "创建导入任务")
	}
	return err
}

func (Import) GetTaskInfo(c *server.Context) error {
	return importerservice.GetTaskInfo(c)
}

func recordImportOperation(c *server.Context, pagePath string, importKey string, message string) {
	operationlog.Record(c, operationlog.Entry{
		Action:   "import",
		PagePath: pagePath,
		TargetID: importKey,
		Message:  message,
		Payload: map[string]any{
			"path":      pagePath,
			"importKey": importKey,
			"fileId":    c.Input("fileId"),
			"sheetName": c.Input("sheetName"),
		},
	})
}
