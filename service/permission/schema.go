package permission

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	"github.com/shemic/dever/util"

	frontpagepath "my/package/front/internal/pagepath"
	frontpage "my/package/front/service/page"
	embedpageservice "my/package/front/service/permission/embedpage"
	"my/package/front/service/siteconfig"
)

type pageSchemaPermissionFilter struct {
	ctx      context.Context
	snapshot *accessSnapshot
	lookup   InputLookup
	query    map[string]string
	pageName string
	pagePath string
	nodes    map[string][]map[string]any
	data     map[string]any
	state    map[string]any
	action   map[string]any

	routeCache   map[string]bool
	authCache    map[string]bool
	actionCache  map[string]bool
	overlayCache map[string]map[string]any
}

type pageSchemaPermissionContext struct {
	row     map[string]any
	patches map[string]any
	itemKey string
}

func (scope *AccessScope) FilterPageSchema(ctx context.Context, pathValue string, currentSchema *frontpage.Schema, lookup InputLookup, query map[string]string) error {
	if scope == nil {
		return nil
	}
	return filterPageSchemaWithSnapshot(ctx, scope.snapshot, pathValue, currentSchema, lookup, query)
}

func shouldSkipSchemaPermissionFilter(snapshot *accessSnapshot) bool {
	return snapshot == nil || hasDefaultRole(snapshot.roleIDs)
}

func filterPageSchemaWithSnapshot(ctx context.Context, snapshot *accessSnapshot, pathValue string, currentSchema *frontpage.Schema, lookup InputLookup, query map[string]string) error {
	if currentSchema == nil || len(currentSchema.Nodes) == 0 {
		return nil
	}
	if shouldSkipSchemaPermissionFilter(snapshot) {
		return nil
	}

	filter := pageSchemaPermissionFilter{
		ctx:          ctx,
		snapshot:     snapshot,
		lookup:       lookup,
		query:        query,
		pageName:     siteconfig.PageFromContext(ctx),
		pagePath:     frontpagepath.NormalizePath(pathValue),
		nodes:        decodeSchemaNodes(currentSchema.Nodes),
		data:         decodeSchemaObject(currentSchema.Data),
		state:        decodeSchemaObject(currentSchema.State),
		action:       decodeSchemaObject(currentSchema.Action),
		routeCache:   map[string]bool{},
		authCache:    map[string]bool{},
		actionCache:  map[string]bool{},
		overlayCache: map[string]map[string]any{},
	}
	if len(filter.nodes) == 0 {
		return nil
	}

	for layoutID, items := range filter.nodes {
		filter.nodes[layoutID] = filter.filterItems(items, pageSchemaPermissionContext{})
	}

	content, err := json.Marshal(filter.nodes)
	if err != nil {
		return err
	}
	currentSchema.Nodes = json.RawMessage(content)
	return nil
}

func decodeSchemaNodes(raw json.RawMessage) map[string][]map[string]any {
	var nodes map[string][]map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &nodes) != nil {
		return nil
	}
	return nodes
}

func decodeSchemaObject(raw json.RawMessage) map[string]any {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	return payload
}

func (filter pageSchemaPermissionFilter) filterItems(items []map[string]any, context pageSchemaPermissionContext) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if filter.filterItem(item, context) {
			result = append(result, item)
		}
	}
	return result
}

func (filter pageSchemaPermissionFilter) filterItem(item map[string]any, context pageSchemaPermissionContext) bool {
	if len(item) == 0 || !filter.canUseItem(item, context) {
		return false
	}

	if children, ok := mapSlice(item["items"]); ok {
		item["items"] = filter.filterItems(children, context)
	}
	filter.filterCategoryListActions(item)
	filter.filterMetaButtons(item, context)
	filter.filterTableButtons(item)

	if schemaType(item) == "show-button-group" {
		items, _ := mapSlice(item["items"])
		return len(items) > 0
	}
	return true
}

