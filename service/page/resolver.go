package page

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/shemic/dever/util"

	frontmeta "my/package/front/service/meta"
)

const (
	DefaultSubmitActionKey = "submit"
	DefaultDeleteActionKey = "delete"
)

type ResolveActionInput struct {
	Context context.Context
	Path    string
	Key     string
	Payload any
	Query   map[string]any
}

type ResolvedAction struct {
	Key         string
	RequestPath string
	Path        string
	Model       string
	PrimaryKey  string
	Config      ActionConfig
	Content     []byte
}

type ResolveOptionInput struct {
	Context  context.Context
	Path     string
	Key      string
	Keyword  string
	Selected string
	ParentID string
	Query    map[string]any
}

type ResolvedOption struct {
	Key          string
	Path         string
	Model        string
	Type         string
	Service      string
	Filters      map[string]any
	SearchFields []string
	ExtraFields  []string
	ValueField   string
	LabelField   string
	ParentField  string
	LeafField    string
	Order        string
	PageSize     int
	Page         int
	Tree         bool
	RootValue    string
	Keyword      string
	Selected     string
	ParentID     string
}

type rawPageConfig struct {
	Nodes  map[string][]map[string]any `json:"nodes"`
	Action map[string]ActionConfig     `json:"action"`
	Data   map[string]any              `json:"data"`
}

func ResolveAction(input ResolveActionInput) (ResolvedAction, error) {
	requestPath := normalizePath(input.Path)
	if requestPath == "" {
		return ResolvedAction{}, fmt.Errorf("页面路径不能为空")
	}

	content, err := ReadContentForContext(input.Context, requestPath)
	if err != nil {
		return ResolvedAction{}, err
	}

	key := normalizeActionKey(input.Key)
	if key == "" {
		key = DefaultSubmitActionKey
	}

	config, ok, err := resolveActionConfig(content, requestPath, key)
	if err != nil {
		return ResolvedAction{}, err
	}
	if !ok {
		return ResolvedAction{}, fmt.Errorf("action 不存在: %s", key)
	}
	config = NormalizeAction(config)
	config.Key = key
	if config.Type == "" {
		config.Type = defaultActionType(key)
	}
	if config.Type == "" {
		return ResolvedAction{}, fmt.Errorf("action.type 不能为空")
	}

	pathValue := ActionPath(requestPath, config)
	if pathValue == "" {
		pathValue = requestPath
	}
	modelName := ActionModelName(pathValue, config)
	return ResolvedAction{
		Key:         key,
		RequestPath: requestPath,
		Path:        pathValue,
		Model:       modelName,
		PrimaryKey:  ActionPrimaryKey(config),
		Config:      config,
		Content:     content,
	}, nil
}

func resolveActionConfig(content []byte, pathValue, key string) (ActionConfig, bool, error) {
	if config, ok, err := ParseNamedAction(content, key); err != nil || ok {
		return config, ok, err
	}

	if config, ok := defaultActionConfig(pathValue, key); ok {
		return config, true, nil
	}
	return ActionConfig{}, false, nil
}

func defaultActionConfig(pathValue, key string) (ActionConfig, bool) {
	switch normalizeActionKey(key) {
	case DefaultSubmitActionKey:
		if DefaultModelName(pathValue) == "" {
			return ActionConfig{}, false
		}
		return ActionConfig{
			Type: "save",
			Path: pathValue,
			Key:  DefaultSubmitActionKey,
		}, true
	case DefaultDeleteActionKey:
		if DefaultModelName(pathValue) == "" {
			return ActionConfig{}, false
		}
		return ActionConfig{
			Type: "delete",
			Path: pathValue,
			Key:  DefaultDeleteActionKey,
		}, true
	default:
		if DefaultModelName(pathValue) == "" || !looksLikeDeleteActionKey(key) {
			return ActionConfig{}, false
		}
		return ActionConfig{
			Type: "delete",
			Path: pathValue,
			Key:  key,
		}, true
	}
}

