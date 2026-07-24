package upload

import (
	"net/http"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	permissionservice "github.com/dever-package/front/service/permission"
	uploadprovider "github.com/dever-package/front/service/upload/provider"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

func CheckUploadStorageConnection(c *server.Context) error {
	storageIDInput := c.Input("id")
	if err := permissionservice.EnsurePageAccessWithInput(
		c.Context(),
		"front/upload_storage/update",
		permissionservice.PayloadInputLookup(map[string]any{"id": storageIDInput}, nil),
	); err != nil {
		return c.Error(err, http.StatusForbidden)
	}
	storageID := util.ToUint64(c.Input("id", "required", "存储方式"))
	storage, err := uploadrepo.FindUploadStorage(c.Context(), storageID)
	if err != nil {
		return c.Error(err)
	}
	if err = uploadprovider.CheckStorage(c.Context(), storage); err != nil {
		return c.Error(err)
	}
	return c.JSON(map[string]any{
		"id":      storage.ID,
		"type":    storage.Type,
		"message": "飞书认证和目标文件夹读取正常",
	})
}
