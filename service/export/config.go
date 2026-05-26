package export

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontcall "my/package/front/service/internal/call"
	frontmeta "my/package/front/service/meta"
	frontpage "my/package/front/service/page"
	frontrecord "my/package/front/service/record"
)

type rawPageSchema struct {
	Page  map[string]any              `json:"page"`
	Nodes map[string][]map[string]any `json:"nodes"`
	Data  map[string]any              `json:"data"`
}

type pageExportCandidate struct {
	TableItem map[string]any
	Export    exportConfig
}

type exportActionSnapshot struct {
	Type         string         `json:"type"`
	Source       string         `json:"source"`
	Scope        string         `json:"scope"`
	Use          string         `json:"use"`
	ExportKey    string         `json:"exportKey"`
	TableID      string         `json:"tableId"`
	Model        string         `json:"model"`
	FileName     string         `json:"fileName"`
	SheetName    string         `json:"sheetName"`
	TemplatePath string         `json:"templatePath"`
	PageSize     int            `json:"pageSize"`
	Filters      map[string]any `json:"filters"`
	Fields       []exportField  `json:"fields"`
	Workbook     *workbookPlan  `json:"workbook"`
	StyleDefs    map[string]any `json:"styleDefs"`
}

func resolveTaskPlan(ctx context.Context, task taskSnapshot) (workbookPlan, error) {
	pageConfig, err := loadPageConfig(task.PagePath, task.TableID, task.ExportKey)
	if err != nil {
		return workbookPlan{}, err
	}

	if pageConfig.Export.Workbook != nil {
		plan := normalizeWorkbookPlan(*pageConfig.Export.Workbook)
		if plan.FileName == "导出结果" {
			plan.FileName = resolveExportFileName(pageConfig.Export, pageConfig.PageTitle)
		}
		if plan.TemplatePath == "" {
			plan.TemplatePath = pageConfig.Export.TemplatePath
		}
		return plan, nil
	}

	exportMode := strings.ToLower(strings.TrimSpace(pageConfig.Export.Mode))
	if exportMode == "" && strings.TrimSpace(pageConfig.Export.Source) == "service" {
		exportMode = "service"
	}
	if exportMode == "" && strings.TrimSpace(pageConfig.Export.Use) != "" {
		exportMode = "service"
	}
	if exportMode == "service" {
		return buildServicePlan(ctx, task, pageConfig)
	}
	return buildGenericPlan(ctx, task, pageConfig)
}

func loadPageConfig(pathValue, tableID, exportKey string) (pageConfigSnapshot, error) {
	content, err := frontpage.ReadContent(pathValue)
	if err != nil {
		return pageConfigSnapshot{}, err
	}

	var payload rawPageSchema
	if err := json.Unmarshal(content, &payload); err != nil {
		return pageConfigSnapshot{}, fmt.Errorf("页面配置解析失败")
	}
	frontpage.ApplyNodeLabelMap(payload.Nodes, pathValue)

	targetTableID := strings.TrimSpace(tableID)
	targetExportKey := strings.TrimSpace(exportKey)
	items, tableMap, orderedTables := flattenPageItems(payload.Nodes)
	candidates, err := collectPageExportCandidates(items, tableMap, orderedTables, targetTableID)
	if err != nil {
		return pageConfigSnapshot{}, err
	}

	matched, ok := matchPageExportCandidate(candidates, targetExportKey)
	if !ok {
		return pageConfigSnapshot{}, fmt.Errorf("未找到可用的导出配置")
	}

	return pageConfigSnapshot{
		PageTitle: strings.TrimSpace(util.FirstNonEmpty(
			util.ToString(payload.Page["name"]),
			util.ToString(payload.Page["title"]),
		)),
		Data:      payload.Data,
		TableItem: matched.TableItem,
		Export:    matched.Export,
	}, nil
}

