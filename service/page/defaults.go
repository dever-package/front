package page

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontoption "my/package/front/service/option"
)

func applyNodeDefaults(
	c *server.Context,
	rawLayout json.RawMessage,
	rawNodes json.RawMessage,
	rawData json.RawMessage,
	pathValue string,
) (json.RawMessage, error) {
	if len(rawData) == 0 {
		rawData = json.RawMessage(`{}`)
	}

	var nodes map[string][]map[string]any
	if len(rawNodes) > 0 {
		if err := json.Unmarshal(rawNodes, &nodes); err != nil {
			return nil, fmt.Errorf("页面 nodes 解析失败")
		}
	}

	var payload any
	if err := json.Unmarshal(rawData, &payload); err != nil {
		return nil, fmt.Errorf("页面 data 解析失败")
	}

	root, ok := payload.(map[string]any)
	if !ok {
		return rawData, nil
	}

	changed := false
	if applyFormFieldDefaults(root, nodes) {
		changed = true
	}
	if applyPageDataDefaults(root, nodes, pathValue) {
		changed = true
	}
	for _, items := range nodes {
		for _, item := range items {
			applied, err := applyDefaultCategoryValue(c, root, item)
			if err != nil {
				return nil, err
			}
			if applied {
				changed = true
			}
		}
	}

	applied, err := applyLinkedPageCategoryDefaults(c, rawLayout, root, pathValue)
	if err != nil {
		return nil, err
	}
	if applied {
		changed = true
	}

	if applySearchDefaults(root) {
		changed = true
	}

	if !changed {
		return rawData, nil
	}

	content, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("页面 data 编码失败")
	}
	return json.RawMessage(content), nil
}

func applyFormFieldDefaults(root map[string]any, nodes map[string][]map[string]any) bool {
	form, _ := root["form"].(map[string]any)
	if len(form) == 0 {
		return false
	}
	if len(mapStringSlice(form["_fields"])) > 0 {
		return false
	}

	fields := collectFormFieldKeys(nodes)
	if len(fields) == 0 {
		return false
	}

	form["_fields"] = stringSliceToAny(fields)
	root["form"] = form
	return true
}

func collectFormFieldKeys(nodes map[string][]map[string]any) []string {
	seen := map[string]struct{}{}
	fields := make([]string, 0)
	for _, items := range nodes {
		for _, item := range items {
			fields = appendFormFieldKeys(fields, seen, item)
		}
	}
	return fields
}

func appendFormFieldKeys(fields []string, seen map[string]struct{}, item map[string]any) []string {
	if field := formFieldKeyFromPath(util.ToStringTrimmed(item["value"])); field != "" {
		if _, exists := seen[field]; !exists {
			seen[field] = struct{}{}
			fields = append(fields, field)
		}
	}

	for _, child := range normalizeNodeItems(item["items"]) {
		fields = appendFormFieldKeys(fields, seen, child)
	}
	return fields
}

func formFieldKeyFromPath(path string) string {
	path = strings.TrimSpace(path)
	switch {
	case strings.HasPrefix(path, "form."):
		path = strings.TrimPrefix(path, "form.")
	case strings.HasPrefix(path, "data.form."):
		path = strings.TrimPrefix(path, "data.form.")
	default:
		return ""
	}

	field := strings.TrimSpace(strings.Split(path, ".")[0])
	if field == "" || isFormMetaField(field) {
		return ""
	}
	return field
}

func stringSliceToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func applyPageDataDefaults(root map[string]any, nodes map[string][]map[string]any, pathValue string) bool {
	if !usesPageTitleNode(nodes) {
		return false
	}

	title := DefaultModelLabel(pathValue)
	if title == "" {
		return false
	}

	pageData, _ := root["page"].(map[string]any)
	if pageData == nil {
		pageData = map[string]any{}
		root["page"] = pageData
	}
	if strings.TrimSpace(util.ToString(pageData["title"])) != "" {
		return false
	}

	pageData["title"] = title
	return true
}

func usesPageTitleNode(nodes map[string][]map[string]any) bool {
	for _, items := range nodes {
		for _, item := range items {
			if usesPageTitleItem(item) {
				return true
			}
		}
	}
	return false
}

