package importer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"
	"github.com/xuri/excelize/v2"

	actionservice "my/package/front/service/action"
	frontcall "my/package/front/service/call"
	frontmeta "my/package/front/service/meta"
	frontoption "my/package/front/service/option"
	frontrecord "my/package/front/service/record"
)

func importWorkbookRows(ctx context.Context, task taskSnapshot, config importConfig) (importSummary, error) {
	_, filePath, err := resolveImportFilePath(ctx, task.FileID)
	if err != nil {
		return importSummary{}, err
	}

	workbook, err := excelize.OpenFile(filePath)
	if err != nil {
		return importSummary{}, fmt.Errorf("读取 Excel 失败: %w", err)
	}
	defer func() {
		_ = workbook.Close()
	}()

	sheetName := strings.TrimSpace(task.SheetName)
	if sheetName == "" {
		sheets := workbook.GetSheetList()
		if len(sheets) == 0 {
			return importSummary{}, fmt.Errorf("Excel 中没有可读取的工作表")
		}
		sheetName = sheets[0]
	}

	rows, err := workbook.GetRows(sheetName)
	if err != nil {
		return importSummary{}, fmt.Errorf("读取工作表失败: %w", err)
	}

	summary := importSummary{
		Errors: make([]map[string]any, 0),
	}
	if len(rows) <= 1 {
		return summary, nil
	}

	model := frontrecord.Resolve(config.Model)
	if model == nil {
		return importSummary{}, fmt.Errorf("导入模型未注册")
	}

	columnLookup := frontrecord.ResolveColumnLookup(config.Model, model)
	optionMap := frontmeta.ResolveModelOptions(ctx, config.Model)
	relationByField := buildRelationMap(config.Model)
	taskInput := decodeTaskInput(task.MappingJSON)
	config = applyImportTaskSettings(config, taskInput)
	fieldByName := buildImportFieldMap(config.Fields)
	uploadCtx := buildImportUploadContext(workbook, filePath, sheetName)
	serverContext := &server.Context{}
	serverContext.SetContext(ctx)

	for rowIndex := 1; rowIndex < len(rows); rowIndex++ {
		row := rows[rowIndex]
		if isEmptyRow(row) {
			continue
		}

		summary.TotalRows += 1
		progress := 5 + summary.TotalRows*90/maxInt(len(rows)-1, 1)
		updateTaskProgress(ctx, task.ID, progress, fmt.Sprintf("正在导入第 %d 行", rowIndex+1))

		record, err := buildImportRecord(
			ctx,
			config.Model,
			rowIndex+1,
			row,
			taskInput.Mappings,
			optionMap,
			relationByField,
			fieldByName,
			columnLookup,
			uploadCtx,
		)
		if err != nil {
			appendImportError(&summary, rowIndex+1, err)
			continue
		}
		if len(record) == 0 {
			appendImportError(&summary, rowIndex+1, fmt.Errorf("未匹配到任何可导入字段"))
			continue
		}

		existing, err := findExistingRecord(ctx, config, record, fieldByName, columnLookup)
		if err != nil {
			appendImportError(&summary, rowIndex+1, err)
			continue
		}
		if len(existing) > 0 {
			record["id"] = existing["id"]
		}

		if _, err := actionservice.SaveModelRecord(serverContext, config.Model, record, "id"); err != nil {
			appendImportError(&summary, rowIndex+1, err)
			continue
		}
		summary.SuccessRows += 1
	}

	summary.FailedRows = summary.TotalRows - summary.SuccessRows
	return summary, nil
}

func buildImportFieldMap(fields []frontmeta.ImportField) map[string]frontmeta.ImportField {
	result := make(map[string]frontmeta.ImportField, len(fields))
	for _, field := range fields {
		result[strings.TrimSpace(field.Field)] = field
	}
	return result
}

