package upload

import (
	"context"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontaction "github.com/dever-package/front/service/action"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type UploadStorageHook struct{}

func (UploadStorageHook) ProviderAttachUploadStorageList(_ *server.Context, params []any) any {
	payload := cloneUploadHookRecord(params)
	rows := cloneUploadStorageRows(payload["rows"])
	for _, row := range rows {
		if strings.EqualFold(util.ToStringTrimmed(row["type"]), "feishu") {
			delete(row, "secret_key")
		}
	}
	return rows
}

func (UploadStorageHook) ProviderAttachUploadStorageForm(_ *server.Context, params []any) any {
	payload := cloneUploadHookRecord(params)
	record := payload
	if loaded, ok := payload["record"].(map[string]any); ok {
		record = util.CloneMap(loaded)
	}
	if strings.EqualFold(util.ToStringTrimmed(record["type"]), "feishu") {
		record["secret_key"] = ""
	}
	return record
}

func (UploadStorageHook) ProviderBeforeSaveUploadStorage(c *server.Context, params []any) any {
	record := cloneUploadHookRecord(params)
	if len(record) == 0 {
		return record
	}
	storageType := strings.ToLower(util.ToStringTrimmed(record["type"]))
	existing := ensureUploadStorageTypeUnchanged(c, record, storageType)
	if storageType != "feishu" {
		return record
	}

	trimUploadStorageField(record, "name")
	trimUploadStorageField(record, "access_key")
	trimUploadStorageField(record, "secret_key")
	trimUploadStorageField(record, "bucket")
	record["type"] = storageType
	record["domain"] = ""
	record["upload_host"] = ""

	if util.ToStringTrimmed(record["name"]) == "" {
		panic(frontaction.NewFieldError("form.name", "存储方式名称不能为空。"))
	}

	preserveUploadStorageSecret(record, storageType, existing)
	requireUploadStorageField(record, "access_key", "form.access_key", "飞书 App ID 不能为空。")
	requireUploadStorageField(record, "secret_key", "form.secret_key", "飞书 App Secret 不能为空。")
	requireUploadStorageField(record, "bucket", "form.bucket", "飞书目标文件夹 Token 不能为空。")
	return record
}

func ensureUploadStorageTypeUnchanged(
	c *server.Context,
	record map[string]any,
	storageType string,
) uploadrepo.UploadStorage {
	storageID := util.ToUint64(record["id"])
	if storageID == 0 {
		return uploadrepo.UploadStorage{}
	}
	ctx := context.Background()
	if c != nil {
		ctx = c.Context()
	}
	existing, err := uploadrepo.FindUploadStorage(ctx, storageID)
	if err != nil {
		panic(frontaction.NewFieldError("form.type", "存储方式不存在。"))
	}
	if !strings.EqualFold(strings.TrimSpace(existing.Type), storageType) {
		panic(frontaction.NewFieldError("form.type", "已有存储方式不能修改存储类型，请新建存储方式。"))
	}
	return existing
}

func preserveUploadStorageSecret(
	record map[string]any,
	storageType string,
	existing uploadrepo.UploadStorage,
) {
	if util.ToStringTrimmed(record["secret_key"]) != "" {
		return
	}
	if existing.ID == 0 {
		return
	}
	if strings.EqualFold(strings.TrimSpace(existing.Type), storageType) {
		record["secret_key"] = existing.SecretKey
	}
}

func requireUploadStorageField(record map[string]any, field, formPath, message string) {
	if util.ToStringTrimmed(record[field]) == "" {
		panic(frontaction.NewFieldError(formPath, message))
	}
}

func trimUploadStorageField(record map[string]any, field string) {
	if _, exists := record[field]; exists {
		record[field] = util.ToStringTrimmed(record[field])
	}
}

func cloneUploadHookRecord(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	record, _ := params[0].(map[string]any)
	if record == nil {
		return map[string]any{}
	}
	return util.CloneMap(record)
}

func cloneUploadStorageRows(value any) []map[string]any {
	result := make([]map[string]any, 0)
	switch rows := value.(type) {
	case []any:
		for _, item := range rows {
			if row, ok := item.(map[string]any); ok {
				result = append(result, util.CloneMap(row))
			}
		}
	case []map[string]any:
		for _, row := range rows {
			result = append(result, util.CloneMap(row))
		}
	}
	return result
}