func defaultActionType(key string) string {
	switch normalizeActionKey(key) {
	case DefaultSubmitActionKey:
		return "save"
	case DefaultDeleteActionKey:
		return "delete"
	default:
		return ""
	}
}

func looksLikeDeleteActionKey(key string) bool {
	key = strings.ToLower(normalizeActionKey(key))
	return key == DefaultDeleteActionKey ||
		strings.HasPrefix(key, "delete") ||
		strings.Contains(key, "-delete") ||
		strings.Contains(key, "_delete")
}

func ResolveOption(input ResolveOptionInput) (ResolvedOption, error) {
	pathValue := normalizePath(input.Path)
	if pathValue == "" {
		return ResolvedOption{}, fmt.Errorf("页面路径不能为空")
	}
	key := normalizeOptionKey(input.Key)
	if key == "" {
		return ResolvedOption{}, fmt.Errorf("option key 不能为空")
	}

	content, err := ReadContentForContext(input.Context, pathValue)
	if err != nil {
		return ResolvedOption{}, err
	}
	pageConfig, err := parseRawPageConfig(content)
	if err != nil {
		return ResolvedOption{}, err
	}
	modelName := nodeLabelModelName(content, pathValue)

	item := findOptionNode(pageConfig.Nodes, key)
	rawOption := any(nil)
	if item != nil {
		rawOption = optionSourceForNode(item, key)
	}

	optionConfig := normalizeOptionConfig(rawOption)
	if item != nil {
		optionConfig = mergeNodeOptionMeta(optionConfig, item, input)
	}
	field := normalizeOptionFieldKey(key)
	if field == "" && item != nil {
		field = normalizeItemValueField(toString(item["value"]))
	}

	resolved := ResolvedOption{
		Key:          key,
		Path:         pathValue,
		Type:         "model",
		Model:        strings.TrimSpace(optionConfig.Use),
		Service:      strings.TrimSpace(optionConfig.Use),
		Filters:      resolveOptionFilters(optionConfig.Filters, input),
		SearchFields: optionConfig.SearchFields,
		ExtraFields:  optionConfig.ExtraFields,
		ValueField:   util.FirstNonEmpty(optionConfig.ValueField, "id"),
		LabelField:   util.FirstNonEmpty(optionConfig.LabelField, "name"),
		ParentField:  util.FirstNonEmpty(optionConfig.ParentField, "parent_id"),
		LeafField:    optionConfig.LeafField,
		Order:        optionConfig.Order,
		PageSize:     optionConfig.PageSize,
		Page:         optionConfig.Page,
		Tree:         optionConfig.Tree,
		RootValue:    optionConfig.RootValue,
		Keyword:      strings.TrimSpace(input.Keyword),
		Selected:     strings.TrimSpace(input.Selected),
		ParentID:     strings.TrimSpace(input.ParentID),
	}

	if optionConfig.Type != "" {
		resolved.Type = optionConfig.Type
	}
	if resolved.Type == "service" {
		if resolved.Service == "" {
			return ResolvedOption{}, fmt.Errorf("option service 不能为空")
		}
		return resolved, nil
	}

	if resolved.Model == "" && isDefaultCategoryOption(item, rawOption) {
		if modelName == "" {
			return ResolvedOption{}, fmt.Errorf("分类 option 无法推导模型: %s", key)
		}
		resolved.Model = modelName
	}
	if resolved.Model == "" {
		relation, ok := resolveOptionRelation(modelName, field)
		if !ok {
			return ResolvedOption{}, fmt.Errorf("option 无法推导模型: %s", key)
		}
		resolved.Model = relation.Option
		resolved.ValueField = util.FirstNonEmpty(optionConfig.ValueField, relation.OptionValueField, "id")
		resolved.LabelField = util.FirstNonEmpty(optionConfig.LabelField, relation.OptionLabelField, "name")
		if optionConfig.ParentField == "" {
			resolved.ParentField = ""
		}
		if resolved.Order == "" {
			resolved.Order = relation.OptionOrder
		}
	}
	if resolved.Model == "" {
		return ResolvedOption{}, fmt.Errorf("option 模型不能为空")
	}
	return resolved, nil
}