func applyImportTaskSettings(config importConfig, input importTaskInput) importConfig {
	if input.MatchFields != nil {
		config.MatchFields = input.MatchFields
	}
	if strings.TrimSpace(input.MatchMode) != "" {
		config.MatchMode = frontmeta.NormalizeImportMatchMode(input.MatchMode)
	}
	fieldSettings := normalizeImportFieldSettings(input.FieldSettings)
	if len(fieldSettings) == 0 || len(config.Fields) == 0 {
		return config
	}

	fields := make([]frontmeta.ImportField, 0, len(config.Fields))
	for _, field := range config.Fields {
		setting, ok := fieldSettings[strings.TrimSpace(field.Field)]
		if ok {
			field.MissingPolicy = frontmeta.NormalizeImportMissingPolicy(field.Kind, setting.MissingPolicy)
			if sourceMode := setting.SourceMode; sourceMode != "" {
				field.SourceMode = sourceMode
			}
			if baseDir := setting.BaseDir; baseDir != "" {
				field.BaseDir = baseDir
			}
		}
		fields = append(fields, field)
	}
	config.Fields = fields
	return config
}

func buildRelationMap(modelName string) map[string]frontmeta.Relation {
	relations := frontmeta.ResolveModelRelations(modelName)
	result := make(map[string]frontmeta.Relation, len(relations))
	for _, relation := range relations {
		result[strings.TrimSpace(relation.Field)] = relation
	}
	return result
}

func buildImportRecord(
	ctx context.Context,
	modelName string,
	rowNumber int,
	row []string,
	mappings []mappingItem,
	optionMap map[string]any,
	relationByField map[string]frontmeta.Relation,
	fieldByName map[string]frontmeta.ImportField,
	columnLookup map[string]string,
	uploadCtx importUploadContext,
) (map[string]any, error) {
	record := make(map[string]any)
	for _, mapping := range mappings {
		fieldName := strings.TrimSpace(mapping.Field)
		if fieldName == "" {
			continue
		}

		field, ok := fieldByName[fieldName]
		if !ok {
			continue
		}

		rawValue := readRowCell(row, mapping.ColumnIndex)
		cellName, _ := excelize.CoordinatesToCellName(mapping.ColumnIndex+1, rowNumber)
		if strings.TrimSpace(rawValue) == "" && strings.ToLower(strings.TrimSpace(field.Kind)) != "upload" {
			continue
		}

		value, err := resolveImportFieldValue(
			ctx,
			modelName,
			field,
			rawValue,
			optionMap,
			relationByField[fieldName],
			columnLookup,
			cellName,
			uploadCtx,
		)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", field.Label, err)
		}
		if value == nil || !frontrecord.HasValue(value) {
			continue
		}
		record[fieldName] = value
	}
	return record, nil
}

func resolveImportFieldValue(
	ctx context.Context,
	modelName string,
	field frontmeta.ImportField,
	rawValue string,
	optionMap map[string]any,
	relation frontmeta.Relation,
	columnLookup map[string]string,
	cellName string,
	uploadCtx importUploadContext,
) (any, error) {
	switch strings.ToLower(strings.TrimSpace(field.Kind)) {
	case "option":
		return resolveOptionFieldValue(optionMap[field.Field], rawValue)
	case "relation":
		return resolveRelationFieldValue(ctx, relation, field, rawValue)
	case "cascade":
		return resolveCascadeFieldValue(ctx, relation, field, rawValue)
	case "children":
		return resolveChildrenFieldValue(relation, field, rawValue)
	case "upload":
		return resolveUploadFieldValue(ctx, modelName, field, rawValue, cellName, uploadCtx)
	case "service":
		return resolveServiceFieldValue(ctx, relation, field, rawValue)
	default:
		columnName := frontrecord.ResolveColumnName(columnLookup, field.Field)
		if strings.HasSuffix(columnName, "_id") {
			id := util.ToUint64(rawValue)
			if id == 0 {
				return nil, fmt.Errorf("值格式无效")
			}
			return id, nil
		}
		return strings.TrimSpace(rawValue), nil
	}
}

