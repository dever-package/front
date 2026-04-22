package page

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontcall "github.com/dever-package/front/service/call"
	frontmeta "github.com/dever-package/front/service/meta"
	frontrecord "github.com/dever-package/front/service/record"
)

func resolveDataValue(
	c *server.Context,
	key string,
	value any,
	collectedOptions map[string]any,
	pathValue string,
) (any, error) {
	switch current := value.(type) {
	case []any:
		items := make([]any, 0, len(current))
		for _, item := range current {
			resolved, err := resolveDataValue(c, key, item, collectedOptions, pathValue)
			if err != nil {
				return nil, err
			}
			items = append(items, resolved)
		}
		return items, nil
	case map[string]any:
		if key == "search" {
			current = syncQueryValues(c, current)
		}
		if resolved, options, ok, err := resolveModelFormContainer(c, key, current, pathValue); ok {
			frontmeta.MergeOptionMap(collectedOptions, options)
			return resolved, err
		}
		if resolved, options, ok, err := resolveModelListContainer(c, current, pathValue); ok {
			frontmeta.MergeOptionMap(collectedOptions, options)
			return resolved, err
		}
		result := make(map[string]any, len(current))
		for childKey, item := range current {
			resolved, err := resolveDataValue(c, childKey, item, collectedOptions, pathValue)
			if err != nil {
				return nil, err
			}
			result[childKey] = resolved
		}
		return result, nil
	case string:
		return resolveDataPlaceholder(c, key, current), nil
	default:
		return value, nil
	}
}

func resolveDataPlaceholder(c *server.Context, key, value string) any {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"))
		if name == "" {
			return value
		}
		return load.Service(name, c)
	}
	if strings.HasPrefix(trimmed, "<<") && strings.HasSuffix(trimmed, ">>") {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "<<"), ">>"))
		if name == "" {
			return value
		}
		return frontrecord.LoadSafe(name)
	}
	return value
}

func syncQueryValues(c *server.Context, current map[string]any) map[string]any {
	result := make(map[string]any, len(current))
	for key, value := range current {
		result[key] = value
		input := normalizeQueryInputText(c.Input(key))
		if input == "" {
			continue
		}
		switch value.(type) {
		case string, int, int8, int16, int32, int64, float32, float64:
			result[key] = input
		case []any, []string:
			parsed := normalizeQueryFilterAny(input)
			if hasMeaningfulQueryFilterValue(parsed) {
				result[key] = parsed
			}
		}
	}
	return result
}

func resolveModelListContainer(
	c *server.Context,
	current map[string]any,
	pathValue string,
) (any, map[string]any, bool, error) {
	modelName, ok := resolveListModelName(current, pathValue)
	if !ok {
		return nil, nil, false, nil
	}

	modelValue := frontrecord.LoadSafe(modelName)
	if modelValue == nil {
		return nil, nil, true, fmt.Errorf("model 未注册")
	}
	options := resolveModelFrontOption(c.Context(), modelName, modelValue)
	queryConfig := util.CloneMap(current)
	queryConfig["modelName"] = modelName
	rows, total, page, pageSize, err := queryModelList(c, modelValue, queryConfig)
	if err != nil {
		return nil, options, true, err
	}
	rows = frontmeta.AttachRelations(c.Context(), modelName, rows)
	rows = frontmeta.HideFields(modelName, rows)
	rows, err = applyListRowService(c, current, pathValue, rows)
	if err != nil {
		return nil, options, true, err
	}

	result := make(map[string]any, len(current)+2)
	for key, value := range current {
		switch key {
		case "list":
			result[key] = rows
		case "searchFields", "order", "modelName":
			continue
		default:
			result[key] = value
		}
	}
	result["list"] = rows
	result["total"] = total
	result["page"] = page
	result["pageSize"] = pageSize
	return result, options, true, nil
}

