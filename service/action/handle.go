package action

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shemic/dever/server"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	actionvalidate "github.com/dever-package/front/service/action/validate"
	operationlog "github.com/dever-package/front/service/operationlog"
	frontpage "github.com/dever-package/front/service/page"
	permissionservice "github.com/dever-package/front/service/permission"
	frontrecord "github.com/dever-package/front/service/record"
	"github.com/dever-package/front/service/runtimecache"
)

func PostAction(c *server.Context) error {
	requestPath := frontpagepath.NormalizePath(c.Input("path", "required", "页面路径"))

	var request frontpage.ActionRequest
	if err := c.BindJSON(&request); err != nil {
		return c.Error("请求体格式错误")
	}

	resolved, err := frontpage.ResolveAction(frontpage.ResolveActionInput{
		Context: c.Context(),
		Path:    requestPath,
		Key:     request.Key,
		Payload: request.Payload,
	})
	if err != nil {
		return c.Error(err)
	}
	config := resolved.Config
	pathValue := resolved.Path

	if config.Type == "delete" {
		if err := permissionservice.EnsureActionAccessWithInput(c.Context(), requestPath, resolved.Key, actionInputLookup(c, request.Payload)); err != nil {
			return respondPermissionDenied(c, err)
		}
	} else {
		if err := ensurePageActionAccess(c, pathValue, request.Payload); err != nil {
			return respondPermissionDenied(c, err)
		}
	}

	content := resolved.Content
	modelName := resolved.Model
	primaryKey := resolved.PrimaryKey

	switch config.Type {
	case "save":
		payload, ok := normalizeFormPayload(request.Payload)
		if !ok {
			return c.Error("保存参数格式错误")
		}
		payload = applyRoutePrimaryKey(c, config, payload)

		failures, err := actionvalidate.Form(c, content, pathValue, payload)
		if err != nil {
			return c.Error(err)
		}
		if len(failures) > 0 {
			return actionvalidate.RespondError(c, failures)
		}

		result, err := runSave(c, pathValue, config, payload)
		if err != nil {
			if failures, ok := FieldFailures(err); ok {
				return actionvalidate.RespondError(c, failures)
			}
			return c.Error(err)
		}
		if resultMap, ok := result.(map[string]any); ok {
			operationlog.RecordAction(c, requestPath, pathValue, modelName, config.Type, primaryKey, payload, resultMap)
		}
		runtimecache.Invalidate()
		return c.JSON(result)
	case "delete":
		result, err := runDelete(c, pathValue, config, request.Payload)
		if err != nil {
			if failures, ok := FieldFailures(err); ok {
				return actionvalidate.RespondError(c, failures)
			}
			return c.Error(err)
		}
		if resultMap, ok := result.(map[string]any); ok {
			operationlog.RecordAction(c, requestPath, pathValue, modelName, config.Type, primaryKey, request.Payload, resultMap)
		}
		runtimecache.Invalidate()
		return c.JSON(result)
	default:
		return c.Error(fmt.Sprintf("action.type 不支持: %s", config.Type))
	}
}

func ensurePageActionAccess(c *server.Context, pathValue string, payload any) error {
	return permissionservice.EnsurePageAccessWithInput(
		c.Context(),
		pathValue,
		actionInputLookup(c, payload),
	)
}

func actionInputLookup(c *server.Context, payload any) permissionservice.InputLookup {
	return permissionservice.PayloadInputLookup(payload, func(key string) string {
		return c.Input(key)
	})
}

func respondPermissionDenied(c *server.Context, err error) error {
	return c.JSONPayload(403, map[string]any{
		"code":   403,
		"status": 2,
		"msg":    err.Error(),
		"data":   nil,
	})
}

func runSave(c *server.Context, pathValue string, config frontpage.ActionConfig, form map[string]any) (any, error) {
	sourcePayload, err := runBeforeHooks(c, config.Before, form)
	if err != nil {
		return nil, err
	}

	sourceMap, ok := sourcePayload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("action.save before 必须返回对象")
	}

	resolvedPayload, err := resolveSavePayload(c.Context(), config, sourceMap)
	if err != nil {
		return nil, err
	}

	record, ok := resolvedPayload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("action.save 参数必须返回对象")
	}

	modelName := frontpage.ActionModelName(pathValue, config)
	result, err := saveModelRecord(c, modelName, record, frontpage.ActionPrimaryKey(config), config.Upsert)
	if err != nil {
		return nil, err
	}

	if err := runAfterHooks(c, config.After, map[string]any{
		"payload": sourcePayload,
		"data":    record,
		"result":  result,
		"id":      extractResultID(result, frontpage.ActionPrimaryKey(config)),
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func runDelete(c *server.Context, pathValue string, config frontpage.ActionConfig, payload any) (any, error) {
	sourcePayload, err := runBeforeHooks(c, config.Before, payload)
	if err != nil {
		return nil, err
	}
	resolvedPayload := resolveDeletePayload(c.Context(), config, sourcePayload)

	modelName := frontpage.ActionModelName(pathValue, config)
	result, err := deleteModelRecord(c, modelName, resolvedPayload, frontpage.ActionPrimaryKey(config))
	if err != nil {
		return nil, err
	}

	if err := runAfterHooks(c, config.After, map[string]any{
		"payload": sourcePayload,
		"data":    resolvedPayload,
		"result":  result,
		"id":      extractResultID(result, frontpage.ActionPrimaryKey(config)),
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func normalizeFormPayload(payload any) (map[string]any, bool) {
	if payload == nil {
		return map[string]any{}, true
	}
	values, ok := payload.(map[string]any)
	return values, ok
}

func applyRoutePrimaryKey(c *server.Context, config frontpage.ActionConfig, payload map[string]any) map[string]any {
	primaryKey := strings.TrimSpace(frontpage.ActionPrimaryKey(config))
	if primaryKey == "" {
		primaryKey = "id"
	}
	if value, ok := frontrecord.ReadValue(payload, primaryKey); ok && frontrecord.HasValue(value) {
		return payload
	}
	value, ok := readRoutePrimaryKey(c, primaryKey)
	if !ok {
		return payload
	}

	next := make(map[string]any, len(payload)+1)
	for key, current := range payload {
		next[key] = current
	}
	next[primaryKey] = value
	return next
}

func readRoutePrimaryKey(c *server.Context, primaryKey string) (int64, bool) {
	keys := []string{primaryKey}
	if primaryKey != "id" {
		keys = append(keys, "id")
	}
	for _, key := range keys {
		value := strings.TrimSpace(c.Input(key))
		if value == "" {
			continue
		}
		number, err := strconv.ParseInt(value, 10, 64)
		if err == nil && number > 0 {
			return number, true
		}
	}
	return 0, false
}

func extractResultID(result map[string]any, primaryKey string) any {
	if value, ok := frontrecord.ReadValue(result, primaryKey); ok && frontrecord.HasValue(value) {
		return value
	}
	if value, ok := frontrecord.ReadValue(result, "id"); ok && frontrecord.HasValue(value) {
		return value
	}
	return nil
}