func flattenPageItems(
	nodes map[string][]map[string]any,
) ([]map[string]any, map[string]map[string]any, []map[string]any) {
	items := make([]map[string]any, 0)
	tableMap := make(map[string]map[string]any)
	orderedTables := make([]map[string]any, 0)

	for layoutID, group := range nodes {
		for index, item := range group {
			itemID := strings.TrimSpace(util.ToString(item["id"]))
			if itemID == "" {
				itemID = fmt.Sprintf("%s-%d", layoutID, index)
				item["id"] = itemID
			}
			items = append(items, item)
			if strings.TrimSpace(util.ToString(item["type"])) != "show-table" {
				continue
			}
			tableMap[itemID] = item
			orderedTables = append(orderedTables, item)
		}
	}

	return items, tableMap, orderedTables
}

func collectPageExportCandidates(
	items []map[string]any,
	tableMap map[string]map[string]any,
	orderedTables []map[string]any,
	targetTableID string,
) ([]pageExportCandidate, error) {
	candidates := make([]pageExportCandidate, 0)

	for _, item := range items {
		currentCandidates, err := collectItemExportCandidates(item, nil, tableMap, orderedTables, targetTableID)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, currentCandidates...)

		children := normalizeNestedItems(item["items"])
		for index, child := range children {
			currentCandidates, err := collectItemExportCandidates(child, map[string]any{
				"id":   item["id"],
				"meta": item["meta"],
				"idx":  index,
			}, tableMap, orderedTables, targetTableID)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, currentCandidates...)
		}
	}

	return candidates, nil
}

func collectItemExportCandidates(
	item map[string]any,
	parent map[string]any,
	tableMap map[string]map[string]any,
	orderedTables []map[string]any,
	targetTableID string,
) ([]pageExportCandidate, error) {
	actionMap, _ := item["action"].(map[string]any)
	clickConfig, ok, err := parseExportAction(actionMap["click"])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	exportKey := resolveExportCandidateKey(item, parent, clickConfig)
	if exportKey == "" {
		return nil, fmt.Errorf("导出项缺少 key")
	}

	exportName := strings.TrimSpace(util.FirstNonEmpty(
		util.ToString(item["name"]),
		util.ToString(item["label"]),
		exportKey,
	))
	parentMeta, _ := parent["meta"].(map[string]any)
	itemMeta, _ := item["meta"].(map[string]any)
	tableItem, include := resolveExportTableItem(
		util.FirstNonEmpty(
			clickConfig.TableID,
			util.ToString(itemMeta["tableId"]),
			util.ToString(parentMeta["tableId"]),
		),
		tableMap,
		orderedTables,
		targetTableID,
	)
	if !include {
		return nil, nil
	}

	return []pageExportCandidate{
		{
			TableItem: tableItem,
			Export: exportConfig{
				Key:          exportKey,
				Name:         exportName,
				Source:       strings.TrimSpace(clickConfig.Source),
				Scope:        strings.TrimSpace(clickConfig.Scope),
				Use:          strings.TrimSpace(clickConfig.Use),
				Model:        strings.TrimSpace(clickConfig.Model),
				FileName:     strings.TrimSpace(clickConfig.FileName),
				SheetName:    strings.TrimSpace(clickConfig.SheetName),
				TemplatePath: strings.TrimSpace(clickConfig.TemplatePath),
				PageSize:     clickConfig.PageSize,
				Filters:      normalizeExportQueryMap(clickConfig.Filters),
				Fields:       clickConfig.Fields,
				Workbook:     clickConfig.Workbook,
				StyleDefs:    clickConfig.StyleDefs,
			},
		},
	}, nil
}

func resolveExportTableItem(
	configuredTableID string,
	tableMap map[string]map[string]any,
	orderedTables []map[string]any,
	targetTableID string,
) (map[string]any, bool) {
	configuredTableID = strings.TrimSpace(configuredTableID)
	if configuredTableID != "" {
		if targetTableID != "" && configuredTableID != targetTableID {
			return nil, false
		}
		return tableMap[configuredTableID], true
	}

	if targetTableID != "" {
		return tableMap[targetTableID], true
	}

	if len(orderedTables) == 1 {
		return orderedTables[0], true
	}

	return nil, true
}

func matchPageExportCandidate(
	candidates []pageExportCandidate,
	targetExportKey string,
) (pageExportCandidate, bool) {
	if len(candidates) == 0 {
		return pageExportCandidate{}, false
	}

	if targetExportKey != "" {
		for _, candidate := range candidates {
			if candidate.Export.Key == targetExportKey {
				return candidate, true
			}
		}
		return pageExportCandidate{}, false
	}

	if len(candidates) == 1 {
		return candidates[0], true
	}

	return pageExportCandidate{}, false
}

