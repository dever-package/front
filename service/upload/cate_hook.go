package upload

import (
	"context"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type UploadFileCateHook struct{}

func (UploadFileCateHook) ProviderBeforeSaveUploadFileCate(_ *server.Context, params []any) any {
	record := cloneUploadFileCateRecord(params)
	if len(record) == 0 {
		return record
	}

	record["name"] = util.ToStringTrimmed(record["name"])
	if sortValue, ok := record["sort"]; ok && util.ToIntDefault(sortValue, 0) <= 0 {
		record["sort"] = 100
	}
	if _, ok := record["created_at"]; !ok {
		record["created_at"] = time.Now()
	}
	return record
}

func (UploadFileCateHook) ProviderBeforeDeleteUploadFileCate(_ *server.Context, params []any) any {
	payload := cloneUploadFileCateRecord(params)
	categoryID := util.ToUint64(payload["raw_id"])
	if categoryID == 0 {
		categoryID = util.ToUint64(payload["id"])
	}
	if categoryID == 0 {
		panic("分类不存在")
	}

	fileModel, err := uploadrepo.ResolveFileModel()
	if err != nil {
		panic(err.Error())
	}
	if fileModel.Count(context.Background(), map[string]any{"category_id": categoryID}) > 0 {
		panic("当前分类下仍有资源，请先转移或清空后再删除")
	}

	return map[string]any{"id": categoryID}
}

func cloneUploadFileCateRecord(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	record, _ := params[0].(map[string]any)
	if record == nil {
		return map[string]any{}
	}
	return util.CloneMap(record)
}