func (filter pageSchemaPermissionFilter) filterMetaButtons(item map[string]any, context pageSchemaPermissionContext) {
	meta, ok := schemaMeta(item)
	if !ok {
		return
	}

	if button, ok := meta["createButton"].(map[string]any); ok {
		if filter.canUseItem(button, context) {
			meta["createButton"] = button
		} else {
			delete(meta, "createButton")
		}
	}

	buttons, ok := mapSlice(meta["buttons"])
	if !ok {
		return
	}

	nextButtons := make([]map[string]any, 0, len(buttons))
	for index, button := range buttons {
		buttonContext := context
		if isCategoryListItem(item) {
			buttonContext.row = permissionProbeRow(categoryListRowKey(item))
		}
		buttonContext.itemKey = itemKey(button, index)
		if filter.canUseItem(button, buttonContext) {
			nextButtons = append(nextButtons, button)
		}
	}
	meta["buttons"] = nextButtons
}

func (filter pageSchemaPermissionFilter) filterCategoryListActions(item map[string]any) {
	if !isCategoryListItem(item) {
		return
	}

	meta, ok := schemaMeta(item)
	if !ok {
		return
	}

	statusAction, ok := meta["statusChangeAction"]
	if !ok || statusAction == nil {
		return
	}

	pathValue := filter.inlineActionPath(statusAction)
	query := filter.inlineActionQuery(
		pathValue,
		categoryListRowKey(item),
		"__permission_probe__",
	)
	if pathValue == "" || filter.canUseRoute(pathValue, query) {
		return
	}
	delete(meta, "statusChangeAction")
}

func (filter pageSchemaPermissionFilter) filterTableButtons(item map[string]any) {
	if schemaType(item) != "show-table" {
		return
	}

	meta, ok := schemaMeta(item)
	if !ok {
		return
	}
	columns, ok := mapSlice(meta["columns"])
	if !ok {
		return
	}

	rows := filter.tableRows(item)
	rowKey := schemaRowKey(meta)
	filter.filterInlineTableEdit(meta, columns, rows, rowKey)

	nextColumns := make([]map[string]any, 0, len(columns))
	for _, column := range columns {
		if !isActionColumn(column) {
			nextColumns = append(nextColumns, column)
			continue
		}

		if filter.filterActionColumn(column, rows, rowKey) {
			nextColumns = append(nextColumns, column)
		}
	}
	meta["columns"] = nextColumns
}

func (filter pageSchemaPermissionFilter) filterInlineTableEdit(
	tableMeta map[string]any,
	columns []map[string]any,
	rows []map[string]any,
	rowKey string,
) {
	defaultAction := filter.defaultInlineAction(tableMeta)
	defaultChecked := false
	defaultAllowed := true

	for _, column := range columns {
		if !isInlineEditableColumn(column) {
			continue
		}

		if changeAction, ok := inlineChangeAction(column); ok {
			if !filter.canUseInlineAction(changeAction, rows, rowKey) {
				disableInlineColumnEdit(column)
			}
			continue
		}

		if len(defaultAction) == 0 {
			continue
		}
		if !defaultChecked {
			defaultAllowed = filter.canUseInlineAction(defaultAction, rows, rowKey)
			defaultChecked = true
		}
		if !defaultAllowed {
			tableMeta["inlineEditable"] = false
		}
	}
}

func (filter pageSchemaPermissionFilter) filterActionColumn(column map[string]any, rows []map[string]any, rowKey string) bool {
	meta := schemaMetaValue(column)
	buttons, ok := mapSlice(meta["buttons"])
	if !ok || len(buttons) == 0 {
		return filter.canUseTableItem(column, rows, rowKey)
	}

	nextButtons := make([]map[string]any, 0, len(buttons))
	for index, button := range buttons {
		buttonContext := pageSchemaPermissionContext{itemKey: itemKey(button, index)}
		if filter.canUseTableItem(button, rows, rowKey, buttonContext) {
			nextButtons = append(nextButtons, button)
		}
	}
	meta["buttons"] = nextButtons
	return len(nextButtons) > 0
}