func parseExportAction(raw any) (exportActionSnapshot, bool, error) {
	if raw == nil {
		return exportActionSnapshot{}, false, nil
	}

	switch current := raw.(type) {
	case string:
		return exportActionSnapshot{}, false, nil
	case []any:
		for _, item := range current {
			config, ok, err := parseExportAction(item)
			if err != nil {
				return exportActionSnapshot{}, false, err
			}
			if ok {
				return config, true, nil
			}
		}
		return exportActionSnapshot{}, false, nil
	case map[string]any, map[string]string:
		var config exportActionSnapshot
		if err := decodeJSONValue(current, &config); err != nil {
			return exportActionSnapshot{}, false, fmt.Errorf("导出动作配置格式错误")
		}

		if strings.ToLower(strings.TrimSpace(config.Type)) != "export" {
			return exportActionSnapshot{}, false, nil
		}

		return config, true, nil
	}

	return exportActionSnapshot{}, false, nil
}

func normalizeNestedItems(raw any) []map[string]any {
	list := make([]map[string]any, 0)
	_ = decodeJSONValue(raw, &list)
	return list
}

func resolveExportCandidateKey(
	item map[string]any,
	parent map[string]any,
	action exportActionSnapshot,
) string {
	if key := strings.TrimSpace(action.ExportKey); key != "" {
		return key
	}

	if key := strings.TrimSpace(util.ToString(item["key"])); key != "" {
		return key
	}

	if key := strings.TrimSpace(util.ToString(item["id"])); key != "" {
		return key
	}

	parentID := strings.TrimSpace(util.ToString(parent["id"]))
	index := util.ToIntDefault(parent["idx"], -1)
	if parentID != "" && index >= 0 {
		return fmt.Sprintf("%s_%d", parentID, index+1)
	}

	return ""
}

func buildGenericPlan(ctx context.Context, task taskSnapshot, pageConfig pageConfigSnapshot) (workbookPlan, error) {
	sourceMode := normalizeExportSource(pageConfig.Export)
	tableMeta := map[string]any{}
	tableValuePath := ""
	if pageConfig.TableItem != nil {
		tableMeta, _ = pageConfig.TableItem["meta"].(map[string]any)
		tableValuePath = util.ToString(pageConfig.TableItem["value"])
	}
	tableConfig := resolveTableConfig(pageConfig.Data, tableValuePath)
	modelName := strings.TrimSpace(util.FirstNonEmpty(
		pageConfig.Export.Model,
		util.ToString(tableMeta["model"]),
		frontpage.DefaultModelName(task.PagePath),
	))
	if modelName == "" {
		return workbookPlan{}, fmt.Errorf("导出模型未配置")
	}
	if frontrecord.Resolve(modelName) == nil {
		return workbookPlan{}, fmt.Errorf("导出模型未注册")
	}

	fields := pageConfig.Export.Fields
	if len(fields) == 0 {
		switch sourceMode {
		case "model":
			fields = buildExportFieldsFromModel(ctx, modelName)
		default:
			fields = buildExportFieldsFromColumns(tableMeta["columns"])
		}
	}
	if len(fields) == 0 {
		return workbookPlan{}, fmt.Errorf("导出字段不能为空")
	}

	pageSize := pageConfig.Export.PageSize
	if pageSize <= 0 {
		pageSize = defaultExportPageSize
	}

	plan := workbookPlan{
		FileName:     resolveExportFileName(pageConfig.Export, pageConfig.PageTitle),
		TemplatePath: pageConfig.Export.TemplatePath,
		Sheets: []sheetPlan{
			{
				Name:       resolveExportSheetName(pageConfig.Export, pageConfig.PageTitle),
				StartCell:  "A1",
				Stream:     true,
				Freeze:     "A2",
				AutoFilter: true,
				Head:       fields,
				Source: &sheetSource{
					Mode:     "generic",
					Model:    modelName,
					PageSize: pageSize,
					Query:    resolveExportQuery(task.Query, pageConfig.Export),
					Table:    tableConfig,
					Payload: map[string]any{
						"option_labels": buildOptionLabelMap(ctx, modelName),
						"source":        sourceMode,
						"page_path":     task.PagePath,
					},
				},
				Styles: sheetStyleRefs{
					Header: "header",
					Body:   "body",
				},
			},
		},
	}
	return normalizeWorkbookPlan(plan), nil
}

