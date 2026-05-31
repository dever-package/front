package option

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontcall "my/package/front/service/internal/call"
	embedpageservice "my/package/front/service/permission/embedpage"
	frontrecord "my/package/front/service/record"
	"my/package/front/service/siteconfig"
)

func Get(c *server.Context) error {
	optionType := strings.ToLower(strings.TrimSpace(c.Input("type")))
	if optionType == "" {
		optionType = "model"
	}

	switch optionType {
	case "model":
		items, err := getModelOptions(c)
		if err != nil {
			return c.Error(err)
		}
		return c.JSON(items)
	case "service":
		items, err := getServiceOptions(c)
		if err != nil {
			return c.Error(err)
		}
		return c.JSON(items)
	default:
		return c.Error(fmt.Errorf("option.type 不支持: %s", optionType))
	}
}

func getModelOptions(c *server.Context) ([]map[string]any, error) {
	return GetModelOptionsByInput(c.Context(), func(key string) string {
		return c.Input(key)
	})
}

func GetModelOptionsByInput(ctx context.Context, getInput func(string) string) ([]map[string]any, error) {
	modelName := strings.TrimSpace(getInput("use"))
	if modelName == "" {
		return nil, fmt.Errorf("模型名不能为空")
	}

	modelValue := resolveOptionModel(modelName)
	if modelValue == nil {
		return nil, fmt.Errorf("model 不支持 SelectMap")
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
	parentField := frontrecord.ResolveColumnName(columnLookup, util.FirstNonEmpty(getInput("parentField"), "parent_id"))
	valueField := frontrecord.ResolveColumnName(columnLookup, util.FirstNonEmpty(getInput("valueField"), "id"))
	if valueField == "" {
		valueField = "id"
	}
	labelField := frontrecord.ResolveColumnName(columnLookup, util.FirstNonEmpty(getInput("labelField"), "name"))
	if labelField == "" {
		labelField = "name"
	}
	leafField := frontrecord.ResolveColumnName(columnLookup, strings.TrimSpace(getInput("leafField")))
	extraFields := resolveOptionExtraFields(
		columnLookup,
		getInput("extraFields"),
		valueField,
		labelField,
		parentField,
		leafField,
	)
	treeMode := util.ToBool(getInput("tree"))

	parentValue := strings.TrimSpace(getInput("parentId"))
	if parentValue == "" {
		parentValue = strings.TrimSpace(getInput("rootValue"))
	}

	filters := map[string]any{}
	if !treeMode && parentField != "" {
		filters[parentField] = normalizeOptionFilterValue(parentValue)
	}
	applyModelOptionFieldFilter(
		filters,
		columnLookup,
		getInput("filterField"),
		getInput("filterValue"),
	)
	queryFilters := buildModelOptionQueryFilters(
		filters,
		columnLookup,
		strings.TrimSpace(getInput("keyword")),
		getInput("searchFields"),
		labelField,
	)

	fields := buildModelOptionSelectFields(valueField, labelField, parentField, leafField, extraFields)

	options := map[string]any{
		"field": strings.Join(fields, ", "),
	}
	if order := strings.TrimSpace(getInput("order")); order != "" {
		options["order"] = order
	}
	if !treeMode {
		if pageSize := util.ToIntDefault(getInput("pageSize"), 0); pageSize > 0 {
			options["pageSize"] = pageSize
			if page := util.ToIntDefault(getInput("page"), 1); page > 0 {
				options["page"] = page
			}
		}
	}

	rows := modelValue.SelectMap(ctx, queryFilters, options)
	rows = mergeSelectedOptionRows(
		ctx,
		modelValue,
		rows,
		valueField,
		fields,
		getInput("selected"),
	)
	if modelName == "front.NewAuthModel" {
		rows = embedpageservice.FilterRowsForPage(siteconfig.PageFromContext(ctx), rows)
	}
	if len(rows) == 0 {
		if parentField == "" || treeMode {
			rows = SeedRowsByField(modelName, parentField, []any{})
		} else {
			rows = SeedRows(modelName, parentField, filters[parentField])
		}
	}

	items := normalizeOptionRows(rows, valueField, labelField, leafField)
	if treeMode && parentField != "" {
		rootValue := normalizeOptionFilterValue(util.FirstNonEmpty(getInput("rootValue"), "0"))
		return buildTreeOptionRows(items, parentField, rootValue), nil
	}
	return items, nil
}

func mergeSelectedOptionRows(
	ctx context.Context,
	modelValue optionModel,
	rows []map[string]any,
	valueField string,
	fields []string,
	rawSelected string,
) []map[string]any {
	selectedValues := normalizeSelectedOptionValues(rawSelected)
	if len(selectedValues) == 0 {
		return rows
	}

	selectedRows := modelValue.SelectMap(
		ctx,
		map[string]any{valueField: selectedValues},
		map[string]any{"field": strings.Join(fields, ", ")},
	)
	if len(selectedRows) == 0 {
		return rows
	}

	seen := map[string]struct{}{}
	result := make([]map[string]any, 0, len(selectedRows)+len(rows))
	for _, row := range selectedRows {
		key := util.ToKeyString(row[valueField])
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, row)
	}
	for _, row := range rows {
		key := util.ToKeyString(row[valueField])
		if _, exists := seen[key]; exists {
			continue
		}
		result = append(result, row)
	}
	return result
}