func usesPageTitleItem(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	if util.ToStringTrimmed(item["type"]) == "show-title" && util.ToStringTrimmed(item["value"]) == "page" {
		return true
	}
	for _, child := range normalizeNodeItems(item["items"]) {
		if usesPageTitleItem(child) {
			return true
		}
	}
	return false
}

func applyLinkedPageCategoryDefaults(
	c *server.Context,
	rawLayout json.RawMessage,
	root map[string]any,
	pathValue string,
) (bool, error) {
	if len(rawLayout) == 0 {
		return false, nil
	}

	var layout any
	if err := json.Unmarshal(rawLayout, &layout); err != nil {
		return false, fmt.Errorf("页面 layout 解析失败")
	}

	visited := map[string]bool{
		normalizePath(pathValue): true,
	}
	return applyLinkedLayoutCategoryDefaults(c, layout, root, visited)
}

func applyLinkedLayoutCategoryDefaults(
	c *server.Context,
	layout any,
	root map[string]any,
	visited map[string]bool,
) (bool, error) {
	current, ok := layout.(map[string]any)
	if !ok {
		return false, nil
	}

	changed := false
	if pagePath := normalizeStaticLayoutPagePath(current["path"]); pagePath != "" && !visited[pagePath] {
		visited[pagePath] = true
		applied, err := applyLinkedPageNodeCategoryDefaults(c, pagePath, root, visited)
		if err != nil {
			return false, err
		}
		if applied {
			changed = true
		}
	}

	if children, ok := current["children"].(map[string]any); ok {
		for _, child := range children {
			applied, err := applyLinkedLayoutCategoryDefaults(c, child, root, visited)
			if err != nil {
				return false, err
			}
			if applied {
				changed = true
			}
		}
		return changed, nil
	}

	for key, child := range current {
		switch key {
		case "meta", "page", "nodes", "data", "state", "action":
			continue
		}
		applied, err := applyLinkedLayoutCategoryDefaults(c, child, root, visited)
		if err != nil {
			return false, err
		}
		if applied {
			changed = true
		}
	}
	return changed, nil
}

func applyLinkedPageNodeCategoryDefaults(
	c *server.Context,
	pagePath string,
	root map[string]any,
	visited map[string]bool,
) (bool, error) {
	content, err := ReadContent(pagePath)
	if err != nil {
		return false, err
	}

	linkedSchema, err := parseSchema(pagePath, content)
	if err != nil {
		return false, err
	}

	changed := false
	if applied, err := applyParentCategoryDefaults(c, root, linkedSchema.Nodes); err != nil {
		return false, err
	} else if applied {
		changed = true
	}

	if len(linkedSchema.Layout) > 0 {
		var linkedLayout any
		if err := json.Unmarshal(linkedSchema.Layout, &linkedLayout); err != nil {
			return false, fmt.Errorf("页面 layout 解析失败")
		}
		applied, err := applyLinkedLayoutCategoryDefaults(c, linkedLayout, root, visited)
		if err != nil {
			return false, err
		}
		if applied {
			changed = true
		}
	}

	return changed, nil
}

func applyParentCategoryDefaults(
	c *server.Context,
	root map[string]any,
	rawNodes json.RawMessage,
) (bool, error) {
	if len(rawNodes) == 0 {
		return false, nil
	}

	var nodes map[string][]map[string]any
	if err := json.Unmarshal(rawNodes, &nodes); err != nil {
		return false, fmt.Errorf("页面 nodes 解析失败")
	}

	changed := false
	for _, items := range nodes {
		for _, item := range items {
			applied, err := applyParentDefaultCategoryValue(c, root, item)
			if err != nil {
				return false, err
			}
			if applied {
				changed = true
			}
		}
	}
	return changed, nil
}