func isDefaultCategoryOption(item map[string]any, rawOption any) bool {
	if len(item) == 0 || strings.TrimSpace(toString(item["type"])) != "show-category-list" {
		return false
	}

	switch current := rawOption.(type) {
	case string:
		_, ok := localOptionDataKey(current)
		return ok
	case nil:
		return true
	default:
		return false
	}
}

func resolveOptionFilters(filters map[string]any, input ResolveOptionInput) map[string]any {
	if len(filters) == 0 {
		return filters
	}
	resolved, _ := ResolveTemplateValue(filters, TemplateContext{
		Context: input.Context,
		Query:   input.Query,
	}).(map[string]any)
	return resolved
}

type optionConfig struct {
	Key          string         `json:"key"`
	Type         string         `json:"type"`
	Use          string         `json:"use"`
	Model        string         `json:"model"`
	Service      string         `json:"service"`
	Filters      map[string]any `json:"filters"`
	SearchFields []string       `json:"searchFields"`
	ExtraFields  []string       `json:"extraFields"`
	ValueField   string         `json:"valueField"`
	LabelField   string         `json:"labelField"`
	ParentField  string         `json:"parentField"`
	LeafField    string         `json:"leafField"`
	Order        string         `json:"order"`
	PageSize     int            `json:"pageSize"`
	Page         int            `json:"page"`
	Tree         bool           `json:"tree"`
	RootValue    string         `json:"rootValue"`
}

func normalizeOptionConfig(raw any) optionConfig {
	switch current := raw.(type) {
	case nil:
		return optionConfig{}
	case string:
		return parseOptionURL(current)
	case map[string]any:
		var config optionConfig
		_ = mapToStruct(current, &config)
		config.Key = strings.TrimSpace(config.Key)
		config.Type = strings.ToLower(strings.TrimSpace(config.Type))
		config.Use = util.FirstNonEmpty(config.Use, config.Model, config.Service)
		if config.Type == "" && strings.TrimSpace(config.Service) != "" {
			config.Type = "service"
		}
		return config
	default:
		return optionConfig{}
	}
}

func parseOptionURL(raw string) optionConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.Contains(raw, "route/option") {
		return optionConfig{}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return optionConfig{}
	}
	values := parsed.Query()
	config := optionConfig{
		Type:        strings.ToLower(strings.TrimSpace(values.Get("type"))),
		Use:         strings.TrimSpace(values.Get("use")),
		ValueField:  strings.TrimSpace(values.Get("valueField")),
		LabelField:  strings.TrimSpace(values.Get("labelField")),
		ParentField: strings.TrimSpace(values.Get("parentField")),
		LeafField:   strings.TrimSpace(values.Get("leafField")),
		Order:       strings.TrimSpace(values.Get("order")),
		PageSize:    util.ToIntDefault(values.Get("pageSize"), 0),
		Page:        util.ToIntDefault(values.Get("page"), 0),
		Tree:        util.ToBool(values.Get("tree")),
		RootValue:   strings.TrimSpace(values.Get("rootValue")),
	}
	if filterField := strings.TrimSpace(values.Get("filterField")); filterField != "" {
		config.Filters = map[string]any{filterField: strings.TrimSpace(values.Get("filterValue"))}
	}
	config.SearchFields = splitCSV(values.Get("searchFields"))
	config.ExtraFields = splitCSV(values.Get("extraFields"))
	return config
}

func parseRawPageConfig(content []byte) (rawPageConfig, error) {
	var config rawPageConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return rawPageConfig{}, fmt.Errorf("页面配置解析失败")
	}
	return config, nil
}