func resolveOptionFieldValue(rawOptions any, rawValue string) (any, error) {
	options := normalizeOptionItems(rawOptions)
	normalizedRaw := strings.TrimSpace(rawValue)
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(util.ToString(option["id"])), normalizedRaw) {
			return option["id"], nil
		}
		if strings.EqualFold(strings.TrimSpace(util.ToString(option["value"])), normalizedRaw) {
			return option["id"], nil
		}
		if strings.EqualFold(strings.TrimSpace(util.ToString(option["label"])), normalizedRaw) {
			return option["id"], nil
		}
	}
	return nil, fmt.Errorf("未匹配到可用选项")
}

func normalizeOptionItems(raw any) []map[string]any {
	items := make([]map[string]any, 0)
	switch current := raw.(type) {
	case []map[string]any:
		return current
	default:
		_ = parseJSONValue(current, &items)
		return items
	}
}

func resolveRelationFieldValue(
	ctx context.Context,
	relation frontmeta.Relation,
	field frontmeta.ImportField,
	rawValue string,
) (any, error) {
	model, columnLookup, labelColumn, valueColumn, err := resolveRelationOptionContext(relation)
	if err != nil {
		return nil, err
	}

	values := splitImportValues(rawValue, field.Delimiters)
	ids := make([]any, 0, len(values))
	seen := map[uint64]struct{}{}
	for _, current := range values {
		target := findRelationTarget(ctx, relation.Option, model, labelColumn, valueColumn, current)
		if len(target) == 0 && field.MissingPolicy == "create" {
			record := map[string]any{labelColumn: current}
			frontrecord.ApplyCreatedAt(record, columnLookup)
			insertID := model.Insert(ctx, record)
			if insertID != 0 {
				target = findRelationTargetByValue(ctx, relation.Option, model, valueColumn, insertID)
			}
			if len(target) == 0 {
				target = findRelationTarget(ctx, relation.Option, model, labelColumn, valueColumn, current)
			}
		}
		if len(target) == 0 {
			return nil, fmt.Errorf("未匹配到“%s”", current)
		}

		targetID := util.ToUint64(target[valueColumn])
		if targetID == 0 {
			targetID = util.ToUint64(target["id"])
		}
		if targetID == 0 {
			return nil, fmt.Errorf("关联数据无效")
		}
		if _, exists := seen[targetID]; exists {
			continue
		}
		seen[targetID] = struct{}{}
		ids = append(ids, targetID)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	if field.Multiple || strings.TrimSpace(relation.Mode) == "multiple" {
		return ids, nil
	}
	return ids[0], nil
}

func resolveCascadeFieldValue(
	ctx context.Context,
	relation frontmeta.Relation,
	field frontmeta.ImportField,
	rawValue string,
) (any, error) {
	model, columnLookup, labelColumn, valueColumn, err := resolveRelationOptionContext(relation)
	if err != nil {
		return nil, err
	}

	parentColumn := strings.TrimSpace(field.ParentField)
	if parentColumn == "" {
		parentColumn = "parent_id"
	}
	parentColumn = frontrecord.ResolveColumnName(columnLookup, parentColumn)
	if parentColumn == "" {
		return nil, fmt.Errorf("级联关联缺少父级字段")
	}

	parentValue := field.RootValue
	if parentValue == nil {
		parentValue = 0
	}

	values := splitImportValues(rawValue, field.Delimiters)
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]any, 0, len(values))
	for _, current := range values {
		target := findCascadeTarget(
			ctx,
			relation.Option,
			model,
			parentColumn,
			parentValue,
			labelColumn,
			valueColumn,
			current,
		)
		if len(target) == 0 && field.MissingPolicy == "create" {
			record := map[string]any{
				parentColumn: parentValue,
				labelColumn:  current,
			}
			frontrecord.ApplyCreatedAt(record, columnLookup)
			insertID := model.Insert(ctx, record)
			if insertID != 0 {
				target = findRelationTargetByValue(ctx, relation.Option, model, valueColumn, insertID)
			}
			if len(target) == 0 {
				target = findCascadeTarget(
					ctx,
					relation.Option,
					model,
					parentColumn,
					parentValue,
					labelColumn,
					valueColumn,
					current,
				)
			}
		}
		if len(target) == 0 {
			return nil, fmt.Errorf("未匹配到“%s”", current)
		}

		targetID := util.ToUint64(target[valueColumn])
		if targetID == 0 {
			targetID = util.ToUint64(target["id"])
		}
		if targetID == 0 {
			return nil, fmt.Errorf("级联节点数据无效")
		}

		ids = append(ids, targetID)
		parentValue = targetID
	}

	if len(ids) == 0 {
		return nil, nil
	}
	if field.Multiple || strings.TrimSpace(relation.Mode) == "multiple" {
		return ids, nil
	}
	return ids[len(ids)-1], nil
}