func (filter pageSchemaPermissionFilter) canUseTableItem(item map[string]any, rows []map[string]any, rowKey string, baseContext ...pageSchemaPermissionContext) bool {
	context := pageSchemaPermissionContext{}
	if len(baseContext) > 0 {
		context = baseContext[0]
	}

	if len(rows) == 0 {
		context.row = permissionProbeRow(rowKey)
		return filter.canUseItem(item, context)
	}

	for _, row := range rows {
		context.row = row
		if filter.canUseItem(item, context) {
			return true
		}
	}
	return false
}

func (filter pageSchemaPermissionFilter) defaultInlineAction(tableMeta map[string]any) map[string]any {
	pathValue := resolveInlineSavePath(tableMeta["savePath"], filter.pagePath)
	if pathValue == "" {
		return nil
	}
	return map[string]any{
		"type": "save",
		"path": pathValue,
	}
}

func (filter pageSchemaPermissionFilter) canUseInlineAction(action any, rows []map[string]any, rowKey string) bool {
	pathValue := filter.inlineActionPath(action)
	if pathValue == "" {
		return true
	}

	if len(rows) == 0 {
		query := filter.inlineActionQuery(pathValue, rowKey, "__permission_probe__")
		return filter.canUseRoute(pathValue, query)
	}

	for _, row := range rows {
		rowID := rowSearchValue(row, rowKey)
		if isEmptyInlineRowID(rowID) {
			continue
		}
		query := filter.inlineActionQuery(pathValue, rowKey, rowID)
		if filter.canUseRoute(pathValue, query) {
			return true
		}
	}
	return false
}

func (filter pageSchemaPermissionFilter) inlineActionQuery(
	pathValue string,
	rowKey string,
	rowID any,
) map[string]string {
	query := inlineEditQuery(rowKey, rowID)
	query = mergeRouteQuery(query, filter.inheritedRouteQuery(pathValue))
	return query
}

func (filter pageSchemaPermissionFilter) inlineActionPath(raw any) string {
	switch action := raw.(type) {
	case string:
		actionName := strings.TrimSpace(action)
		if actionName == "" {
			return ""
		}
		return filter.inlineActionPath(filter.action[actionName])
	case []any:
		for _, current := range action {
			if pathValue := filter.inlineActionPath(current); pathValue != "" {
				return pathValue
			}
		}
	case map[string]any:
		switch schemaLowerType(action) {
		case "save", "delete":
			return resolveInlineSavePath(action["path"], filter.pagePath)
		}
	}
	return ""
}

func (filter pageSchemaPermissionFilter) canUseItem(item map[string]any, context pageSchemaPermissionContext) bool {
	if !filter.canUseDeclaredAuth(item["auth"]) {
		return false
	}

	if to := itemRoute(item); to != "" {
		route, query := parseRoute(to)
		if searchParam := itemSearchParam(item); searchParam != "" && len(context.row) > 0 {
			if query == nil {
				query = map[string]string{}
			}
			query[searchParam] = util.ToString(rowSearchValue(context.row, itemSearchField(item)))
		}
		return filter.canUseRoute(route, query)
	}

	actionMap, _ := item["action"].(map[string]any)
	if click, ok := actionMap["click"]; ok {
		return filter.canUseAction(click, item, context)
	}
	return true
}

func (filter pageSchemaPermissionFilter) canUseAction(raw any, owner map[string]any, context pageSchemaPermissionContext) bool {
	switch action := raw.(type) {
	case string:
		actionName := strings.TrimSpace(action)
		if actionName == "" {
			return true
		}
		config, ok := filter.action[actionName]
		if !ok {
			return true
		}
		if context.itemKey == "" {
			context.itemKey = actionName
		}
		return filter.canUseAction(config, owner, context)
	case []any:
		nextContext := context
		nextContext.patches = map[string]any{}
		for _, current := range action {
			if collectActionPatch(current, nextContext.patches, nextContext) {
				continue
			}
			if !filter.canUseAction(current, owner, nextContext) {
				return false
			}
		}
		return true
	case map[string]any:
		return filter.canUseActionConfig(action, owner, context)
	default:
		return true
	}
}