func buildServicePlan(ctx context.Context, task taskSnapshot, pageConfig pageConfigSnapshot) (workbookPlan, error) {
	serviceName := strings.TrimSpace(pageConfig.Export.Use)
	if serviceName == "" {
		return workbookPlan{}, fmt.Errorf("导出服务不能为空")
	}

	serverContext := &server.Context{}
	serverContext.SetContext(ctx)
	result, err := frontcall.Service(serverContext, serviceName, map[string]any{
		"task":       taskPayload(task),
		"query":      resolveExportQuery(task.Query, pageConfig.Export),
		"page_path":  task.PagePath,
		"table_id":   task.TableID,
		"export_key": task.ExportKey,
		"page_title": pageConfig.PageTitle,
		"table":      pageConfig.TableItem,
		"data":       pageConfig.Data,
	})
	if err != nil {
		return workbookPlan{}, err
	}

	plan, err := parseServiceWorkbookPlan(result)
	if err != nil {
		return workbookPlan{}, err
	}
	if strings.TrimSpace(plan.FileName) == "" {
		plan.FileName = resolveExportFileName(pageConfig.Export, pageConfig.PageTitle)
	}
	if strings.TrimSpace(plan.TemplatePath) == "" {
		plan.TemplatePath = pageConfig.Export.TemplatePath
	}
	return normalizeWorkbookPlan(plan), nil
}

func parseServiceWorkbookPlan(raw any) (workbookPlan, error) {
	var plan workbookPlan
	if err := decodeJSONValue(raw, &plan); err == nil && len(plan.Sheets) > 0 {
		return plan, nil
	}

	var singleSheet sheetPlan
	if err := decodeJSONValue(raw, &singleSheet); err == nil && (len(singleSheet.Head) > 0 || len(singleSheet.Body) > 0) {
		return workbookPlan{
			FileName: "导出结果",
			Sheets:   []sheetPlan{singleSheet},
		}, nil
	}

	var page pageResult
	if err := decodeJSONValue(raw, &page); err == nil && (len(page.Head) > 0 || len(page.Body) > 0) {
		return workbookPlan{
			FileName: "导出结果",
			Sheets: []sheetPlan{
				{
					Name: "导出结果",
					Head: page.Head,
					Body: page.Body,
				},
			},
		}, nil
	}

	return workbookPlan{}, fmt.Errorf("导出服务返回格式不支持")
}

func resolveTableConfig(data map[string]any, valuePath string) map[string]any {
	for _, candidate := range tableConfigCandidates(valuePath) {
		current, ok := lookupMapPath(data, candidate)
		if ok && len(current) > 0 {
			return util.CloneMap(current)
		}
	}
	return map[string]any{}
}

func tableConfigCandidates(valuePath string) []string {
	valuePath = strings.TrimSpace(valuePath)
	if valuePath == "" {
		return []string{"table"}
	}
	valuePath = strings.TrimPrefix(valuePath, "data.")
	if strings.HasSuffix(valuePath, ".list") {
		return []string{strings.TrimSuffix(valuePath, ".list"), "table"}
	}
	parts := strings.Split(valuePath, ".")
	if len(parts) > 1 {
		return []string{strings.Join(parts[:len(parts)-1], "."), valuePath, "table"}
	}
	return []string{valuePath, "table"}
}

func lookupMapPath(root map[string]any, path string) (map[string]any, bool) {
	path = strings.TrimSpace(strings.TrimPrefix(path, "data."))
	if path == "" {
		return nil, false
	}

	current := any(root)
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, false
		}
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := mapped[segment]
		if !exists {
			return nil, false
		}
		current = next
	}

	mapped, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}
	return mapped, true
}

