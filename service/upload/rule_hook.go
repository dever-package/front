package upload

import (
	"context"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontaction "github.com/dever-package/front/service/action"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type UploadRuleHook struct{}

func (UploadRuleHook) ProviderBeforeSaveUploadRule(c *server.Context, params []any) any {
	record := cloneUploadHookRecord(params)
	transport := strings.ToLower(util.ToStringTrimmed(record["transport"]))
	if transport != "direct" {
		return record
	}

	storageID := util.ToUint64(record["storage_id"])
	ctx := context.Background()
	if c != nil {
		ctx = c.Context()
	}
	storage, err := uploadrepo.FindUploadStorage(ctx, storageID)
	if err != nil {
		panic(frontaction.NewFieldError("form.storage_id", "存储方式不存在。"))
	}
	if err := validateUploadTransport(storage.Type, transport); err != nil {
		panic(frontaction.NewFieldError("form.transport", err.Error()))
	}
	record["transport"] = transport
	return record
}
