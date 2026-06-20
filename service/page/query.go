package page

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmeta "github.com/dever-package/front/service/meta"
	frontrecord "github.com/dever-package/front/service/record"
)

type filterFieldConfig struct {
	Field     string
	Type      string
	StartKey  string
	EndKey    string
	ValueType string
}

type queryValueLookup func(string) string

type listModel interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
	Count(ctx context.Context, filters any, options ...map[string]any) int64
}

const (
	maxQueryListPageSize = 500
	maxQueryKeywordRunes = 100
)

func queryRecordID(c *server.Context, key string) (uint64, bool) {
	value := normalizeQueryInputText(c.Input(key))
	if value == "" {
		return 0, false
	}

	number, ok := util.ParseInt64(value)
	if !ok || number <= 0 {
		return 0, false
	}

	return uint64(number), true
}

func queryModelList(c *server.Context, modelValue any, current map[string]any) ([]map[string]any, int64, int, int, error) {
	return queryModelListWithLookup(c.Context(), modelValue, current, readContextQueryLookup(c))
}

func QueryModelListWithQuery(
	ctx context.Context,
	modelValue any,
	current map[string]any,
	query map[string]string,
) ([]map[string]any, int64, int, int, error) {
	return queryModelListWithLookup(ctx, modelValue, current, mapQueryLookup(query))
}

func queryModelListWithLookup(
	ctx context.Context,
	modelValue any,
	current map[string]any,
	lookup queryValueLookup,
) ([]map[string]any, int64, int, int, error) {
	treeMode := util.ToBool(current["tree"])
	pageSize := normalizeQueryListPageSize(queryIntValue(lookup, "pageSize", util.ToIntDefault(current["pageSize"], 10)))
	page := queryIntValue(lookup, "page", util.ToIntDefault(current["page"], 1))
	if page < 1 {
		page = 1
	}

	filters := buildModelFiltersWithLookup(ctx, lookup, current)
	if filters == nil {
		filters = map[string]any{}
	}
	order := util.ToStringTrimmed(current["order"])

	model, ok := resolveListModel(modelValue)
	if !ok {
		return nil, 0, page, pageSize, fmt.Errorf("model 不支持 SelectMap/Count")
	}

	options := map[string]any{
		"page":     page,
		"pageSize": pageSize,
	}
	if order != "" {
		options["order"] = order
	}
	if treeMode {
		delete(options, "page")
		delete(options, "pageSize")
		page = 1
	}

	rows := model.SelectMap(ctx, filters, options)
	total := model.Count(ctx, filters)
	if treeMode {
		pageSize = int(total)
		if pageSize <= 0 {
			pageSize = len(rows)
		}
	}

	return rows, total, page, pageSize, nil
}

func resolveListModel(modelValue any) (listModel, bool) {
	if model, ok := modelValue.(listModel); ok {
		return model, true
	}
	adapter := frontrecord.Wrap(modelValue)
	if adapter == nil || !adapter.HasMethod("SelectMap", 3) {
		return nil, false
	}
	if !adapter.HasMethod("Count", 3) && !adapter.HasMethod("Count", 2) {
		return nil, false
	}
	return adapter, true
}

func buildModelFilters(c *server.Context, current map[string]any) any {
	return buildModelFiltersWithLookup(c.Context(), readContextQueryLookup(c), current)
}

func BuildModelFiltersWithQuery(current map[string]any, query map[string]string) any {
	return buildModelFiltersWithLookup(context.Background(), mapQueryLookup(query), current)
}

func buildModelFiltersWithLookup(
	ctx context.Context,
	lookup queryValueLookup,
	current map[string]any,
) any {
	filters := buildExactModelFiltersWithLookup(ctx, lookup, current)

	keyword := normalizeQueryKeyword(lookup("keyword"))
	if keyword == "" {
		return filters
	}

	searchFields := mapStringSlice(current["searchFields"])
	if len(searchFields) == 0 {
		return filters
	}

	orFilters := make([]any, 0, len(searchFields))
	for _, field := range searchFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		orFilters = append(orFilters, map[string]any{
			"main." + util.ToSnake(field): map[string]any{
				"like": "%" + keyword + "%",
			},
		})
	}
	if len(orFilters) == 0 {
		return filters
	}

	if filters == nil {
		return map[string]any{"or": orFilters}
	}

	return map[string]any{
		"and": []any{
			filters,
			map[string]any{"or": orFilters},
		},
	}
}

func buildExactModelFilters(c *server.Context, current map[string]any) any {
	return buildExactModelFiltersWithLookup(c.Context(), readContextQueryLookup(c), current)
}