func buildExportFieldsFromColumns(raw any) []exportField {
	columns := make([]map[string]any, 0)
	_ = decodeJSONValue(raw, &columns)
	result := make([]exportField, 0, len(columns))
	for _, column := range columns {
		valuePath := strings.TrimSpace(util.ToString(column["value"]))
		if valuePath == "" {
			continue
		}
		columnType := strings.TrimSpace(util.ToString(column["type"]))

		field := exportField{
			Key:   valuePath,
			Value: valuePath,
			Title: strings.TrimSpace(util.ToString(column["name"])),
			Width: inferExportFieldWidth(strings.TrimSpace(util.ToString(column["name"])), columnType),
		}

		meta, _ := column["meta"].(map[string]any)
		if formatValue := strings.ToLower(strings.TrimSpace(util.ToString(meta["format"]))); formatValue != "" {
			field.Format = formatValue
		}
		switch columnType {
		case "show-tag":
			field.Format = "join"
			field.Field = strings.TrimSpace(util.FirstNonEmpty(util.ToString(meta["field"]), "name"))
		case "show-select", "show-status":
			field.Format = "option"
		}

		result = append(result, normalizeExportField(field))
	}
	return result
}

func buildExportFieldsFromModel(ctx context.Context, modelName string) []exportField {
	modelValue := frontrecord.Resolve(modelName)
	if modelValue == nil {
		return nil
	}

	columnOrder := frontrecord.ResolveOrderedColumns(modelName, modelValue)
	if len(columnOrder) == 0 {
		columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
		if len(columnLookup) == 0 {
			return nil
		}
		columnOrder = make([]string, 0, len(columnLookup))
		for _, column := range columnLookup {
			columnOrder = append(columnOrder, column)
		}
	}

	optionLabels := buildOptionLabelMap(ctx, modelName)
	relations := frontmeta.ResolveModelRelations(modelName)
	relationByField := make(map[string]frontmeta.Relation, len(relations))
	for _, relation := range relations {
		field := strings.TrimSpace(relation.Field)
		if field == "" {
			continue
		}
		relationByField[field] = relation
	}

	fields := make([]exportField, 0, len(columnOrder))
	usedRelationFields := make(map[string]struct{}, len(relations))
	for _, column := range columnOrder {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		if relation, ok := relationByField[column]; ok {
			if field, ok := buildRelationExportField(modelName, relation); ok {
				fields = append(fields, field)
				usedRelationFields[column] = struct{}{}
				continue
			}
		}

		title := strings.TrimSpace(util.FirstNonEmpty(frontmeta.ResolveFieldLabel(modelName, column), column))
		field := exportField{
			Key:   column,
			Value: column,
			Title: title,
			Width: inferExportFieldWidth(title, ""),
		}
		if _, ok := optionLabels[column]; ok {
			field.Format = "option"
		} else if isDatetimeExportField(column) {
			field.Format = "datetime"
		} else if isDateExportField(column) {
			field.Format = "date"
		}
		fields = append(fields, normalizeExportField(field))
	}

	for _, relation := range relations {
		fieldName := strings.TrimSpace(relation.Field)
		if fieldName == "" {
			continue
		}
		if _, ok := usedRelationFields[fieldName]; ok {
			continue
		}
		if field, ok := buildRelationExportField(modelName, relation); ok {
			fields = append(fields, field)
		}
	}
	return fields
}

func buildRelationExportField(modelName string, relation frontmeta.Relation) (exportField, bool) {
	valuePath := strings.TrimSpace(relation.Name)
	if valuePath == "" {
		return exportField{}, false
	}

	title := strings.TrimSpace(util.FirstNonEmpty(
		frontmeta.ResolveFieldLabel(modelName, relation.Field),
		frontmeta.ResolveFieldLabel(modelName, relation.Name),
		valuePath,
	))
	displayField := resolveExportRelationDisplayField(relation)
	field := exportField{
		Key:   strings.TrimSpace(relation.Field),
		Title: title,
		Width: inferExportFieldWidth(title, "show-tag"),
	}

	if relation.Kind == "children" || relation.Mode == "multiple" {
		field.Value = valuePath
		field.Format = "join"
		field.Field = strings.TrimSpace(util.ToString(util.FirstNonEmpty(displayField, "name")))
		return normalizeExportField(field), true
	}

	if displayField != "" {
		field.Value = valuePath + "." + displayField
	} else {
		field.Value = valuePath
	}
	if isDatetimeExportField(field.Value) {
		field.Format = "datetime"
	}
	return normalizeExportField(field), true
}

