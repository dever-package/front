package option

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	devercache "github.com/shemic/dever/cache"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	authctx "github.com/dever-package/front/service/internal/authctx"
	frontcall "github.com/dever-package/front/service/internal/call"
	optionseed "github.com/dever-package/front/service/internal/optionseed"
	frontpage "github.com/dever-package/front/service/page"
	embedpageservice "github.com/dever-package/front/service/permission/embedpage"
	frontrecord "github.com/dever-package/front/service/record"
	"github.com/dever-package/front/service/runtimecache"
	"github.com/dever-package/front/service/siteconfig"
)

const (
	optionCacheTTL              = 5 * time.Second
	maxOptionKeywordLength      = 100
	maxOptionSelectedTextLength = 4096
	maxOptionSelectedValues     = 100
	maxOptionPageSize           = 500
)

var optionCache = devercache.New[string, []map[string]any](
	devercache.WithTTL(optionCacheTTL),
	devercache.WithMaxEntries(2048),
)

func init() {
	runtimecache.Register("front.option", optionCache.Invalidate, optionCache.Clear)
	frontpage.RegisterOptionRowsResolver(GetResolvedOptions)
}

func Get(c *server.Context) error {
	cacheKey := optionRequestCacheKey(c)
	if cacheKey != "" {
		if cached, ok := optionCache.Load(cacheKey); ok {
			return c.JSON(cloneOptionRows(cached))
		}
	}

	items, err := GetByInput(c, func(key string) string {
		return c.Input(key)
	})
	if err != nil {
		return c.Error(err)
	}
	if cacheKey != "" {
		optionCache.Store(cacheKey, cloneOptionRows(items))
	}
	return c.JSON(items)
}

func GetByInput(c *server.Context, getInput func(string) string) ([]map[string]any, error) {
	if getInput == nil {
		getInput = func(key string) string {
			return c.Input(key)
		}
	}
	if strings.TrimSpace(getInput("path")) == "" || strings.TrimSpace(getInput("key")) == "" {
		return nil, fmt.Errorf("option.path 和 option.key 不能为空")
	}
	return GetResolvedOptions(c, getInput)
}

func GetResolvedOptions(c *server.Context, getInput func(string) string) ([]map[string]any, error) {
	pathValue, pathQuery := splitOptionPathInput(getInput("path"))
	resolved, err := frontpage.ResolveOption(frontpage.ResolveOptionInput{
		Context:  c.Context(),
		Path:     pathValue,
		Key:      getInput("key"),
		Keyword:  normalizeOptionKeyword(getInput("keyword")),
		Selected: normalizeOptionSelectedText(getInput("selected")),
		ParentID: getInput("parentId"),
		Query:    collectOptionQuery(getInput, pathValue, pathQuery),
	})
	if err != nil {
		return nil, err
	}
	return getResolvedOptions(c, resolved)
}

func collectOptionQuery(getInput func(string) string, pathValue string, pathQuery map[string]string) map[string]any {
	result := map[string]any{}
	if pathValue != "" {
		result["path"] = pathValue
	}
	for key, value := range pathQuery {
		if strings.TrimSpace(value) != "" {
			result[key] = value
		}
	}
	for _, key := range []string{"path", "key", "keyword", "selected", "parentId", "level"} {
		if value := strings.TrimSpace(getInput(key)); value != "" {
			if key == "path" && pathValue != "" {
				continue
			}
			result[key] = value
		}
	}
	return result
}

func splitOptionPathInput(rawPath string) (string, map[string]string) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", nil
	}

	pathValue := rawPath
	rawQuery := ""
	if index := strings.IndexAny(rawPath, "?#"); index >= 0 {
		pathValue = rawPath[:index]
		if rawPath[index] == '?' {
			rawQuery = rawPath[index+1:]
			if hashIndex := strings.Index(rawQuery, "#"); hashIndex >= 0 {
				rawQuery = rawQuery[:hashIndex]
			}
		}
	}

	return frontpagepath.NormalizePath(pathValue), parseOptionPathQuery(rawQuery)
}