func applyListRowService(
	c *server.Context,
	current map[string]any,
	pathValue string,
	rows []map[string]any,
) ([]map[string]any, error) {
	serviceName := util.ToStringTrimmed(current["service"])
	if serviceName == "" || len(rows) == 0 {
		return rows, nil
	}

	result, err := frontcall.Service(c, serviceName, map[string]any{
		"path":      pathValue,
		"container": util.CloneMap(current),
		"rows":      rows,
	})
	if err != nil {
		return nil, err
	}

	if normalized := normalizeServiceRowMaps(result); normalized != nil {
		return normalized, nil
	}
	if mapped, ok := result.(map[string]any); ok {
		if normalized := normalizeServiceRowMaps(mapped["rows"]); normalized != nil {
			return normalized, nil
		}
	}
	return rows, nil
}

func normalizeServiceRowMaps(value any) []map[string]any {
	switch current := value.(type) {
	case []map[string]any:
		return current
	case []any:
		rows := make([]map[string]any, 0, len(current))
		for _, item := range current {
			mapped, ok := item.(map[string]any)
			if !ok || mapped == nil {
				continue
			}
			rows = append(rows, mapped)
		}
		return rows
	default:
		return nil
	}
}

func resolveModelFormContainer(
	c *server.Context,
	key string,
	current map[string]any,
	pathValue string,
) (any, map[string]any, bool, error) {
	modelName, ok := resolveFormModelName(key, current, pathValue)
	if !ok {
		return nil, nil, false, nil
	}

	modelValue := frontrecord.LoadSafe(modelName)
	if modelValue == nil {
		return nil, nil, true, fmt.Errorf("model 未注册")
	}
	options := resolveModelFrontOption(c.Context(), modelName, modelValue)
	form := syncQueryValues(c, util.CloneMap(current))

	recordID, hasRecordID := queryRecordID(c, "id")
	recordIDFromTemplate := false
	if !hasRecordID {
		recordID, hasRecordID = formRecordID(current, "id")
		recordIDFromTemplate = hasRecordID
	}
	if !hasRecordID {
		return cleanFormMetaFields(form), options, true, nil
	}

	record, found, err := queryModelRecord(c.Context(), modelName, recordID)
	if err != nil {
		return nil, options, true, err
	}
	if !found {
		if recordIDFromTemplate {
			return cleanFormMetaFields(form), options, true, nil
		}
		return nil, options, true, fmt.Errorf("记录不存在")
	}

	return mergeFormRecord(current, form, record), options, true, nil
}

func resolveFormModelName(
	key string,
	current map[string]any,
	pathValue string,
) (string, bool) {
	if strings.TrimSpace(key) != "form" {
		return "", false
	}

	if modelName := explicitFormModelName(current); modelName != "" {
		return modelName, true
	}

	if !shouldUseDefaultFormModel(key, current, pathValue) {
		return "", false
	}

	modelName := DefaultModelName(pathValue)
	if modelName == "" {
		return "", false
	}
	return modelName, true
}

func explicitFormModelName(current map[string]any) string {
	// JSON 表单内部字段：声明当前 form 应按哪个 model 自动加载，避免为固定单页写专用 service。
	for _, key := range []string{"_model", "_use"} {
		modelName := util.ToStringTrimmed(current[key])
		if modelName != "" {
			return modelName
		}
	}
	return ""
}

func shouldUseDefaultFormModel(
	key string,
	current map[string]any,
	pathValue string,
) bool {
	if strings.TrimSpace(key) != "form" {
		return false
	}

	switch {
	case strings.HasSuffix(NormalizePath(pathValue), "/update"):
		return true
	case strings.HasSuffix(NormalizePath(pathValue), "/create"):
		return true
	case strings.HasSuffix(NormalizePath(pathValue), "/view"):
		return true
	case strings.HasSuffix(NormalizePath(pathValue), "/detail"):
		return true
	case strings.HasSuffix(NormalizePath(pathValue), "/info"):
		return true
	default:
		return false
	}
}

func queryModelRecord(ctx context.Context, modelName string, recordID uint64) (map[string]any, bool, error) {
	modelValue := frontrecord.Resolve(modelName)
	if modelValue == nil {
		return nil, false, fmt.Errorf("model 不支持详情查询")
	}

	row := modelValue.FindMap(ctx, map[string]any{"id": recordID})
	if len(row) == 0 {
		return nil, false, nil
	}

	rows := frontmeta.AttachRelations(ctx, modelName, []map[string]any{row})
	rows = frontmeta.HideFields(modelName, rows)
	return rows[0], true, nil
}