func applyParentDefaultCategoryValue(c *server.Context, root map[string]any, item map[string]any) (bool, error) {
	if util.ToStringTrimmed(item["type"]) != "show-category-list" {
		return false, nil
	}

	meta, _ := item["meta"].(map[string]any)
	if !util.ToBool(meta["defaultFirst"]) {
		return false, nil
	}

	valuePath := normalizeParentDataPath(util.ToStringTrimmed(item["value"]))
	if valuePath == "" {
		return false, nil
	}

	queryKey := resolveCategoryDefaultQueryKey(valuePath, meta["queryKey"])
	if queryKey != "" && normalizeQueryInputText(c.Input(queryKey)) != "" {
		return false, nil
	}
	if hasMeaningfulFrontDataPathValue(root, valuePath) {
		return false, nil
	}

	firstValue, ok, err := resolveDefaultCategoryOptionValue(c, root, item["option"])
	if err != nil || !ok {
		return false, err
	}

	if !assignFrontDataPathValue(root, valuePath, firstValue) {
		return false, nil
	}
	return true, nil
}

func normalizeStaticLayoutPagePath(value any) string {
	path := util.ToStringTrimmed(value)
	if path == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(path, "state."):
		return ""
	case strings.HasPrefix(path, "data."):
		return ""
	case strings.HasPrefix(path, "parent."):
		return ""
	}
	return normalizePath(path)
}

func normalizeParentDataPath(path string) string {
	path = strings.TrimSpace(path)
	switch {
	case strings.HasPrefix(path, "parent.data."):
		return strings.TrimPrefix(path, "parent.data.")
	case strings.HasPrefix(path, "parent."):
		return strings.TrimPrefix(path, "parent.")
	default:
		return ""
	}
}

func applySearchDefaults(root map[string]any) bool {
	search, _ := root["search"].(map[string]any)
	if len(search) == 0 {
		return false
	}

	table, _ := root["table"].(map[string]any)
	if len(table) == 0 {
		return false
	}

	filterFields := mapStringSlice(table["filterFields"])
	if len(filterFields) == 0 {
		return false
	}

	defaultFilters, _ := table["defaultFilters"].(map[string]any)
	if defaultFilters == nil {
		defaultFilters = map[string]any{}
	} else {
		defaultFilters = util.CloneMap(defaultFilters)
	}

	changed := false
	for _, field := range filterFields {
		field = strings.TrimSpace(field)
		if field == "" || hasMeaningfulFrontValue(defaultFilters[field]) {
			continue
		}

		value, ok := search[field]
		if !ok || !hasMeaningfulFrontValue(value) {
			continue
		}

		defaultFilters[field] = value
		changed = true
	}

	if changed {
		table["defaultFilters"] = defaultFilters
		root["table"] = table
	}
	return changed
}

func applyDefaultCategoryValue(c *server.Context, root map[string]any, item map[string]any) (bool, error) {
	if util.ToStringTrimmed(item["type"]) != "show-category-list" {
		return false, nil
	}

	meta, _ := item["meta"].(map[string]any)
	if !util.ToBool(meta["defaultFirst"]) {
		return false, nil
	}

	valuePath := util.ToStringTrimmed(item["value"])
	if valuePath == "" {
		return false, nil
	}
	if strings.HasPrefix(valuePath, "parent.") {
		return false, nil
	}

	queryKey := resolveCategoryDefaultQueryKey(valuePath, meta["queryKey"])
	if queryKey != "" && normalizeQueryInputText(c.Input(queryKey)) != "" {
		return false, nil
	}
	if hasMeaningfulFrontDataPathValue(root, valuePath) {
		return false, nil
	}

	firstValue, ok, err := resolveDefaultCategoryOptionValue(c, root, item["option"])
	if err != nil || !ok {
		return false, err
	}

	if !assignFrontDataPathValue(root, valuePath, firstValue) {
		return false, nil
	}
	return true, nil
}

func resolveCategoryDefaultQueryKey(valuePath string, override any) string {
	if customKey := util.ToStringTrimmed(override); customKey != "" {
		return customKey
	}

	keyPath := normalizeDataPath(valuePath)
	segments := strings.Split(strings.TrimSpace(keyPath), ".")
	if len(segments) == 0 {
		return ""
	}
	return strings.TrimSpace(segments[len(segments)-1])
}

func hasMeaningfulFrontDataPathValue(root map[string]any, valuePath string) bool {
	value, ok := lookupFrontDataPathValue(root, valuePath)
	if !ok {
		return false
	}
	return hasMeaningfulFrontValue(value)
}