func (filter pageSchemaPermissionFilter) canUseActionConfig(action map[string]any, owner map[string]any, context pageSchemaPermissionContext) bool {
	switch schemaLowerType(action) {
	case "modal":
		return filter.canUseOverlay(util.ToString(action["key"]), context)
	case "export", "import", "delete":
		key := actionAuthKey(action, owner, context.itemKey)
		key = normalizeActionPermissionKey(key)
		if key == "" {
			return true
		}
		return filter.canUseActionKey(key)
	default:
		return true
	}
}

func (filter pageSchemaPermissionFilter) canUseOverlay(stateKey string, context pageSchemaPermissionContext) bool {
	overlay := filter.findOverlay(stateKey)
	if len(overlay) == 0 {
		return true
	}

	meta := schemaMetaValue(overlay)
	if route := filter.resolveOverlayRoute(meta, context); route != "" {
		pathValue, query := parseRoute(route)
		query = mergeRouteQuery(query, resolveRouteQuery(meta["pageRouteQuery"], filter.data, filter.state, context))
		query = mergeRouteQuery(query, filter.inheritedRouteQuery(pathValue))
		return filter.canUseRoute(pathValue, query)
	}

	actionMap, _ := overlay["action"].(map[string]any)
	if confirm, ok := actionMap["confirm"]; ok {
		return filter.canUseAction(confirm, overlay, context)
	}
	return true
}

func (filter pageSchemaPermissionFilter) inheritedRouteQuery(childPath string) map[string]string {
	parentPath := frontpagepath.NormalizePath(filter.pagePath)
	childPath = frontpagepath.NormalizePath(childPath)
	if parentPath == "" || childPath == "" || parentPath == childPath {
		return nil
	}
	if !embedpageservice.IsChildForPage(filter.pageName, parentPath, childPath) {
		return nil
	}
	query := map[string]string{
		inheritInputKey:      "form",
		inheritParentPathKey: parentPath,
	}
	for key, value := range filter.query {
		appendInheritedQueryValue(query, key, value)
	}
	for key, value := range routeQueryStrings(filter.state) {
		appendInheritedQueryValue(query, key, value)
	}
	return query
}

func appendInheritedQueryValue(query map[string]string, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if query == nil || key == "" || value == "" {
		return
	}
	query[inheritParentInputPrefix+key] = value
}

func (filter pageSchemaPermissionFilter) resolveOverlayRoute(meta map[string]any, context pageSchemaPermissionContext) string {
	if route := strings.TrimSpace(util.ToString(meta["pageRoute"])); route != "" {
		return route
	}

	pathRef := strings.TrimSpace(util.ToString(meta["pageRoutePath"]))
	if pathRef == "" {
		return ""
	}

	resolved := resolveSchemaValue(pathRef, filter.data, filter.state, context)
	if route := strings.TrimSpace(util.ToString(resolved)); route != "" && route != pathRef {
		return route
	}

	pathRef = strings.TrimPrefix(pathRef, "state.")
	return strings.TrimSpace(util.ToString(valueAtPath(filter.state, pathRef)))
}

func (filter pageSchemaPermissionFilter) findOverlay(stateKey string) map[string]any {
	stateKey = strings.TrimSpace(stateKey)
	if stateKey == "" {
		return nil
	}
	if cached, ok := filter.overlayCache[stateKey]; ok {
		return cached
	}
	for _, items := range filter.nodes {
		for _, item := range items {
			if matched := findOverlayInItem(item, stateKey); len(matched) > 0 {
				filter.overlayCache[stateKey] = matched
				return matched
			}
		}
	}
	filter.overlayCache[stateKey] = nil
	return nil
}

func findOverlayInItem(item map[string]any, stateKey string) map[string]any {
	meta := schemaMetaValue(item)
	if strings.TrimSpace(util.ToString(meta["stateKey"])) == stateKey {
		return item
	}
	children, ok := mapSlice(item["items"])
	if !ok {
		return nil
	}
	for _, child := range children {
		if matched := findOverlayInItem(child, stateKey); len(matched) > 0 {
			return matched
		}
	}
	return nil
}