func resolveServiceFieldValue(
	ctx context.Context,
	relation frontmeta.Relation,
	field frontmeta.ImportField,
	rawValue string,
) (any, error) {
	if strings.TrimSpace(field.Use) == "" {
		return nil, fmt.Errorf("未配置导入解析服务")
	}

	serverContext := &server.Context{}
	serverContext.SetContext(ctx)

	result, err := frontcall.Service(
		serverContext,
		field.Use,
		BuildServicePayload(
			ServiceFieldPayload{
				Field:         field.Field,
				Label:         field.Label,
				Kind:          field.Kind,
				Use:           field.Use,
				Multiple:      field.Multiple,
				MissingPolicy: field.MissingPolicy,
				SaveMode:      field.SaveMode,
				UploadKind:    field.UploadKind,
				UploadRuleID:  field.UploadRuleID,
				SourceMode:    field.SourceMode,
				BaseDir:       field.BaseDir,
				Delimiters:    append([]string(nil), field.Delimiters...),
				ParentField:   field.ParentField,
				RootValue:     field.RootValue,
			},
			ServiceRelationPayload{
				Field:            relation.Field,
				Option:           relation.Option,
				Mode:             relation.Mode,
				OwnerField:       relation.OwnerField,
				TargetField:      relation.TargetField,
				OptionValueField: relation.OptionValueField,
				OptionLabelField: relation.OptionLabelField,
			},
			rawValue,
		),
	)
	if err != nil {
		return nil, err
	}
	return UnwrapServiceValue(result), nil
}

func resolveRelationOptionContext(
	relation frontmeta.Relation,
) (frontrecord.Model, map[string]string, string, string, error) {
	if strings.TrimSpace(relation.Option) == "" {
		return nil, nil, "", "", fmt.Errorf("未配置关联模型")
	}

	model := frontrecord.Resolve(relation.Option)
	if model == nil {
		return nil, nil, "", "", fmt.Errorf("关联模型未注册")
	}

	columnLookup := frontrecord.ResolveColumnLookup(relation.Option, model)
	labelColumn := frontrecord.ResolveColumnName(columnLookup, relation.OptionLabelField)
	if labelColumn == "" {
		labelColumn = "name"
	}
	valueColumn := frontrecord.ResolveColumnName(columnLookup, relation.OptionValueField)
	if valueColumn == "" {
		valueColumn = "id"
	}
	return model, columnLookup, labelColumn, valueColumn, nil
}

func findRelationTarget(
	ctx context.Context,
	modelName string,
	model frontrecord.Model,
	labelColumn string,
	valueColumn string,
	current string,
) map[string]any {
	target := model.FindMap(ctx, map[string]any{labelColumn: current})
	if len(target) > 0 {
		return target
	}
	if id := util.ToUint64(current); id != 0 && valueColumn != labelColumn {
		return findRelationTargetByValue(ctx, modelName, model, valueColumn, id)
	}

	if rows := frontoption.SeedRowsByField(modelName, labelColumn, []any{current}); len(rows) > 0 {
		return rows[0]
	}
	if id := util.ToUint64(current); id != 0 && valueColumn != labelColumn {
		if rows := frontoption.SeedRowsByField(modelName, valueColumn, []any{id}); len(rows) > 0 {
			return rows[0]
		}
	}
	return nil
}