func normalizeSelectedOptionValues(rawSelected string) []any {
	trimmed := strings.TrimSpace(rawSelected)
	if trimmed == "" {
		return nil
	}

	var decoded []any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		return normalizeSelectedOptionItems(decoded)
	}

	parts := strings.Split(trimmed, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, normalizeOptionFilterValue(part))
	}
	return values
}

func normalizeSelectedOptionItems(items []any) []any {
	values := make([]any, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(util.ToString(item))
		if text == "" {
			continue
		}
		values = append(values, normalizeOptionFilterValue(text))
	}
	return values
}

func applyModelOptionFieldFilter(
	filters map[string]any,
	columnLookup map[string]string,
	rawField string,
	rawValue string,
) {
	field := frontrecord.ResolveColumnName(columnLookup, rawField)
	if field == "" || strings.TrimSpace(rawValue) == "" {
		return
	}

	values := normalizeSelectedOptionValues(rawValue)
	if len(values) == 0 {
		return
	}
	if len(values) == 1 {
		filters[field] = values[0]
		return
	}

	filters[field] = values
}

func buildModelOptionQueryFilters(
	baseFilters map[string]any,
	columnLookup map[string]string,
	keyword string,
	rawSearchFields string,
	labelField string,
) any {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return baseFilters
	}

	keywordFilter := buildModelOptionKeywordFilter(
		columnLookup,
		keyword,
		rawSearchFields,
		labelField,
	)
	if keywordFilter == nil {
		return baseFilters
	}
	if len(baseFilters) == 0 {
		return keywordFilter
	}

	return map[string]any{
		"and": []any{
			baseFilters,
			keywordFilter,
		},
	}
}

func buildModelOptionKeywordFilter(
	columnLookup map[string]string,
	keyword string,
	rawSearchFields string,
	labelField string,
) any {
	fields := resolveModelOptionSearchFields(columnLookup, rawSearchFields, labelField)
	if len(fields) == 0 {
		return nil
	}

	conditions := make([]any, 0, len(fields))
	for _, field := range fields {
		conditions = append(conditions, map[string]any{
			"main." + field: map[string]any{
				"like": "%" + keyword + "%",
			},
		})
	}

	if len(conditions) == 1 {
		return conditions[0]
	}
	return map[string]any{"or": conditions}
}

func resolveModelOptionSearchFields(
	columnLookup map[string]string,
	rawSearchFields string,
	labelField string,
) []string {
	candidates := strings.Split(strings.TrimSpace(rawSearchFields), ",")
	if strings.TrimSpace(rawSearchFields) == "" {
		candidates = []string{labelField}
	}

	fields := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		field := frontrecord.ResolveColumnName(columnLookup, candidate)
		if field == "" {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		fields = append(fields, field)
	}
	return fields
}

func getServiceOptions(c *server.Context) ([]map[string]any, error) {
	return GetServiceOptionsByInput(c, func(key string) string {
		return c.Input(key)
	})
}

func GetServiceOptionsByInput(c *server.Context, getInput func(string) string) ([]map[string]any, error) {
	serviceName := strings.TrimSpace(getInput("use"))
	if serviceName == "" {
		return nil, fmt.Errorf("服务名不能为空")
	}
	payload := map[string]any{
		"parent_id":    normalizeOptionFilterValue(util.FirstNonEmpty(getInput("parentId"), getInput("rootValue"))),
		"parent_field": util.FirstNonEmpty(getInput("parentField"), "parent_id"),
		"value_field":  util.FirstNonEmpty(getInput("valueField"), "id"),
		"label_field":  util.FirstNonEmpty(getInput("labelField"), "name"),
		"leaf_field":   strings.TrimSpace(getInput("leafField")),
	}

	result, err := frontcall.Service(c, serviceName, payload)
	if err != nil {
		return nil, err
	}

	switch current := result.(type) {
	case []map[string]any:
		return normalizeOptionRows(
			current,
			util.ToString(payload["value_field"]),
			util.ToString(payload["label_field"]),
			util.ToString(payload["leaf_field"]),
		), nil
	case []any:
		rows := make([]map[string]any, 0, len(current))
		for _, item := range current {
			if mapped, ok := item.(map[string]any); ok {
				rows = append(rows, mapped)
			}
		}
		return normalizeOptionRows(
			rows,
			util.ToString(payload["value_field"]),
			util.ToString(payload["label_field"]),
			util.ToString(payload["leaf_field"]),
		), nil
	default:
		return nil, fmt.Errorf("service option 返回格式错误")
	}
}