func buildExactModelFiltersWithLookup(
	ctx context.Context,
	lookup queryValueLookup,
	current map[string]any,
) any {
	filterFields := mapFilterFieldConfigs(current["filterFields"])
	if len(filterFields) == 0 {
		filterFields = []filterFieldConfig{
			{Field: "status", Type: "exact"},
			{Field: "role", Type: "exact"},
		}
	}

	defaultFilters, _ := current["defaultFilters"].(map[string]any)
	modelName := util.ToStringTrimmed(current["modelName"])
	conditions := make([]any, 0, len(filterFields))

	for _, config := range filterFields {
		if config.Field == "" {
			continue
		}

		if condition, ok := buildQueryFilterCondition(ctx, lookup, defaultFilters, modelName, config); ok {
			conditions = append(conditions, condition)
			continue
		}
	}

	return joinFilterConditions(conditions)
}

func buildQueryFilterCondition(
	ctx context.Context,
	lookup queryValueLookup,
	defaultFilters map[string]any,
	modelName string,
	config filterFieldConfig,
) (any, bool) {
	if strings.EqualFold(config.Type, "date-range") {
		return buildDateRangeFilterCondition(lookup, defaultFilters, config)
	}

	value, ok := readQueryFilterValue(lookup, config.Field, defaultFilters)
	if !ok {
		return nil, false
	}
	if isAllQueryFilterValue(value) || !hasMeaningfulQueryFilterValue(value) {
		return nil, false
	}

	if relationFilter, ok := frontmeta.BuildRelationFilter(ctx, modelName, config.Field, value); ok {
		return relationFilter, true
	}

	return map[string]any{
		"main." + util.ToSnake(config.Field): normalizeQueryFilterValue(value),
	}, true
}

func buildDateRangeFilterCondition(
	lookup queryValueLookup,
	defaultFilters map[string]any,
	config filterFieldConfig,
) (any, bool) {
	startKey := firstNonEmptyQueryKey(config.StartKey, config.Field+"_start")
	endKey := firstNonEmptyQueryKey(config.EndKey, config.Field+"_end")
	startValue, hasStart := readQueryFilterValue(lookup, startKey, defaultFilters)
	endValue, hasEnd := readQueryFilterValue(lookup, endKey, defaultFilters)

	operators := map[string]any{}
	valueType := strings.ToLower(strings.TrimSpace(config.ValueType))
	if valueType == "" {
		valueType = "datetime"
	}

	if hasStart && !isAllQueryFilterValue(startValue) && hasMeaningfulQueryFilterValue(startValue) {
		if normalized, ok := normalizeRangeStartValue(startValue, valueType); ok {
			operators["gte"] = normalized
		}
	}
	if hasEnd && !isAllQueryFilterValue(endValue) && hasMeaningfulQueryFilterValue(endValue) {
		if normalized, operator, ok := normalizeRangeEndValue(endValue, valueType); ok {
			operators[operator] = normalized
		}
	}

	if len(operators) == 0 {
		return nil, false
	}

	return map[string]any{
		"main." + util.ToSnake(config.Field): operators,
	}, true
}

func normalizeRangeStartValue(value any, valueType string) (any, bool) {
	parsed, granularity, ok := parseRangeTimeValue(value)
	if !ok {
		return nil, false
	}
	if valueType == "date" {
		return parsed.Format("2006-01-02"), true
	}
	if granularity == "date" {
		return parsed.Format("2006-01-02 00:00:00"), true
	}
	return parsed.Format("2006-01-02 15:04:05"), true
}

func normalizeRangeEndValue(value any, valueType string) (any, string, bool) {
	parsed, granularity, ok := parseRangeTimeValue(value)
	if !ok {
		return nil, "", false
	}
	if valueType == "date" {
		return parsed.Format("2006-01-02"), "lte", true
	}

	switch granularity {
	case "date":
		return parsed.AddDate(0, 0, 1).Format("2006-01-02 15:04:05"), "lt", true
	case "minute":
		return parsed.Add(time.Minute).Format("2006-01-02 15:04:05"), "lt", true
	default:
		return parsed.Add(time.Second).Format("2006-01-02 15:04:05"), "lt", true
	}
}

func parseRangeTimeValue(value any) (time.Time, string, bool) {
	text := normalizeQueryInputText(util.ToString(value))
	if text == "" {
		return time.Time{}, "", false
	}

	normalized := strings.ReplaceAll(text, "T", " ")
	layouts := []struct {
		layout      string
		granularity string
	}{
		{layout: "2006-01-02 15:04:05", granularity: "second"},
		{layout: "2006-01-02 15:04", granularity: "minute"},
		{layout: "2006-01-02", granularity: "date"},
	}

	for _, current := range layouts {
		parsed, err := time.ParseInLocation(current.layout, normalized, time.Local)
		if err == nil {
			return parsed, current.granularity, true
		}
	}

	return time.Time{}, "", false
}

