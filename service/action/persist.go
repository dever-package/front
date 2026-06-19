package action

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontservice "github.com/dever-package/front/service"
	actionpayload "github.com/dever-package/front/service/action/internal/payload"
	frontmeta "github.com/dever-package/front/service/meta"
	permissionservice "github.com/dever-package/front/service/permission"
	frontrecord "github.com/dever-package/front/service/record"
)

func saveModelRecord(c *server.Context, modelName string, record map[string]any, primaryKey string, upsert bool) (result map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	modelValue := load.Model(modelName)
	modelRef := reflect.ValueOf(modelValue)
	insertMethod := modelRef.MethodByName("Insert")
	updateMethod := modelRef.MethodByName("Update")
	if !insertMethod.IsValid() || !updateMethod.IsValid() {
		return nil, fmt.Errorf("model 不支持保存操作")
	}

	if strings.TrimSpace(primaryKey) == "" {
		primaryKey = "id"
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
	if len(columnLookup) == 0 {
		return nil, fmt.Errorf("model 字段解析失败")
	}

	data := frontrecord.SanitizeRecord(record, columnLookup)
	frontservice.NormalizeModelPasswordFields(modelName, data, columnLookup)

	pkColumn := frontrecord.ResolveColumnName(columnLookup, primaryKey)
	if pkColumn == "" {
		pkColumn = util.ToSnake(primaryKey)
	}

	pkValue, hasPrimaryKey := frontrecord.ReadValue(record, primaryKey)
	delete(data, pkColumn)

	if hasPrimaryKey && frontrecord.HasValue(pkValue) {
		normalizedID := actionpayload.NormalizeIdentifier(pkValue)
		if len(data) == 0 {
			if !hasVirtualModelPayload(record, primaryKey) {
				return nil, fmt.Errorf("没有可保存的字段")
			}

			if err := runModelAfterSave(c.Context(), modelName, modelValue, normalizedID, record, false); err != nil {
				return nil, err
			}

			return map[string]any{
				pkColumn:  normalizedID,
				"updated": true,
			}, nil
		}

		out := updateMethod.Call([]reflect.Value{
			reflect.ValueOf(c.Context()),
			reflect.ValueOf(map[string]any{pkColumn: normalizedID}),
			reflect.ValueOf(data),
		})
		if len(out) == 0 || util.ToInt64(out[0].Interface()) == 0 {
			if !upsert {
				return nil, fmt.Errorf("记录不存在或未更新")
			}

			if modelRecordExists(c.Context(), modelName, pkColumn, normalizedID) {
				if err := runModelAfterSave(c.Context(), modelName, modelValue, normalizedID, record, false); err != nil {
					return nil, err
				}

				return map[string]any{
					pkColumn:  normalizedID,
					"updated": true,
				}, nil
			}

			insertData := util.CloneMap(data)
			insertData[pkColumn] = normalizedID
			frontrecord.ApplyCreatedAt(insertData, columnLookup)
			if _, insertErr := callModelInsert(insertMethod, c.Context(), insertData); insertErr != nil {
				return nil, insertErr
			}

			if err := runModelAfterSave(c.Context(), modelName, modelValue, normalizedID, record, true); err != nil {
				return nil, err
			}

			return map[string]any{
				pkColumn:  normalizedID,
				"created": true,
			}, nil
		}

		if err := runModelAfterSave(c.Context(), modelName, modelValue, normalizedID, record, false); err != nil {
			return nil, err
		}

		return map[string]any{
			pkColumn:  normalizedID,
			"updated": true,
		}, nil
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("没有可保存的字段")
	}

	frontrecord.ApplyCreatedAt(data, columnLookup)
	insertID, insertErr := callModelInsert(insertMethod, c.Context(), data)
	if insertErr != nil {
		if isPrimaryKeyDuplicateError(insertErr) {
			if syncErr := permissionservice.SyncModelPrimarySequence(c.Context(), modelName); syncErr == nil {
				insertID, insertErr = callModelInsert(insertMethod, c.Context(), data)
			}
		}
		if insertErr != nil {
			return nil, insertErr
		}
	}

	if err := runModelAfterSave(c.Context(), modelName, modelValue, insertID, record, true); err != nil {
		return nil, err
	}
	return map[string]any{
		pkColumn:  insertID,
		"created": true,
	}, nil
}

func SaveModelRecord(c *server.Context, modelName string, record map[string]any, primaryKey string) (map[string]any, error) {
	return saveModelRecord(c, modelName, record, primaryKey, false)
}

func SaveModelRecordUpsert(c *server.Context, modelName string, record map[string]any, primaryKey string) (map[string]any, error) {
	return saveModelRecord(c, modelName, record, primaryKey, true)
}

func deleteModelRecord(c *server.Context, modelName string, payload any, primaryKey string) (result map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	modelValue := load.Model(modelName)
	modelRef := reflect.ValueOf(modelValue)
	deleteMethod := modelRef.MethodByName("Delete")
	if !deleteMethod.IsValid() {
		return nil, fmt.Errorf("model 不支持删除操作")
	}

	if strings.TrimSpace(primaryKey) == "" {
		primaryKey = "id"
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
	if len(columnLookup) == 0 {
		return nil, fmt.Errorf("model 字段解析失败")
	}

	pkColumn := frontrecord.ResolveColumnName(columnLookup, primaryKey)
	if pkColumn == "" {
		pkColumn = util.ToSnake(primaryKey)
	}

	filters, err := buildDeleteFilters(payload, primaryKey, pkColumn)
	if err != nil {
		return nil, err
	}

	out := deleteMethod.Call([]reflect.Value{
		reflect.ValueOf(c.Context()),
		reflect.ValueOf(filters),
	})

	deleted := util.ToInt64(out[0].Interface())
	if deleted > 0 {
		if err := runModelAfterDelete(c.Context(), modelName, modelValue, payload); err != nil {
			return nil, err
		}
	}

	return map[string]any{
		"deleted": deleted,
	}, nil
}

func runModelAfterSave(ctx context.Context, modelName string, modelValue any, id any, record map[string]any, created bool) error {
	if err := frontmeta.SaveModelRelations(ctx, modelName, id, record); err != nil {
		return err
	}

	type afterSaveHook interface {
		AfterSave(ctx context.Context, id any, record map[string]any, created bool) error
	}

	if hook, ok := modelValue.(afterSaveHook); ok {
		return hook.AfterSave(ctx, id, record, created)
	}

	return nil
}

func runModelAfterDelete(ctx context.Context, modelName string, modelValue any, payload any) error {
	if err := frontmeta.DeleteModelRelations(ctx, modelName, payload); err != nil {
		return err
	}

	type afterDeleteHook interface {
		AfterDelete(ctx context.Context, payload any) error
	}

	if hook, ok := modelValue.(afterDeleteHook); ok {
		return hook.AfterDelete(ctx, payload)
	}

	return nil
}

func modelRecordExists(ctx context.Context, modelName, pkColumn string, id any) bool {
	model := frontrecord.Resolve(modelName)
	if model == nil {
		return false
	}
	return len(model.FindMap(ctx, map[string]any{pkColumn: id})) > 0
}

func buildDeleteFilters(payload any, primaryKey, pkColumn string) (map[string]any, error) {
	switch current := payload.(type) {
	case map[string]any:
		pkValue, ok := frontrecord.ReadValue(current, primaryKey)
		if !ok || !frontrecord.HasValue(pkValue) {
			return nil, fmt.Errorf("删除缺少主键字段")
		}
		return map[string]any{
			pkColumn: actionpayload.NormalizeIdentifier(pkValue),
		}, nil
	case []any:
		ids := make([]any, 0, len(current))
		for _, item := range current {
			switch typed := item.(type) {
			case map[string]any:
				pkValue, ok := frontrecord.ReadValue(typed, primaryKey)
				if !ok || !frontrecord.HasValue(pkValue) {
					continue
				}
				ids = append(ids, actionpayload.NormalizeIdentifier(pkValue))
			default:
				if frontrecord.HasValue(typed) {
					ids = append(ids, actionpayload.NormalizeIdentifier(typed))
				}
			}
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("删除缺少主键字段")
		}
		return map[string]any{
			pkColumn: ids,
		}, nil
	default:
		if !frontrecord.HasValue(current) {
			return nil, fmt.Errorf("删除缺少主键字段")
		}
		return map[string]any{
			pkColumn: actionpayload.NormalizeIdentifier(current),
		}, nil
	}
}

func callModelInsert(insertMethod reflect.Value, ctx context.Context, data map[string]any) (result int64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	out := insertMethod.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(data),
	})
	if len(out) == 0 {
		return 0, fmt.Errorf("保存失败")
	}

	return util.ToInt64(out[0].Interface()), nil
}

func isPrimaryKeyDuplicateError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if !strings.Contains(message, "duplicate key value violates unique constraint") {
		return false
	}

	return strings.Contains(message, "_pkey") || strings.Contains(message, "primary key")
}

func hasVirtualModelPayload(record map[string]any, primaryKey string) bool {
	for key, value := range record {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(primaryKey)) {
			continue
		}
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			if len(typed) > 0 {
				return true
			}
		case []any:
			if len(typed) > 0 {
				return true
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		default:
			return true
		}
	}

	return false
}
