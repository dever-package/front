package api

import (
	"github.com/shemic/dever/server"

	uploadservice "my/package/front/service/upload"
)

type Upload struct{}

func (Upload) PostInit(c *server.Context) error {
	return uploadservice.InitUpload(c)
}

func (Upload) GetRule(c *server.Context) error {
	return uploadservice.ListUploadRules(c)
}

func (Upload) PostPart(c *server.Context) error {
	return uploadservice.UploadPart(c)
}

func (Upload) PostComplete(c *server.Context) error {
	return uploadservice.CompleteUpload(c)
}

func (Upload) PostSign(c *server.Context) error {
	return uploadservice.SignUploadOpen(c)
}

func (Upload) PostImportUrl(c *server.Context) error {
	return uploadservice.ImportURLUpload(c)
}

func (Upload) PostImportUrlStream(c *server.Context) error {
	return uploadservice.ImportURLUploadStream(c)
}

func (Upload) GetStream(c *server.Context) error {
	return uploadservice.ReadUploadStream(c)
}

func (Upload) GetOpen(c *server.Context) error {
	return uploadservice.OpenUpload(c)
}