func resolveExportRelationDisplayField(relation frontmeta.Relation) string {
	if isUploadExportRelation(relation) {
		return "url"
	}

	for _, modelName := range []string{
		strings.TrimSpace(relation.Option),
		strings.TrimSpace(relation.Through),
	} {
		if modelName == "" {
			continue
		}
		modelValue := frontrecord.Resolve(modelName)
		columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
		for _, candidate := range []string{"name", "title", "value", "label", "path", "content", "text"} {
			if frontrecord.ResolveColumnName(columnLookup, candidate) != "" {
				return candidate
			}
		}
	}
	return ""
}

func isUploadExportRelation(relation frontmeta.Relation) bool {
	return strings.TrimSpace(relation.Option) == "front.NewUploadFileModel"
}

func isDatetimeExportField(field string) bool {
	field = strings.TrimSpace(field)
	switch {
	case strings.HasSuffix(field, "_at"),
		strings.HasSuffix(field, ".created_at"),
		strings.HasSuffix(field, ".updated_at"),
		strings.HasSuffix(field, ".finished_at"),
		strings.HasSuffix(field, ".started_at"):
		return true
	default:
		return false
	}
}

func isDateExportField(field string) bool {
	field = strings.TrimSpace(field)
	switch {
	case strings.HasSuffix(field, "_date"),
		strings.HasSuffix(field, ".date"):
		return true
	default:
		return false
	}
}

func inferExportFieldWidth(title string, columnType string) float64 {
	title = strings.TrimSpace(title)
	switch strings.TrimSpace(columnType) {
	case "show-tag":
		return 24
	case "show-select", "show-status":
		return 14
	}

	width := len([]rune(title))*2 + 10
	if width < 12 {
		width = 12
	}
	if width > 28 {
		width = 28
	}
	return float64(width)
}

func resolveExportFileName(config exportConfig, pageTitle string) string {
	return strings.TrimSpace(util.FirstNonEmpty(config.FileName, config.Name, pageTitle, "导出结果"))
}

func resolveExportSheetName(config exportConfig, pageTitle string) string {
	return strings.TrimSpace(util.FirstNonEmpty(config.SheetName, config.Name, pageTitle, "导出结果"))
}

func normalizeExportSource(config exportConfig) string {
	source := strings.ToLower(strings.TrimSpace(config.Source))
	switch source {
	case "model", "service":
		return source
	}
	return "table"
}

func normalizeExportScope(config exportConfig) string {
	scope := strings.ToLower(strings.TrimSpace(config.Scope))
	switch scope {
	case "all", "fixed":
		return scope
	}
	return "current"
}

func resolveExportQuery(current map[string]string, config exportConfig) map[string]string {
	switch normalizeExportScope(config) {
	case "all":
		return map[string]string{}
	case "fixed":
		return cloneStringMap(config.Filters)
	default:
		return cloneStringMap(current)
	}
}

func normalizeExportQueryMap(raw any) map[string]string {
	if raw == nil {
		return map[string]string{}
	}

	switch current := raw.(type) {
	case map[string]string:
		return cloneStringMap(current)
	case map[string]any:
		result := make(map[string]string, len(current))
		for key, value := range current {
			key = strings.TrimSpace(key)
			if key == "" || value == nil {
				continue
			}
			switch typed := value.(type) {
			case string:
				if text := strings.TrimSpace(typed); text != "" {
					result[key] = text
				}
			default:
				data, err := json.Marshal(value)
				if err != nil {
					continue
				}
				if text := strings.TrimSpace(string(data)); text != "" && text != "null" {
					result[key] = text
				}
			}
		}
		return result
	default:
		return map[string]string{}
	}
}