func normalizeOptionRows(rows []map[string]any, valueField, labelField, leafField string) []map[string]any {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := util.CloneMap(row)
		if valueField != "id" && row["id"] != nil {
			item["raw_id"] = row["id"]
		}
		item["id"] = row[valueField]
		item["value"] = fmt.Sprint(row[labelField])
		if strings.TrimSpace(leafField) != "" {
			item["leaf"] = util.ToBool(row[leafField])
		}
		items = append(items, item)
	}
	return items
}

func buildModelOptionSelectFields(
	valueField string,
	labelField string,
	parentField string,
	leafField string,
	extraFields []string,
) []string {
	fields := make([]string, 0, 4+len(extraFields))
	seen := map[string]struct{}{}

	appendField := func(field string) {
		field = strings.TrimSpace(field)
		if field == "" {
			return
		}

		fullField := "main." + field
		if _, ok := seen[fullField]; ok {
			return
		}

		seen[fullField] = struct{}{}
		fields = append(fields, fullField)
	}

	appendField(valueField)
	appendField(labelField)
	appendField(parentField)
	appendField(leafField)
	for _, field := range extraFields {
		appendField(field)
	}

	return fields
}

func resolveOptionExtraFields(columnLookup map[string]string, raw string, reserved ...string) []string {
	exclude := make(map[string]struct{}, len(reserved))
	for _, field := range reserved {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		exclude[field] = struct{}{}
	}

	result := make([]string, 0)
	seen := map[string]struct{}{}
	for _, part := range strings.Split(strings.TrimSpace(raw), ",") {
		field := frontrecord.ResolveColumnName(columnLookup, part)
		if field == "" {
			continue
		}
		if _, skip := exclude[field]; skip {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}

		seen[field] = struct{}{}
		result = append(result, field)
	}

	return result
}

func buildTreeOptionRows(items []map[string]any, parentField string, rootValue any) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}

	parentField = strings.TrimSpace(parentField)
	if parentField == "" {
		parentField = "parent_id"
	}

	children := map[string][]map[string]any{}
	byID := map[string]map[string]any{}
	for _, item := range items {
		idKey := util.ToKeyString(item["id"])
		parentKey := util.ToKeyString(item[parentField])
		cloned := util.CloneMap(item)
		cloned["children"] = []map[string]any{}
		byID[idKey] = cloned
		children[parentKey] = append(children[parentKey], cloned)
	}

	for idKey, childItems := range children {
		parent, ok := byID[idKey]
		if !ok {
			continue
		}
		parent["children"] = childItems
	}

	rootKey := util.ToKeyString(rootValue)
	return util.CloneMapSlice(children[rootKey])
}

func normalizeOptionFilterValue(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if number, ok := util.ParseInt64(trimmed); ok && number >= 0 {
		return number
	}
	return trimmed
}

func SeedRows(modelName, parentField string, parentValue any) []map[string]any {
	return SeedRowsByField(modelName, parentField, []any{parentValue})
}

func SeedRowsByField(modelName, field string, values []any) []map[string]any {
	resource := frontrecord.ResourceName(modelName)
	if resource == "" {
		return nil
	}

	for _, path := range optionSchemaCandidates(resource) {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var payload struct {
			Seeds []map[string]any `json:"seeds"`
		}
		if err := json.Unmarshal(content, &payload); err != nil || len(payload.Seeds) == 0 {
			continue
		}

		rows := make([]map[string]any, 0)
		for _, row := range payload.Seeds {
			if len(values) > 0 && !optionSeedMatchAny(row[field], values) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			return rows
		}
	}

	return nil
}

func optionSchemaCandidates(resource string) []string {
	candidates := []string{
		filepath.Join("data", "table", "shemic_"+resource+".json"),
		filepath.Join("data", "table", resource+".json"),
		filepath.Join("data", resource+".json"),
	}
	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if cleaned == "." || cleaned == "" {
			continue
		}
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func optionSeedMatch(left, right any) bool {
	return util.ToKeyString(left) == util.ToKeyString(right)
}

func optionSeedMatchAny(left any, values []any) bool {
	leftValue := util.ToKeyString(left)
	for _, value := range values {
		if leftValue == util.ToKeyString(value) {
			return true
		}
	}
	return false
}

type optionModel interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
}

func resolveOptionModel(modelName string) optionModel {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}

	if modelValue, ok := frontrecord.LoadSafe(modelName).(optionModel); ok {
		return modelValue
	}
	adapter := frontrecord.ResolveAdapter(modelName)
	if adapter == nil || !adapter.HasMethod("SelectMap", 3) {
		return nil
	}
	return adapter
}
