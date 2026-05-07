package action

import (
	"fmt"

	"github.com/shemic/dever/server"

	actionvalidate "my/package/front/service/action/validate"
	frontpage "my/package/front/service/page"
	permissionservice "my/package/front/service/permission"
	frontrecord "my/package/front/service/record"
)

func PostAction(c *server.Context) error {
	requestPath := frontpage.NormalizePath(c.Input("path", "required", "页面路径"))

	var request frontpage.ActionRequest
	if err := c.BindJSON(&request); err != nil {
		return c.Error("请求体格式错误")
	}
	if request.Action == nil {
		return c.Error("action 不能为空")
	}

	config := frontpage.NormalizeAction(*request.Action)
	pathValue := frontpage.ActionPath(requestPath, config)
	if pathValue == "" {
		return c.Error("页面路径不能为空")
	}

	if config.Type == "delete" {
		protected, err := permissionservice.CheckActionAccess(c.Context(), requestPath, config.Key)
		if err != nil {
			return respondPermissionDenied(c, err)
		}
		if !protected {
			if err := ensurePageActionAccess(c, pathValue, request.Payload); err != nil {
				return respondPermissionDenied(c, err)
			}
		}
	} else {
		if err := ensurePageActionAccess(c, pathValue, request.Payload); err != nil {
			return respondPermissionDenied(c, err)
		}
	}

	content, err := frontpage.ReadContent(pathValue)
	if err != nil {
		return c.Error(err)
	}

	switch config.Type {
	case "save":
		payload, ok := normalizeFormPayload(request.Payload)
		if !ok {
			return c.Error("保存参数格式错误")
		}

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
		return c.JSON(result)
	case "delete":
		result, err := runDelete(c, pathValue, config, request.Payload)
		if err != nil {
			if failures, ok := FieldFailures(err); ok {
				return actionvalidate.RespondError(c, failures)
			}
			return c.Error(err)
		}
		return c.JSON(result)
	default:
		return c.Error(fmt.Sprintf("action.type 不支持: %s", config.Type))
	}
}

func ensurePageActionAccess(c *server.Context, pathValue string, payload any) error {
	return permissionservice.EnsurePageAccessWithInput(
		c.Context(),
		pathValue,
		permissionservice.PayloadInputLookup(payload, func(key string) string {
			return c.Input(key)
		}),
	)
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

	resolvedPayload, err := resolveSavePayload(config, sourceMap)
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
	resolvedPayload := resolveDeletePayload(config, sourcePayload)

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

func extractResultID(result map[string]any, primaryKey string) any {
	if value, ok := frontrecord.ReadValue(result, primaryKey); ok && frontrecord.HasValue(value) {
		return value
	}
	if value, ok := frontrecord.ReadValue(result, "id"); ok && frontrecord.HasValue(value) {
		return value
	}
	return nil
}