func parseOptionPathQuery(rawQuery string) map[string]string {
	if strings.TrimSpace(rawQuery) == "" {
		return nil
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil || len(values) == 0 {
		return nil
	}

	result := make(map[string]string, len(values))
	for key, items := range values {
		if key == "" || len(items) == 0 {
			continue
		}
		result[key] = items[0]
	}
	return result
}

func getResolvedOptions(c *server.Context, resolved frontpage.ResolvedOption) ([]map[string]any, error) {
	switch strings.ToLower(strings.TrimSpace(resolved.Type)) {
	case "service":
		return GetServiceOptionsByInput(c, resolvedOptionInput(resolved))
	case "model", "":
		return GetModelOptionsByInput(c.Context(), resolvedOptionInput(resolved))
	default:
		return nil, fmt.Errorf("option.type 不支持: %s", resolved.Type)
	}
}

func resolvedOptionInput(resolved frontpage.ResolvedOption) func(string) string {
	values := url.Values{}
	if strings.EqualFold(resolved.Type, "service") {
		values.Set("service", resolved.Service)
	} else {
		values.Set("model", resolved.Model)
	}
	values.Set("valueField", resolved.ValueField)
	values.Set("labelField", resolved.LabelField)
	values.Set("parentField", resolved.ParentField)
	values.Set("leafField", resolved.LeafField)
	values.Set("order", resolved.Order)
	values.Set("keyword", resolved.Keyword)
	values.Set("selected", resolved.Selected)
	values.Set("rootValue", resolved.RootValue)
	if resolved.Tree {
		values.Set("tree", "1")
	}
	if resolved.PageSize > 0 {
		values.Set("pageSize", fmt.Sprint(resolved.PageSize))
	}
	if resolved.Page > 0 {
		values.Set("page", fmt.Sprint(resolved.Page))
	}
	if len(resolved.SearchFields) > 0 {
		values.Set("searchFields", strings.Join(resolved.SearchFields, ","))
	}
	if len(resolved.ExtraFields) > 0 {
		values.Set("extraFields", strings.Join(resolved.ExtraFields, ","))
	}
	for field, value := range resolved.Filters {
		if strings.TrimSpace(field) == "" {
			continue
		}
		values.Set("filterField", field)
		values.Set("filterValue", fmt.Sprint(value))
		break
	}
	if resolved.ParentID != "" {
		values.Set("parentId", resolved.ParentID)
	}
	return func(key string) string {
		return values.Get(key)
	}
}

func GetModelOptionsByInput(ctx context.Context, getInput func(string) string) ([]map[string]any, error) {
	modelName := strings.TrimSpace(getInput("model"))
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
		normalizeOptionKeyword(getInput("keyword")),
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
		if pageSize := normalizeOptionPageSize(getInput("pageSize")); pageSize > 0 {
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

func normalizeOptionKeyword(value string) string {
	return truncateOptionInput(value, maxOptionKeywordLength)
}

func normalizeOptionSelectedText(value string) string {
	return truncateOptionInput(value, maxOptionSelectedTextLength)
}

func normalizeOptionPageSize(value string) int {
	pageSize := util.ToIntDefault(value, 0)
	if pageSize <= 0 {
		return 0
	}
	if pageSize > maxOptionPageSize {
		return maxOptionPageSize
	}
	return pageSize
}

func truncateOptionInput(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if maxLength <= 0 || value == "" {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return string(runes[:maxLength])
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

	trimmed = normalizeOptionSelectedText(trimmed)
	parts := strings.Split(trimmed, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		if len(values) >= maxOptionSelectedValues {
			break
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, normalizeOptionFilterValue(part))
	}
	return values
}

func normalizeSelectedOptionItems(items []any) []any {
	capacity := len(items)
	if capacity > maxOptionSelectedValues {
		capacity = maxOptionSelectedValues
	}
	values := make([]any, 0, capacity)
	for _, item := range items {
		if len(values) >= maxOptionSelectedValues {
			break
		}
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

func optionRequestCacheKey(c *server.Context) string {
	if c == nil {
		return ""
	}
	requestKey := optionOriginalURL(c)
	if pathValue := strings.TrimSpace(c.Input("path")); pathValue != "" || strings.TrimSpace(c.Input("key")) != "" {
		cleanPath, pathQuery := splitOptionPathInput(pathValue)
		requestKey = strings.Join([]string{
			cleanPath,
			optionPathQueryCacheKey(pathQuery),
			strings.TrimSpace(c.Input("key")),
			normalizeOptionKeyword(c.Input("keyword")),
			normalizeOptionSelectedText(c.Input("selected")),
			strings.TrimSpace(c.Input("parentId")),
			strings.TrimSpace(c.Input("level")),
		}, "|")
	}
	return fmt.Sprintf(
		"%s:%s:%d:%s",
		siteconfig.SiteKeyFromContext(c.Context()),
		siteconfig.PageFromContext(c.Context()),
		authctx.OptionalUID(c.Context()),
		requestKey,
	)
}

func optionOriginalURL(c *server.Context) string {
	if c == nil || c.Raw == nil {
		return ""
	}
	if raw, ok := c.Raw.(interface{ OriginalURL() string }); ok {
		return raw.OriginalURL()
	}
	return c.Path()
}

func cloneOptionRows(rows []map[string]any) []map[string]any {
	return util.CloneMapSlice(rows)
}

func GetServiceOptionsByInput(c *server.Context, getInput func(string) string) ([]map[string]any, error) {
	serviceName := strings.TrimSpace(getInput("service"))
	if serviceName == "" {
		return nil, fmt.Errorf("服务名不能为空")
	}
	payload := map[string]any{
		"parent_id":    normalizeOptionFilterValue(util.FirstNonEmpty(getInput("parentId"), getInput("rootValue"))),
		"parent_field": util.FirstNonEmpty(getInput("parentField"), "parent_id"),
		"value_field":  util.FirstNonEmpty(getInput("valueField"), "id"),
		"label_field":  util.FirstNonEmpty(getInput("labelField"), "name"),
		"leaf_field":   strings.TrimSpace(getInput("leafField")),
		"keyword":      normalizeOptionKeyword(getInput("keyword")),
		"selected":     normalizeOptionSelectedText(getInput("selected")),
		"page_size":    normalizeOptionPageSize(getInput("pageSize")),
		"page":         util.ToIntDefault(getInput("page"), 0),
	}
	if tree := strings.TrimSpace(getInput("tree")); tree != "" {
		payload["tree"] = util.ToBool(tree)
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

func optionPathQueryCacheKey(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}

	query := url.Values{}
	for key, value := range values {
		query.Set(key, value)
	}
	return query.Encode()
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
	return optionseed.Rows(modelName, parentField, parentValue)
}

func SeedRowsByField(modelName, field string, values []any) []map[string]any {
	return optionseed.RowsByField(modelName, field, values)
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