func firstNonEmptyQueryKey(values ...string) string {
	for _, value := range values {
		current := strings.TrimSpace(value)
		if current != "" {
			return current
		}
	}
	return ""
}

func queryInt(c *server.Context, key string, fallback int) int {
	return queryIntValue(readContextQueryLookup(c), key, fallback)
}

func queryIntValue(lookup queryValueLookup, key string, fallback int) int {
	value := normalizeQueryInputText(lookup(key))
	if value == "" {
		return fallback
	}
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return fallback
	}
	return number
}

func normalizeQueryListPageSize(value int) int {
	if value > maxQueryListPageSize {
		return maxQueryListPageSize
	}
	if value < 0 {
		return 0
	}
	return value
}

func normalizeQueryKeyword(raw string) string {
	value := normalizeQueryInputText(raw)
	if value == "" {
		return ""
	}
	return truncateQueryText(value, maxQueryKeywordRunes)
}

func truncateQueryText(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func readContextQueryLookup(c *server.Context) queryValueLookup {
	if c == nil {
		return func(string) string { return "" }
	}
	return func(key string) string {
		return c.Input(key)
	}
}

func mapQueryLookup(query map[string]string) queryValueLookup {
	return func(key string) string {
		if len(query) == 0 {
			return ""
		}
		return query[strings.TrimSpace(key)]
	}
}

func normalizeQueryInputText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if !strings.HasPrefix(trimmed, "\"") || !strings.HasSuffix(trimmed, "\"") {
		return trimmed
	}

	var decoded string
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return trimmed
	}

	return strings.TrimSpace(decoded)
}

func normalizeQueryFilterValue(value any) any {
	return normalizeQueryFilterAny(value)
}

func normalizeQueryFilterAny(value any) any {
	switch current := value.(type) {
	case nil:
		return nil
	case string:
		normalized := normalizeQueryInputText(current)
		if normalized == "" {
			return ""
		}
		if decoded, ok := decodeQueryJSONValue(normalized); ok {
			return normalizeQueryFilterAny(decoded)
		}
		if number, ok := util.ParseInt64(normalized); ok {
			return number
		}
		return normalized
	case []string:
		items := make([]any, 0, len(current))
		for _, item := range current {
			normalized := normalizeQueryFilterAny(item)
			if hasMeaningfulQueryFilterValue(normalized) {
				items = append(items, normalized)
			}
		}
		return items
	case []any:
		items := make([]any, 0, len(current))
		for _, item := range current {
			normalized := normalizeQueryFilterAny(item)
			if hasMeaningfulQueryFilterValue(normalized) {
				items = append(items, normalized)
			}
		}
		return items
	default:
		return value
	}
}

func decodeQueryJSONValue(value string) (any, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false
	}
	if !strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func readQueryFilterValue(
	lookup queryValueLookup,
	key string,
	defaultFilters map[string]any,
) (any, bool) {
	raw := normalizeQueryInputText(lookup(key))
	if raw != "" {
		return normalizeQueryFilterAny(raw), true
	}
	if defaultFilters == nil {
		return nil, false
	}
	value, ok := defaultFilters[key]
	if !ok {
		return nil, false
	}
	return value, true
}

func hasMeaningfulQueryFilterValue(value any) bool {
	switch current := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(current) != ""
	case []any:
		return len(current) > 0
	case []string:
		return len(current) > 0
	default:
		return true
	}
}

func isAllQueryFilterValue(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == "__all__"
}

func joinFilterConditions(conditions []any) any {
	switch len(conditions) {
	case 0:
		return nil
	case 1:
		return conditions[0]
	default:
		return map[string]any{"and": conditions}
	}
}

func mapStringSlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, text)
		}
	}
	return result
}

func mapFilterFieldConfigs(value any) []filterFieldConfig {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}

	result := make([]filterFieldConfig, 0, len(raw))
	for _, item := range raw {
		switch current := item.(type) {
		case string:
			field := strings.TrimSpace(current)
			if field == "" {
				continue
			}
			result = append(result, filterFieldConfig{
				Field: field,
				Type:  "exact",
			})
		case map[string]any:
			field := strings.TrimSpace(util.ToString(current["field"]))
			if field == "" {
				field = strings.TrimSpace(util.ToString(current["key"]))
			}
			if field == "" {
				continue
			}
			result = append(result, filterFieldConfig{
				Field:     field,
				Type:      strings.TrimSpace(util.ToString(current["type"])),
				StartKey:  strings.TrimSpace(util.ToString(current["startKey"])),
				EndKey:    strings.TrimSpace(util.ToString(current["endKey"])),
				ValueType: strings.TrimSpace(util.ToString(current["valueType"])),
			})
		}
	}

	return result
}