func (filter pageSchemaPermissionFilter) canUseDeclaredAuth(auth any) bool {
	keys := requiredAuthKeys(auth)
	if len(keys) == 0 {
		return true
	}
	for _, key := range keys {
		allowed, ok := filter.authCache[key]
		if !ok {
			row, exists := filter.snapshot.auth.rowByKey[key]
			allowed = exists && canAccessAuthRow(filter.snapshot, row)
			filter.authCache[key] = allowed
		}
		if allowed {
			return true
		}
	}
	return false
}

func (filter pageSchemaPermissionFilter) canUseRoute(route string, query map[string]string) bool {
	pathValue := frontpagepath.NormalizePath(route)
	if pathValue == "" {
		return true
	}
	cacheKey := routeAccessCacheKey(pathValue, query)
	if allowed, ok := filter.routeCache[cacheKey]; ok {
		return allowed
	}
	row, protected := resolveAccessAuthRow(filter.snapshot.auth, pathValue, func(key string) string {
		return query[key]
	})
	if !protected {
		allowed := filter.canUseInheritedRoute(pathValue, query)
		filter.routeCache[cacheKey] = allowed
		return allowed
	}
	allowed := canAccessAuthRow(filter.snapshot, row)
	if !allowed && filter.canUseInheritedRoute(pathValue, query) {
		allowed = true
	}
	filter.routeCache[cacheKey] = allowed
	return allowed
}

func (filter pageSchemaPermissionFilter) canUseInheritedRoute(childPath string, query map[string]string) bool {
	if filter.snapshot == nil || query == nil || !allowsInheritedAccess(query[inheritInputKey]) {
		return false
	}
	parentPath := filter.pagePath
	parentPath = frontpagepath.NormalizePath(util.FirstNonEmpty(query[inheritParentPathKey], parentPath))
	childPath = frontpagepath.NormalizePath(childPath)
	if parentPath == "" || childPath == "" || parentPath == childPath {
		return false
	}
	if !embedpageservice.IsChildForPage(filter.pageName, parentPath, childPath) {
		return false
	}
	return ensurePageAccessWithSnapshot(filter.ctx, filter.snapshot, parentPath, chainInheritedLookup(query, filter.lookup)) == nil
}

func chainInheritedLookup(query map[string]string, fallback InputLookup) InputLookup {
	return func(key string) string {
		value := ""
		if query != nil {
			value = strings.TrimSpace(query[inheritParentInputPrefix+strings.TrimSpace(key)])
		}
		if value != "" {
			return value
		}
		if fallback != nil {
			return fallback(key)
		}
		return ""
	}
}

func (filter pageSchemaPermissionFilter) canUseActionKey(actionKey string) bool {
	fullKey := filter.pagePath + "/" + normalizeActionPermissionKey(actionKey)
	if allowed, ok := filter.actionCache[fullKey]; ok {
		return allowed
	}
	row, ok := filter.snapshot.auth.rowByKey[fullKey]
	if !ok || len(row) == 0 {
		filter.actionCache[fullKey] = false
		return false
	}
	allowed := canAccessAuthRow(filter.snapshot, row)
	filter.actionCache[fullKey] = allowed
	return allowed
}

func collectActionPatch(raw any, patches map[string]any, context pageSchemaPermissionContext) bool {
	action, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	actionType := schemaLowerType(action)
	if actionType != "data" && actionType != "state" {
		return false
	}
	key := strings.TrimSpace(util.ToString(action["key"]))
	if key != "" {
		value := resolveContextValue(action["value"], context)
		patches[key] = value
		patches[actionType+"."+key] = value
	}
	return true
}

func resolveRouteQuery(raw any, data map[string]any, state map[string]any, context pageSchemaPermissionContext) map[string]string {
	payload, ok := raw.(map[string]any)
	if !ok || len(payload) == 0 {
		return nil
	}
	query := make(map[string]string, len(payload))
	for key, value := range payload {
		resolved := resolveSchemaValue(value, data, state, context)
		if text := strings.TrimSpace(util.ToString(resolved)); text != "" {
			query[key] = text
		}
	}
	return query
}