func mergeFormRecord(
	template map[string]any,
	defaults map[string]any,
	record map[string]any,
) map[string]any {
	if len(template) == 0 {
		return util.CloneMap(record)
	}

	result := util.CloneMap(defaults)
	for key := range template {
		if isFormMetaField(key) {
			continue
		}
		if value, ok := frontrecord.ReadValue(record, key); ok {
			result[key] = value
		}
		copyRelationCompanionValue(result, record, key)
	}
	return cleanFormMetaFields(result)
}

func formRecordID(current map[string]any, key string) (uint64, bool) {
	value, ok := frontrecord.ReadValue(current, key)
	if !ok {
		return 0, false
	}

	number := util.ToInt64(value)
	if number <= 0 {
		return 0, false
	}
	return uint64(number), true
}

func cleanFormMetaFields(values map[string]any) map[string]any {
	result := util.CloneMap(values)
	for key := range result {
		if isFormMetaField(key) {
			delete(result, key)
		}
	}
	return result
}

func isFormMetaField(key string) bool {
	switch strings.TrimSpace(key) {
	case "_model", "_use":
		return true
	default:
		return false
	}
}

func copyRelationCompanionValue(target map[string]any, record map[string]any, key string) {
	companionKey := resolveRelationCompanionKey(key)
	if companionKey == "" {
		return
	}
	if _, exists := target[companionKey]; exists {
		return
	}

	value, ok := frontrecord.ReadValue(record, companionKey)
	if !ok {
		return
	}
	target[companionKey] = value
}

func resolveRelationCompanionKey(key string) string {
	normalizedKey := strings.TrimSpace(key)
	switch {
	case strings.HasSuffix(normalizedKey, "_ids"):
		return strings.TrimSuffix(normalizedKey, "_ids") + "s"
	case strings.HasSuffix(normalizedKey, "_id"):
		return strings.TrimSuffix(normalizedKey, "_id")
	default:
		return ""
	}
}

func resolveModelFrontOption(ctx context.Context, modelName string, modelValue any) map[string]any {
	if modelValue == nil {
		return nil
	}

	if options := frontmeta.ResolveModelOptions(ctx, modelName); len(options) > 0 {
		return options
	}

	type optionProvider interface {
		FrontOption() map[string]any
	}

	if provider, ok := modelValue.(optionProvider); ok {
		return provider.FrontOption()
	}

	method := reflect.ValueOf(modelValue).MethodByName("FrontOption")
	if !method.IsValid() {
		return nil
	}

	out := method.Call(nil)
	if len(out) == 0 || out[0].IsNil() {
		return nil
	}

	options, _ := out[0].Interface().(map[string]any)
	return options
}

func resolveSubmitModelFrontOption(content []byte, pathValue string) map[string]any {
	modelName := strings.TrimSpace(SubmitModelName(content, pathValue))
	if modelName == "" {
		return nil
	}

	return resolveModelFrontOption(context.Background(), modelName, frontrecord.LoadSafe(modelName))
}

func parseModelPlaceholder(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "<<") || !strings.HasSuffix(trimmed, ">>") {
		return "", false
	}

	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "<<"), ">>"))
	if name == "" {
		return "", false
	}
	return name, true
}

func resolveListModelName(
	current map[string]any,
	pathValue string,
) (string, bool) {
	if rawList, exists := current["list"]; exists {
		if text, ok := rawList.(string); ok {
			if modelName, ok := parseModelPlaceholder(text); ok {
				return modelName, true
			}
			if strings.TrimSpace(text) != "" {
				return "", false
			}
		} else if rawList != nil {
			return "", false
		}
	}

	if !shouldUseDefaultListModel(pathValue, current) {
		return "", false
	}

	modelName := DefaultModelName(pathValue)
	if modelName == "" {
		return "", false
	}
	return modelName, true
}

func shouldUseDefaultListModel(pathValue string, current map[string]any) bool {
	if !strings.HasSuffix(NormalizePath(pathValue), "/list") {
		return false
	}
	if _, ok := current["page"]; !ok {
		return false
	}
	if _, ok := current["pageSize"]; !ok {
		return false
	}
	if _, ok := current["total"]; !ok {
		return false
	}
	rawList, exists := current["list"]
	return !exists || rawList == nil
}