func buildOptionLabelMap(ctx context.Context, modelName string) map[string]map[string]string {
	options := frontmeta.ResolveModelOptions(ctx, modelName)
	if len(options) == 0 {
		return map[string]map[string]string{}
	}

	result := make(map[string]map[string]string, len(options))
	for key, raw := range options {
		items := normalizeOptionItems(raw)
		if len(items) == 0 {
			continue
		}
		labels := make(map[string]string, len(items))
		for _, item := range items {
			id := strings.TrimSpace(util.ToString(item["id"]))
			if id == "" {
				continue
			}
			label := strings.TrimSpace(util.FirstNonEmpty(
				util.ToString(item["value"]),
				util.ToString(item["label"]),
				util.ToString(item["name"]),
			))
			if label == "" {
				label = id
			}
			labels[id] = label
		}
		if len(labels) > 0 {
			result[strings.TrimSpace(key)] = labels
		}
	}
	return result
}

func normalizeOptionItems(raw any) []map[string]any {
	switch current := raw.(type) {
	case []map[string]any:
		return current
	case []any:
		result := make([]map[string]any, 0, len(current))
		for _, item := range current {
			mapped, _ := item.(map[string]any)
			if mapped != nil {
				result = append(result, mapped)
			}
		}
		return result
	default:
		return nil
	}
}

func lastPathSegment(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

func loadGenericPageResult(ctx context.Context, source *sheetSource, page int) (pageResult, error) {
	if source == nil {
		return pageResult{}, fmt.Errorf("导出数据源不能为空")
	}
	modelValue := frontrecord.Resolve(strings.TrimSpace(source.Model))
	if modelValue == nil {
		return pageResult{}, fmt.Errorf("导出模型未注册")
	}

	query := cloneStringMap(source.Query)
	query["page"] = fmt.Sprintf("%d", page)
	query["pageSize"] = fmt.Sprintf("%d", source.PageSize)
	rows, total, _, _, err := frontpage.QueryModelListWithQuery(ctx, modelValue, source.Table, query)
	if err != nil {
		return pageResult{}, err
	}
	rows = frontmeta.AttachRelations(ctx, strings.TrimSpace(source.Model), rows)
	rows = frontmeta.HideFields(strings.TrimSpace(source.Model), rows)
	rows, err = applyGenericRowService(ctx, source, rows)
	if err != nil {
		return pageResult{}, err
	}
	return pageResult{
		Body:  rows,
		Total: total,
	}, nil
}

func applyGenericRowService(
	ctx context.Context,
	source *sheetSource,
	rows []map[string]any,
) ([]map[string]any, error) {
	if source == nil || len(rows) == 0 {
		return rows, nil
	}

	serviceName := strings.TrimSpace(util.ToString(source.Table["service"]))
	if serviceName == "" {
		return rows, nil
	}

	serverContext := &server.Context{}
	serverContext.SetContext(ctx)
	result, err := frontcall.Service(serverContext, serviceName, map[string]any{
		"path":      util.ToString(source.Payload["page_path"]),
		"container": util.CloneMap(source.Table),
		"rows":      rows,
	})
	if err != nil {
		return nil, err
	}

	if normalized := normalizeServiceRows(result); normalized != nil {
		return normalized, nil
	}
	if mapped, ok := result.(map[string]any); ok {
		if normalized := normalizeServiceRows(mapped["rows"]); normalized != nil {
			return normalized, nil
		}
	}
	return rows, nil
}

func normalizeServiceRows(value any) []map[string]any {
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

func loadServicePageResult(ctx context.Context, task taskSnapshot, sheet sheetPlan, source *sheetSource, page int) (pageResult, error) {
	if source == nil || strings.TrimSpace(source.Service) == "" {
		return pageResult{}, fmt.Errorf("导出服务不能为空")
	}

	serverContext := &server.Context{}
	serverContext.SetContext(ctx)
	result, err := frontcall.Service(serverContext, strings.TrimSpace(source.Service), map[string]any{
		"task":      taskPayload(task),
		"page":      page,
		"page_size": source.PageSize,
		"query":     cloneStringMap(source.Query),
		"source":    source,
		"sheet":     sheet,
		"payload":   util.CloneMap(source.Payload),
	})
	if err != nil {
		return pageResult{}, err
	}

	var pageResultValue pageResult
	if err := decodeJSONValue(result, &pageResultValue); err != nil {
		return pageResult{}, fmt.Errorf("导出服务分页返回格式错误")
	}
	return pageResultValue, nil
}