func routeQueryStrings(state map[string]any) map[string]string {
	route, _ := state["route"].(map[string]any)
	if len(route) == 0 {
		return nil
	}
	query, _ := route["query"].(map[string]any)
	if len(query) == 0 {
		return nil
	}
	result := make(map[string]string, len(query))
	for key, value := range query {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if text := strings.TrimSpace(util.ToString(value)); text != "" {
			result[key] = text
		}
	}
	return result
}

func resolveSchemaValue(value any, data map[string]any, state map[string]any, context pageSchemaPermissionContext) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	if context.patches != nil {
		if value, ok := context.patches[text]; ok {
			return value
		}
	}
	if isContextValue(text) {
		return resolveContextValue(text, context)
	}
	if strings.HasPrefix(text, "data.") {
		return valueAtPath(data, strings.TrimPrefix(text, "data."))
	}
	if strings.HasPrefix(text, "state.") {
		return valueAtPath(state, strings.TrimPrefix(text, "state."))
	}
	return value
}

func resolveContextValue(value any, context pageSchemaPermissionContext) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	if text == "$row" {
		return context.row
	}
	if strings.HasPrefix(text, "$row.") {
		return valueAtPath(context.row, strings.TrimPrefix(text, "$row."))
	}
	return value
}

func isContextValue(value string) bool {
	return value == "$row" || strings.HasPrefix(value, "$row.")
}

func requiredAuthKeys(auth any) []string {
	switch value := auth.(type) {
	case string:
		return nonEmptyStrings(value)
	case []any:
		keys := make([]string, 0, len(value))
		for _, item := range value {
			keys = append(keys, nonEmptyStrings(util.ToString(item))...)
		}
		return keys
	default:
		return nil
	}
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func schemaType(item map[string]any) string {
	return strings.TrimSpace(util.ToString(item["type"]))
}

func schemaLowerType(item map[string]any) string {
	return strings.ToLower(schemaType(item))
}

func schemaMeta(item map[string]any) (map[string]any, bool) {
	meta, ok := item["meta"].(map[string]any)
	return meta, ok
}

func schemaMetaValue(item map[string]any) map[string]any {
	meta, _ := schemaMeta(item)
	return meta
}

func ensureSchemaMeta(item map[string]any) map[string]any {
	meta := schemaMetaValue(item)
	if meta == nil {
		meta = map[string]any{}
		item["meta"] = meta
	}
	return meta
}

func schemaRowKey(meta map[string]any) string {
	rowKey := strings.TrimSpace(util.ToString(meta["rowKey"]))
	if rowKey == "" {
		return "id"
	}
	return rowKey
}

func isActionColumn(column map[string]any) bool {
	switch schemaType(column) {
	case "show-button", "show-link":
		return true
	default:
		return false
	}
}

func isCategoryListItem(item map[string]any) bool {
	return schemaType(item) == "show-category-list"
}

func categoryListRowKey(item map[string]any) string {
	return schemaRowKey(schemaMetaValue(item))
}

func permissionProbeRow(rowKey string) map[string]any {
	row := map[string]any{"id": "__permission_probe__"}
	if rowKey = strings.TrimSpace(rowKey); rowKey != "" {
		row[rowKey] = "__permission_probe__"
	}
	return row
}

func isInlineEditableColumn(column map[string]any) bool {
	if _, ok := inlineChangeAction(column); ok {
		return true
	}
	columnType := schemaType(column)
	switch columnType {
	case "form-input", "form-number", "form-select", "form-switch":
		return true
	case "show-status", "show-select":
		meta := schemaMetaValue(column)
		return meta["editable"] != false
	}
	if _, ok := column["trigger"]; ok {
		return true
	}
	switch editor := column["editor"].(type) {
	case bool:
		return editor
	case string:
		return strings.TrimSpace(editor) != ""
	default:
		meta := schemaMetaValue(column)
		editorMeta, _ := meta["editor"].(map[string]any)
		return schemaType(editorMeta) != ""
	}
}

func inlineChangeAction(column map[string]any) (any, bool) {
	actionMap, _ := column["action"].(map[string]any)
	change, ok := actionMap["change"]
	return change, ok && change != nil
}

func disableInlineColumnEdit(column map[string]any) {
	ensureSchemaMeta(column)["inlineEditable"] = false
}

func inlineEditQuery(rowKey string, rowID any) map[string]string {
	query := map[string]string{"id": util.ToString(rowID)}
	rowKey = strings.TrimSpace(rowKey)
	if rowKey != "" && rowKey != "id" {
		query[rowKey] = util.ToString(rowID)
	}
	return query
}

func isEmptyInlineRowID(value any) bool {
	text := strings.TrimSpace(util.ToString(value))
	if text == "" {
		return true
	}
	if number, ok := util.ParseInt64(text); ok && number <= 0 {
		return true
	}
	return false
}

func resolveInlineSavePath(path any, currentPagePath string) string {
	if pathValue := normalizeInlineSavePath(path); pathValue != "" {
		return pathValue
	}

	currentPath := normalizeInlineSavePath(currentPagePath)
	if currentPath == "" {
		return ""
	}
	if strings.HasSuffix(currentPath, "/list") {
		return strings.TrimSuffix(currentPath, "/list") + "/update"
	}
	return currentPath
}

func normalizeInlineSavePath(path any) string {
	pathValue := util.ToString(path)
	pathValue, _, _ = strings.Cut(pathValue, "?")
	pathValue, _, _ = strings.Cut(pathValue, "#")
	return frontpagepath.NormalizePath(pathValue)
}

func mapSlice(value any) ([]map[string]any, bool) {
	switch items := value.(type) {
	case []map[string]any:
		return items, true
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if mapped, ok := item.(map[string]any); ok {
				result = append(result, mapped)
			}
		}
		return result, true
	default:
		return nil, false
	}
}