func findRelationTargetByValue(
	ctx context.Context,
	modelName string,
	model frontrecord.Model,
	valueColumn string,
	value any,
) map[string]any {
	target := model.FindMap(ctx, map[string]any{valueColumn: value})
	if len(target) > 0 {
		return target
	}
	if rows := frontoption.SeedRowsByField(modelName, valueColumn, []any{value}); len(rows) > 0 {
		return rows[0]
	}
	return nil
}

func findCascadeTarget(
	ctx context.Context,
	modelName string,
	model frontrecord.Model,
	parentColumn string,
	parentValue any,
	labelColumn string,
	valueColumn string,
	current string,
) map[string]any {
	target := model.FindMap(ctx, map[string]any{
		parentColumn: parentValue,
		labelColumn:  current,
	})
	if len(target) > 0 {
		return target
	}

	rows := frontoption.SeedRows(modelName, parentColumn, parentValue)
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(util.ToString(row[labelColumn])), strings.TrimSpace(current)) {
			return row
		}
	}
	if id := util.ToUint64(current); id != 0 && valueColumn != labelColumn {
		for _, row := range rows {
			if util.ToUint64(row[valueColumn]) == id || util.ToUint64(row["id"]) == id {
				return row
			}
		}
	}
	return nil
}

func resolveChildrenFieldValue(
	relation frontmeta.Relation,
	field frontmeta.ImportField,
	rawValue string,
) (any, error) {
	modelName := strings.TrimSpace(relation.Through)
	if modelName == "" {
		return nil, fmt.Errorf("未配置子表模型")
	}

	model := frontrecord.Resolve(modelName)
	if model == nil {
		return nil, fmt.Errorf("子表模型未注册")
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, model)
	valueColumn := resolveImportChildValueColumn(columnLookup, relation)
	if valueColumn == "" {
		return nil, fmt.Errorf("未找到可写入的子项字段")
	}

	values := splitImportValues(rawValue, field.Delimiters)
	if len(values) == 0 {
		return nil, nil
	}

	items := make([]any, 0, len(values))
	sortColumn := frontrecord.ResolveColumnName(columnLookup, "sort")
	for index, value := range values {
		item := map[string]any{
			valueColumn: value,
		}
		if sortColumn != "" {
			item[sortColumn] = index + 1
		}
		items = append(items, item)
	}
	return items, nil
}

func resolveImportChildValueColumn(columnLookup map[string]string, relation frontmeta.Relation) string {
	if len(columnLookup) == 0 {
		return ""
	}

	preferredColumns := []string{"name", "title", "value", "label", "content", "text"}
	for _, field := range preferredColumns {
		if column := frontrecord.ResolveColumnName(columnLookup, field); column != "" {
			return column
		}
	}

	ignored := map[string]struct{}{
		"id":         {},
		"created_at": {},
		"updated_at": {},
		"deleted_at": {},
		"sort":       {},
	}
	if ownerField := strings.TrimSpace(relation.OwnerField); ownerField != "" {
		ignored[ownerField] = struct{}{}
	}

	candidates := make([]string, 0, len(columnLookup))
	for _, columnName := range columnLookup {
		columnName = strings.TrimSpace(columnName)
		if columnName == "" {
			continue
		}
		if _, exists := ignored[columnName]; exists {
			continue
		}
		candidates = append(candidates, columnName)
	}
	if len(candidates) == 0 {
		return ""
	}

	slices.Sort(candidates)
	return candidates[0]
}