func hasMeaningfulFrontValue(value any) bool {
	switch current := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(current) != ""
	case []any:
		return len(current) > 0
	case []map[string]any:
		return len(current) > 0
	default:
		return util.ToStringTrimmed(current) != ""
	}
}

func resolveDefaultCategoryOptionValue(c *server.Context, root map[string]any, optionSource any) (any, bool, error) {
	switch current := optionSource.(type) {
	case []any:
		value, ok := extractFirstOptionValue(current)
		return value, ok, nil
	case []map[string]any:
		value, ok := extractFirstMappedOptionValue(current)
		return value, ok, nil
	case string:
		source := strings.TrimSpace(current)
		if source == "" {
			return nil, false, nil
		}
		if !isRemoteOptionURL(source) {
			value, ok := lookupFrontDataPathValue(root, source)
			if !ok {
				return nil, false, nil
			}
			switch typed := value.(type) {
			case []any:
				nextValue, ok := extractFirstOptionValue(typed)
				return nextValue, ok, nil
			case []map[string]any:
				nextValue, ok := extractFirstMappedOptionValue(typed)
				return nextValue, ok, nil
			default:
				return nil, false, nil
			}
		}

		items, err := resolveRemoteDefaultCategoryOptions(c, source)
		if err != nil {
			return nil, false, err
		}
		value, ok := extractFirstMappedOptionValue(items)
		return value, ok, nil
	default:
		return nil, false, nil
	}
}

func resolveRemoteDefaultCategoryOptions(c *server.Context, source string) ([]map[string]any, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil {
		return nil, fmt.Errorf("分类选项地址无效")
	}
	if !strings.HasSuffix(parsed.Path, "/front/route/option") {
		return nil, nil
	}

	queryValues := parsed.Query()
	optionType := strings.ToLower(strings.TrimSpace(queryValues.Get("type")))
	if optionType == "" {
		optionType = "model"
	}
	switch optionType {
	case "model":
		return frontoption.GetModelOptionsByInput(c.Context(), func(key string) string {
			return queryValues.Get(key)
		})
	case "service":
		return frontoption.GetServiceOptionsByInput(c, func(key string) string {
			return queryValues.Get(key)
		})
	default:
		return nil, nil
	}
}

func extractFirstOptionValue(items []any) (any, bool) {
	if len(items) == 0 {
		return nil, false
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return nil, false
	}
	value, exists := first["id"]
	if !exists || value == nil {
		return nil, false
	}
	return value, true
}

func extractFirstMappedOptionValue(items []map[string]any) (any, bool) {
	if len(items) == 0 {
		return nil, false
	}
	return extractFirstOptionValue([]any{items[0]})
}

func normalizeDataPath(path string) string {
	path = strings.TrimSpace(path)
	switch {
	case strings.HasPrefix(path, "data."):
		return strings.TrimPrefix(path, "data.")
	case strings.HasPrefix(path, "state."):
		return ""
	default:
		return path
	}
}

func lookupFrontDataPathValue(root map[string]any, valuePath string) (any, bool) {
	keyPath := normalizeDataPath(valuePath)
	if keyPath == "" {
		return nil, false
	}

	current := any(root)
	for _, segment := range strings.Split(keyPath, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, false
		}
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		nextValue, exists := nextMap[segment]
		if !exists {
			return nil, false
		}
		current = nextValue
	}

	return current, true
}

func assignFrontDataPathValue(root map[string]any, valuePath string, value any) bool {
	keyPath := normalizeDataPath(valuePath)
	if keyPath == "" {
		return false
	}

	segments := strings.Split(keyPath, ".")
	current := root
	for index, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return false
		}
		if index == len(segments)-1 {
			current[segment] = value
			return true
		}

		next, ok := current[segment]
		if !ok {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}

		child, ok := next.(map[string]any)
		if !ok {
			return false
		}
		current = child
	}

	return false
}

func isRemoteOptionURL(source string) bool {
	source = strings.TrimSpace(source)
	return strings.HasPrefix(source, "/") || strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}