func findOptionNode(nodes map[string][]map[string]any, key string) map[string]any {
	key = normalizeOptionKey(key)
	if key == "" {
		return nil
	}
	candidates := []optionNodeMatchCandidate{}
	for _, items := range nodes {
		candidates = append(candidates, optionNodeMatchCandidates(items, key)...)
	}
	for _, candidate := range candidates {
		if candidate.exact {
			return candidate.item
		}
	}
	for _, candidate := range candidates {
		if candidate.field {
			return candidate.item
		}
	}
	return nil
}

type optionNodeMatchCandidate struct {
	item  map[string]any
	exact bool
	field bool
}

func optionNodeMatchCandidates(items []map[string]any, key string) []optionNodeMatchCandidate {
	result := []optionNodeMatchCandidate{}
	for _, item := range items {
		if match := matchOptionNode(item, key); match.exact || match.field {
			match.item = item
			result = append(result, match)
		}
		for _, child := range normalizeNodeItems(item["items"]) {
			result = append(result, optionNodeMatchCandidates([]map[string]any{child}, key)...)
		}
	}
	return result
}

func matchOptionNode(item map[string]any, key string) optionNodeMatchCandidate {
	key = normalizeOptionKey(key)
	if key == "" || !canResolveNodeOption(item) {
		return optionNodeMatchCandidate{}
	}
	if isDefaultCategoryOptionKey(item, key) {
		return optionNodeMatchCandidate{exact: true}
	}
	for _, candidate := range stableNodeOptionKeyCandidates(item) {
		if normalizeOptionKey(candidate) == key {
			return optionNodeMatchCandidate{exact: true}
		}
	}
	for _, candidate := range fieldNodeOptionKeyCandidates(item) {
		if normalizeOptionKey(candidate) == key {
			return optionNodeMatchCandidate{field: true}
		}
	}
	return optionNodeMatchCandidate{}
}

func nodeOptionKey(item map[string]any) string {
	if key := defaultCategoryOptionKeyForNode(item); key != "" {
		return key
	}
	for _, candidate := range stableNodeOptionKeyCandidates(item) {
		if key := normalizeOptionKey(candidate); key != "" {
			return key
		}
	}
	for _, candidate := range fieldNodeOptionKeyCandidates(item) {
		if key := normalizeOptionKey(candidate); key != "" {
			return key
		}
	}
	return ""
}

func isDefaultCategoryOptionKey(item map[string]any, key string) bool {
	return defaultCategoryOptionKeyForNode(item) == normalizeOptionKey(key)
}

func defaultCategoryOptionKeyForNode(item map[string]any) string {
	if strings.TrimSpace(toString(item["type"])) != "show-category-list" {
		return ""
	}
	if rawOption, ok := item["option"].(string); ok {
		rawOption = strings.TrimSpace(rawOption)
		if key, ok := localOptionDataKey(rawOption); ok {
			return normalizeOptionKey(key)
		}
		if rawOption != "" && !strings.Contains(rawOption, "route/option") {
			return ""
		}
	}
	return normalizeOptionKey(defaultCategoryOptionKey(item))
}

func stableNodeOptionKeyCandidates(item map[string]any) []string {
	meta, _ := item["meta"].(map[string]any)
	option, _ := item["option"].(map[string]any)
	return []string{
		toString(item["optionKey"]),
		toString(meta["optionKey"]),
		toString(item["key"]),
		toString(meta["loadKey"]),
		toString(meta["sourceKey"]),
		toString(meta["optionSourceKey"]),
		toString(meta["paramSourceKey"]),
		nestedOptionKey(meta["fillFromOption"]),
		toString(option["key"]),
	}
}

func fieldNodeOptionKeyCandidates(item map[string]any) []string {
	if hasExplicitNodeOptionSource(item) {
		return []string{
			toString(item["id"]),
			toString(item["value"]),
		}
	}
	return []string{
		toString(item["value"]),
		toString(item["id"]),
	}
}