func (filter pageSchemaPermissionFilter) tableRows(item map[string]any) []map[string]any {
	valuePath := strings.TrimSpace(util.ToString(item["value"]))
	rows, _ := mapSlice(valueAtPath(filter.data, strings.TrimPrefix(valuePath, "data.")))
	return rows
}

func itemRoute(item map[string]any) string {
	if route := strings.TrimSpace(util.ToString(item["to"])); route != "" {
		return route
	}
	meta := schemaMetaValue(item)
	return strings.TrimSpace(util.ToString(meta["to"]))
}

func itemSearchParam(item map[string]any) string {
	if value := strings.TrimSpace(util.ToString(item["searchParam"])); value != "" {
		return value
	}
	meta := schemaMetaValue(item)
	return strings.TrimSpace(util.ToString(meta["searchParam"]))
}

func itemSearchField(item map[string]any) string {
	if value := strings.TrimSpace(util.ToString(item["searchField"])); value != "" {
		return value
	}
	meta := schemaMetaValue(item)
	return strings.TrimSpace(util.ToString(meta["searchField"]))
}

func rowSearchValue(row map[string]any, field string) any {
	if field == "" {
		return row["id"]
	}
	return valueAtPath(row, field)
}

func itemKey(item map[string]any, index int) string {
	for _, key := range []string{"key", "id"} {
		if value := strings.TrimSpace(util.ToString(item[key])); value != "" {
			return value
		}
	}
	return ""
}

func valueAtPath(payload map[string]any, pathValue string) any {
	if len(payload) == 0 || strings.TrimSpace(pathValue) == "" {
		return nil
	}
	current := any(payload)
	for _, part := range strings.Split(pathValue, ".") {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapped[strings.TrimSpace(part)]
	}
	return current
}

func parseRoute(route string) (string, map[string]string) {
	pathValue, rawQuery, _ := strings.Cut(strings.TrimSpace(route), "?")
	values, err := url.ParseQuery(rawQuery)
	if err != nil || len(values) == 0 {
		return pathValue, nil
	}
	query := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) > 0 {
			query[key] = items[0]
		}
	}
	return pathValue, query
}

func mergeRouteQuery(base map[string]string, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = map[string]string{}
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func routeAccessCacheKey(pathValue string, query map[string]string) string {
	if len(query) == 0 {
		return pathValue
	}

	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.Grow(len(pathValue) + len(keys)*8)
	builder.WriteString(pathValue)
	builder.WriteByte('?')
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(query[key])
	}
	return builder.String()
}