func splitImportValues(rawValue string, delimiters []string) []string {
	parts := []string{strings.TrimSpace(rawValue)}
	for _, delimiter := range delimiters {
		next := make([]string, 0, len(parts))
		for _, part := range parts {
			next = append(next, strings.Split(part, delimiter)...)
		}
		parts = next
	}

	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func findExistingRecord(
	ctx context.Context,
	config importConfig,
	record map[string]any,
	fieldByName map[string]frontmeta.ImportField,
	columnLookup map[string]string,
) (map[string]any, error) {
	model := frontrecord.Resolve(config.Model)
	if model == nil {
		return nil, fmt.Errorf("导入模型未注册")
	}

	if strings.ToLower(strings.TrimSpace(config.MatchMode)) == "all" {
		return findExistingRecordByAllFields(ctx, model, config.MatchFields, record, fieldByName, columnLookup)
	}
	return findExistingRecordByAnyField(ctx, model, config.MatchFields, record, columnLookup)
}

func findExistingRecordByAnyField(
	ctx context.Context,
	model frontrecord.Model,
	matchFields []string,
	record map[string]any,
	columnLookup map[string]string,
) (map[string]any, error) {
	existingID := uint64(0)
	var existing map[string]any
	for _, field := range matchFields {
		value, ok := frontrecord.ReadValue(record, field)
		if !ok || !frontrecord.HasValue(value) {
			continue
		}
		row, rowID := findMatchedRow(ctx, model, field, value, columnLookup)
		if rowID == 0 {
			continue
		}
		if existingID != 0 && existingID != rowID {
			return nil, fmt.Errorf("唯一匹配字段命中了不同记录")
		}
		existingID = rowID
		existing = row
	}
	return existing, nil
}

func findExistingRecordByAllFields(
	ctx context.Context,
	model frontrecord.Model,
	matchFields []string,
	record map[string]any,
	fieldByName map[string]frontmeta.ImportField,
	columnLookup map[string]string,
) (map[string]any, error) {
	if len(matchFields) == 0 {
		return nil, nil
	}

	existingID := uint64(0)
	var existing map[string]any
	matchedFields := make([]string, 0, len(matchFields))
	totalComparableFields := 0
	for _, field := range matchFields {
		value, ok := frontrecord.ReadValue(record, field)
		if !ok || !frontrecord.HasValue(value) {
			continue
		}
		totalComparableFields += 1

		row, rowID := findMatchedRow(ctx, model, field, value, columnLookup)
		if rowID == 0 {
			continue
		}
		if existingID != 0 && existingID != rowID {
			return nil, fmt.Errorf("唯一匹配字段命中了不同记录")
		}
		existingID = rowID
		existing = row
		matchedFields = append(matchedFields, field)
	}

	if existingID == 0 {
		return nil, nil
	}

	if totalComparableFields == len(matchFields) && len(matchedFields) == len(matchFields) {
		return existing, nil
	}

	if len(matchedFields) > 0 {
		return nil, fmt.Errorf(
			"匹配字段冲突：%s已存在，但未满足全部匹配字段更新条件",
			strings.Join(resolveImportFieldLabels(matchedFields, fieldByName), "、"),
		)
	}
	return nil, nil
}

func resolveImportFieldLabels(fields []string, fieldByName map[string]frontmeta.ImportField) []string {
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if config, ok := fieldByName[field]; ok && strings.TrimSpace(config.Label) != "" {
			result = append(result, strings.TrimSpace(config.Label))
			continue
		}
		result = append(result, field)
	}
	if len(result) == 0 {
		return fields
	}
	return result
}

func findMatchedRow(
	ctx context.Context,
	model frontrecord.Model,
	field string,
	value any,
	columnLookup map[string]string,
) (map[string]any, uint64) {
	columnName := frontrecord.ResolveColumnName(columnLookup, field)
	if columnName == "" {
		return nil, 0
	}

	row := model.FindMap(ctx, map[string]any{columnName: value})
	if len(row) == 0 {
		return nil, 0
	}

	rowID := util.ToUint64(row["id"])
	if rowID == 0 {
		return nil, 0
	}
	return row, rowID
}

func appendImportError(summary *importSummary, rowNumber int, err error) {
	if summary == nil {
		return
	}
	if summary.Errors == nil {
		summary.Errors = make([]map[string]any, 0)
	}
	if len(summary.Errors) < maxImportErrorSamples {
		summary.Errors = append(summary.Errors, map[string]any{
			"row":     rowNumber,
			"message": normalizeErrorMessage(err),
		})
	}
}

func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
