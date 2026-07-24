package api

import (
	"github.com/shemic/dever/server"

	uploadservice "github.com/dever-package/front/service/upload"
)

type UploadStorage struct{}

func (UploadStorage) PostCheck(c *server.Context) error {
	return uploadservice.CheckUploadStorageConnection(c)
}