func canResolveNodeOption(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	if strings.TrimSpace(toString(item["type"])) == "show-category-list" {
		return true
	}
	if hasExplicitNodeOptionSource(item) {
		return true
	}
	meta, _ := item["meta"].(map[string]any)
	if hasMetaNodeOptionSource(meta) {
		return true
	}
	option := item["option"]
	if option == nil {
		return false
	}
	return hasOptionSourceValue(option)
}

func hasExplicitNodeOptionSource(item map[string]any) bool {
	meta, _ := item["meta"].(map[string]any)
	option, _ := item["option"].(map[string]any)
	for _, value := range []string{
		toString(meta["use"]),
		toString(meta["model"]),
		toString(meta["service"]),
		toString(meta["childUse"]),
		toString(meta["childModel"]),
		toString(meta["childService"]),
		toString(option["use"]),
		toString(option["model"]),
		toString(option["service"]),
		toString(option["type"]),
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func hasMetaNodeOptionSource(meta map[string]any) bool {
	if len(meta) == 0 {
		return false
	}
	for _, key := range []string{
		"loadOption",
		"loadApi",
		"source",
		"optionSource",
		"optionSourceOption",
		"paramSource",
		"paramSourceOption",
	} {
		if hasOptionSourceValue(meta[key]) {
			return true
		}
	}
	return hasNestedOptionSource(meta["fillFromOption"])
}

func hasNestedOptionSource(raw any) bool {
	config, _ := raw.(map[string]any)
	if len(config) == 0 {
		return false
	}
	return hasOptionSourceValue(config["option"]) || hasOptionSourceValue(config["source"])
}

func hasOptionSourceValue(raw any) bool {
	switch current := raw.(type) {
	case nil:
		return false
	case string:
		return strings.Contains(strings.TrimSpace(current), "route/option")
	case map[string]any:
		return len(current) > 0
	default:
		return false
	}
}

func nestedOptionKey(raw any) string {
	config, _ := raw.(map[string]any)
	if len(config) == 0 {
		return ""
	}
	return toString(config["optionKey"])
}

func optionSourceForNode(item map[string]any, key string) any {
	meta, _ := item["meta"].(map[string]any)
	switch normalizeOptionKey(key) {
	case normalizeOptionKey(toString(meta["loadKey"])):
		if meta["loadOption"] != nil {
			return meta["loadOption"]
		}
		return meta["loadApi"]
	case normalizeOptionKey(toString(meta["sourceKey"])):
		return meta["source"]
	case normalizeOptionKey(toString(meta["optionSourceKey"])):
		if meta["optionSourceOption"] != nil {
			return meta["optionSourceOption"]
		}
		return meta["optionSource"]
	case normalizeOptionKey(toString(meta["paramSourceKey"])):
		if meta["paramSourceOption"] != nil {
			return meta["paramSourceOption"]
		}
		return meta["paramSource"]
	default:
		if fillOption, ok := nestedOptionSource(meta["fillFromOption"], key); ok {
			return fillOption
		}
		return item["option"]
	}
}

func nestedOptionSource(raw any, key string) (any, bool) {
	config, _ := raw.(map[string]any)
	if len(config) == 0 {
		return nil, false
	}
	if normalizeOptionKey(toString(config["optionKey"])) != normalizeOptionKey(key) {
		return nil, false
	}
	if config["option"] != nil {
		return config["option"], true
	}
	return config["source"], true
}

func mergeNodeOptionMeta(config optionConfig, item map[string]any, input ResolveOptionInput) optionConfig {
	meta, _ := item["meta"].(map[string]any)
	if len(meta) == 0 {
		return config
	}
	childLevel := util.ToIntDefault(toString(input.Query["level"]), 0) > 0 &&
		strings.TrimSpace(toString(meta["childUse"])) != ""
	if config.Use == "" {
		config.Use = util.FirstNonEmpty(
			childOptionMetaValue(meta, childLevel, "Use", "use"),
			toString(meta["model"]),
			toString(meta["service"]),
		)
	}
	if config.Type == "" {
		config.Type = strings.ToLower(strings.TrimSpace(
			childOptionMetaValue(meta, childLevel, "Type", "type"),
		))
	}
	if config.ValueField == "" {
		config.ValueField = toString(meta["valueField"])
	}
	if config.LabelField == "" {
		config.LabelField = toString(meta["labelField"])
	}
	if len(config.ExtraFields) == 0 {
		config.ExtraFields = splitCSV(childOptionMetaValue(meta, childLevel, "ExtraFields", "extraFields"))
	}
	if strings.TrimSpace(toString(item["type"])) == "show-category-list" {
		config.ExtraFields = appendMissingCSVItems(
			config.ExtraFields,
			toString(meta["sortField"]),
			toString(meta["statusField"]),
			toString(meta["countField"]),
		)
	}
	if config.ParentField == "" {
		config.ParentField = childOptionMetaValue(meta, childLevel, "ParentField", "parentField")
	}
	if childLevel && strings.TrimSpace(input.Selected) != "" {
		config.ExtraFields = appendMissingCSVItem(config.ExtraFields, config.ParentField)
	}
	if config.LeafField == "" {
		config.LeafField = childOptionMetaValue(meta, childLevel, "LeafField", "leafField")
	}
	if config.Order == "" {
		config.Order = childOptionMetaValue(meta, childLevel, "Order", "order")
	}
	if config.PageSize == 0 {
		config.PageSize = util.ToIntDefault(toString(meta["pageSize"]), 0)
	}
	if config.RootValue == "" {
		config.RootValue = toString(meta["rootValue"])
	}
	if config.Use != "" && config.Type == "" && strings.TrimSpace(toString(meta["service"])) != "" {
		config.Type = "service"
	}
	if config.Use != "" && config.Type == "" && childLevel && strings.EqualFold(toString(meta["type"]), "service") {
		config.Type = "service"
	}
	return config
}

func appendMissingCSVItem(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, item := range items {
		if strings.TrimSpace(item) == value {
			return items
		}
	}
	return append(items, value)
}

func appendMissingCSVItems(items []string, values ...string) []string {
	for _, value := range values {
		items = appendMissingCSVItem(items, value)
	}
	return items
}

func childOptionMetaValue(meta map[string]any, childLevel bool, suffix string, fallback string) string {
	if childLevel {
		if value := toString(meta["child"+suffix]); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return toString(meta[fallback])
}

func normalizeOptionKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "option.")
	key = strings.TrimPrefix(key, "data.option.")
	return key
}

func normalizeOptionFieldKey(key string) string {
	key = normalizeOptionKey(key)
	for _, prefix := range []string{"form.", "search.", "data.", "state."} {
		key = strings.TrimPrefix(key, prefix)
	}
	return strings.TrimSpace(key)
}

func resolveOptionRelation(modelName, field string) (frontmeta.Relation, bool) {
	field = normalizeOptionFieldKey(field)
	if modelName == "" || field == "" {
		return frontmeta.Relation{}, false
	}
	for _, relation := range frontmeta.ResolveModelRelations(modelName) {
		for _, candidate := range relationOptionCandidates(relation) {
			if normalizeOptionFieldKey(candidate) == field {
				return relation, true
			}
		}
	}
	return frontmeta.Relation{}, false
}

func relationOptionCandidates(relation frontmeta.Relation) []string {
	candidates := []string{relation.Field, relation.Name}
	candidates = append(candidates, relation.OptionKeys...)
	if strings.HasSuffix(relation.Field, "_id") {
		candidates = append(candidates, strings.TrimSuffix(relation.Field, "_id"))
	}
	if strings.HasSuffix(relation.Field, "_ids") {
		base := strings.TrimSuffix(relation.Field, "_ids")
		candidates = append(candidates, base, base+"_id", base+"s")
	}
	return candidates
}

func mapToStruct(source any, target any) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	items := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
