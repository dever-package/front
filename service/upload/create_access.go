package upload

import (
	"strconv"

	"github.com/shemic/dever/server"

	uploadaccess "github.com/dever-package/front/service/upload/access"
)

type uploadCreateAccessInput struct {
	BizKey     string
	Kind       string
	CategoryID uint64
}

func requireUploadCreateAccess(c *server.Context, input uploadCreateAccessInput) error {
	if err := uploadaccess.EnsureResourceRequest(c, uploadaccess.Request{
		Operation:  uploadaccess.OperationCreate,
		BizKey:     input.BizKey,
		Kind:       input.Kind,
		CategoryID: uploadCreateCategoryID(input.CategoryID),
	}); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}
	return nil
}

func uploadCreateCategoryID(categoryID uint64) string {
	if categoryID == 0 {
		return ""
	}
	return strconv.FormatUint(categoryID, 10)
}
