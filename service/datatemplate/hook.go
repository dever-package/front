package datatemplate

import (
	"context"
	"regexp"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmodel "github.com/dever-package/front/model"
	frontaction "github.com/dever-package/front/service/action"
)

type DataTemplateHook struct{}

var dataTemplateKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

var optionFieldTypes = map[string]struct{}{
	"radio":        {},
	"checkbox":     {},
	"select":       {},
	"multi_select": {},
}

var allowedFieldTypes = map[string]struct{}{
	"text":         {},
	"textarea":     {},
	"editor":       {},
	"date":         {},
	"datetime":     {},
	"boolean":      {},
	"radio":        {},
	"checkbox":     {},
	"select":       {},
	"multi_select": {},
	"image":        {},
	"video":        {},
	"audio":        {},
}

var uploadFieldTypes = map[string]struct{}{
	"image": {},
	"video": {},
	"audio": {},
}

func (DataTemplateHook) ProviderBeforeSaveDataTarget(_ *server.Context, params []any) any {
	record := cloneDataTemplateRecord(params)
	if len(record) == 0 {
		return record
	}
	partial := isPartialDataTemplateRecord(record)
	trimDataTemplateString(record, "name", partial)
	trimDataTemplateString(record, "model_name", partial)
	trimDataTemplateString(record, "table_key", partial)
	trimDataTemplateString(record, "primary_key", partial)
	trimDataTemplateString(record, "label_field", partial)
	if !partial {
		requireDataTemplateText(record, "name", "form.name", "名称不能为空。")
		requireDataTemplateText(record, "model_name", "form.model_name", "Model不能为空。")
	}
	defaultDataTemplateText(record, "primary_key", "id", partial)
	defaultDataTemplateText(record, "label_field", "name", partial)
	defaultDataTemplateStatus(record, partial)
	defaultDataTemplateSort(record, partial)
	defaultDataTemplateCreatedAt(record)
	return record
}

func (DataTemplateHook) ProviderBeforeSaveDataTemplateCate(_ *server.Context, params []any) any {
	record := cloneDataTemplateRecord(params)
	if len(record) == 0 {
		return record
	}
	partial := isPartialDataTemplateRecord(record)
	trimDataTemplateString(record, "name", partial)
	trimDataTemplateString(record, "description", partial)
	if !partial {
		requireDataTemplateText(record, "name", "form.name", "分类名称不能为空。")
	}
	defaultDataTemplateUint64(record, "target_id", 0, partial)
	defaultDataTemplateStatus(record, partial)
	defaultDataTemplateSort(record, partial)
	defaultDataTemplateCreatedAt(record)
	return record
}

func (DataTemplateHook) ProviderBeforeSaveDataTemplate(c *server.Context, params []any) any {
	record := cloneDataTemplateRecord(params)
	if len(record) == 0 {
		return record
	}
	partial := isPartialDataTemplateRecord(record)
	trimDataTemplateString(record, "name", partial)
	trimDataTemplateString(record, "template_key", partial)
	if !partial {
		if util.ToUint64(record["cate_id"]) == 0 {
			panic(frontaction.NewFieldError("form.cate_id", "模板分类不能为空。"))
		}
		requireDataTemplateText(record, "name", "form.name", "模板名称不能为空。")
		requireDataTemplateText(record, "template_key", "form.template_key", "模板Key不能为空。")
	}
	normalizeDataTemplateKey(c, record, partial)
	if _, exists := record["fields"]; exists {
		record["fields"] = normalizeDataTemplateFields(record["fields"])
		ensureSubmittedFieldKeysUnique(dataTemplateRows(record["fields"]))
		ensureDataTemplateFieldsMutable(c, util.ToUint64(record["id"]), dataTemplateRows(record["fields"]))
	}
	defaultDataTemplateStatus(record, partial)
	defaultDataTemplateSort(record, partial)
	defaultDataTemplateCreatedAt(record)
	return record
}

func (DataTemplateHook) ProviderBeforeSaveDataField(c *server.Context, params []any) any {
	record := cloneDataTemplateRecord(params)
	if len(record) == 0 {
		return record
	}
	partial := isPartialDataTemplateRecord(record)
	normalizeDataFieldRecord(record, partial, 0)
	ensureDataFieldMutable(c, record)
	ensureDataFieldKeyUnique(c, record)
	return record
}

func (DataTemplateHook) ProviderBeforeSaveDataFieldOption(c *server.Context, params []any) any {
	record := cloneDataTemplateRecord(params)
	if len(record) == 0 {
		return record
	}
	partial := isPartialDataTemplateRecord(record)
	trimDataTemplateString(record, "name", partial)
	trimDataTemplateString(record, "value", partial)
	if !partial {
		requireDataTemplateText(record, "name", "form.name", "选项名不能为空。")
		requireDataTemplateText(record, "value", "form.value", "选项值不能为空。")
	}
	defaultDataTemplateSort(record, partial)
	ensureDataFieldOptionMutable(c, record)
	return record
}

func (DataTemplateHook) ProviderBeforeDeleteDataTarget(c *server.Context, params []any) any {
	payload := cloneDataTemplateRecord(params)
	targetID := deletePayloadID(payload)
	if targetID == 0 {
		panic("扩展数据表不存在")
	}
	ctx := dataTemplateContext(c)
	if frontmodel.NewDataTemplateCateModel().Count(ctx, map[string]any{"target_id": targetID}) > 0 {
		panic("当前扩展数据表已被模板分类使用，请先调整分类后再删除")
	}
	if frontmodel.NewDataRecordModel().Count(ctx, map[string]any{"target_id": targetID}) > 0 {
		panic("当前扩展数据表已有模板数据，请先清理数据后再删除")
	}
	return map[string]any{"id": targetID}
}

func (DataTemplateHook) ProviderBeforeDeleteDataTemplateCate(c *server.Context, params []any) any {
	payload := cloneDataTemplateRecord(params)
	cateID := deletePayloadID(payload)
	if cateID == 0 {
		panic("模板分类不存在")
	}
	if frontmodel.NewDataTemplateModel().Count(dataTemplateContext(c), map[string]any{"cate_id": cateID}) > 0 {
		panic("当前分类下已有数据模板，请先移动或删除模板后再删除分类")
	}
	return map[string]any{"id": cateID}
}

func (DataTemplateHook) ProviderBeforeDeleteDataTemplate(c *server.Context, params []any) any {
	payload := cloneDataTemplateRecord(params)
	templateID := deletePayloadID(payload)
	if templateID == 0 {
		panic("数据模板不存在")
	}
	ctx := dataTemplateContext(c)
	if frontmodel.NewDataRecordModel().Count(ctx, map[string]any{"data_template_id": templateID}) > 0 {
		panic("当前模板已有填写数据，请先清理数据后再删除模板")
	}
	deleteTemplateFields(ctx, templateID)
	return map[string]any{"id": templateID}
}

func (DataTemplateHook) ProviderBeforeDeleteDataField(c *server.Context, params []any) any {
	payload := cloneDataTemplateRecord(params)
	fieldID := deletePayloadID(payload)
	if fieldID == 0 {
		panic("数据字段不存在")
	}
	ctx := dataTemplateContext(c)
	field := frontmodel.NewDataFieldModel().FindMap(ctx, map[string]any{"id": fieldID})
	if dataTemplateHasRecords(ctx, util.ToUint64(field["data_template_id"])) {
		panic("当前模板已有填写数据，不能删除字段")
	}
	frontmodel.NewDataFieldOptionModel().Delete(ctx, map[string]any{"data_field_id": fieldID})
	return map[string]any{"id": fieldID}
}

func (DataTemplateHook) ProviderBeforeDeleteDataFieldOption(c *server.Context, params []any) any {
	payload := cloneDataTemplateRecord(params)
	optionID := deletePayloadID(payload)
	if optionID == 0 {
		panic("字段选项不存在")
	}
	ctx := dataTemplateContext(c)
	option := frontmodel.NewDataFieldOptionModel().FindMap(ctx, map[string]any{"id": optionID})
	field := frontmodel.NewDataFieldModel().FindMap(ctx, map[string]any{
		"id": util.ToUint64(option["data_field_id"]),
	})
	if dataTemplateHasRecords(ctx, util.ToUint64(field["data_template_id"])) {
		panic("当前模板已有填写数据，不能删除字段选项")
	}
	return map[string]any{"id": optionID}
}

func (DataTemplateHook) ProviderBuildDataTemplateForm(c *server.Context, params []any) any {
	record := dataTemplateFormRecord(params)
	if len(record) == 0 {
		return record
	}
	attachDataFieldOptions(c, record)
	return record
}

func dataTemplateContext(c *server.Context) context.Context {
	if c != nil {
		return c.Context()
	}
	return context.Background()
}

func deletePayloadID(payload map[string]any) uint64 {
	id := util.ToUint64(payload["raw_id"])
	if id == 0 {
		id = util.ToUint64(payload["id"])
	}
	return id
}

func cloneDataTemplateRecord(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	record, _ := params[0].(map[string]any)
	if record == nil {
		return map[string]any{}
	}
	return util.CloneMap(record)
}

func isPartialDataTemplateRecord(record map[string]any) bool {
	return util.ToBool(record["_partial"])
}

func trimDataTemplateString(record map[string]any, field string, partial bool) {
	if partial {
		if _, exists := record[field]; !exists {
			return
		}
	}
	record[field] = util.ToStringTrimmed(record[field])
}

func requireDataTemplateText(record map[string]any, field string, formPath string, message string) {
	if util.ToStringTrimmed(record[field]) == "" {
		panic(frontaction.NewFieldError(formPath, message))
	}
}

func defaultDataTemplateText(record map[string]any, field string, fallback string, partial bool) {
	if partial {
		if _, exists := record[field]; !exists {
			return
		}
	}
	if util.ToStringTrimmed(record[field]) == "" {
		record[field] = fallback
	}
}

func defaultDataTemplateUint64(record map[string]any, field string, fallback uint64, partial bool) {
	if partial {
		if _, exists := record[field]; !exists {
			return
		}
	}
	if util.ToUint64(record[field]) == 0 {
		record[field] = fallback
	}
}

func defaultDataTemplateStatus(record map[string]any, partial bool) {
	if partial {
		if _, exists := record["status"]; !exists {
			return
		}
	}
	status := util.ToIntDefault(record["status"], frontmodel.DataStatusEnabled)
	if status != frontmodel.DataStatusDisabled {
		status = frontmodel.DataStatusEnabled
	}
	record["status"] = status
}

func defaultDataTemplateSort(record map[string]any, partial bool) {
	if partial {
		if _, exists := record["sort"]; !exists {
			return
		}
	}
	sort := util.ToIntDefault(record["sort"], 100)
	if sort <= 0 {
		sort = 100
	}
	record["sort"] = sort
}

func defaultDataTemplateCreatedAt(record map[string]any) {
	if util.ToUint64(record["id"]) > 0 {
		return
	}
	if _, exists := record["created_at"]; !exists {
		record["created_at"] = time.Now()
	}
}

func normalizeDataTemplateFields(rawFields any) []map[string]any {
	rows := dataTemplateRows(rawFields)
	fields := make([]map[string]any, 0, len(rows))
	for index, row := range rows {
		if blankDataTemplateFieldRow(row) {
			continue
		}
		field := util.CloneMap(row)
		normalizeDataFieldRecord(field, false, (index+1)*10)
		fields = append(fields, field)
	}
	return fields
}

func normalizeDataFieldRecord(record map[string]any, partial bool, fallbackSort int) {
	trimDataTemplateString(record, "name", partial)
	trimDataTemplateString(record, "field_key", partial)
	trimDataTemplateString(record, "field_type", partial)
	trimDataTemplateString(record, "default_value", partial)
	trimDataTemplateString(record, "placeholder", partial)
	trimDataTemplateString(record, "help_text", partial)
	if !partial {
		requireDataTemplateText(record, "name", "form.name", "字段名称不能为空。")
		requireDataTemplateText(record, "field_key", "form.field_key", "字段Key不能为空。")
	}
	normalizeDataFieldKey(record, partial)
	fieldType := util.ToStringTrimmed(record["field_type"])
	if fieldType == "" {
		fieldType = "text"
	}
	if _, ok := allowedFieldTypes[fieldType]; !ok {
		panic(frontaction.NewFieldError("form.field_type", "字段类型不正确。"))
	}
	record["field_type"] = fieldType
	if _, exists := record["required"]; !exists && !partial {
		record["required"] = false
	}
	if _, ok := uploadFieldTypes[fieldType]; ok {
		maxCount := util.ToIntDefault(record["max_count"], 1)
		if maxCount <= 0 {
			maxCount = 1
		}
		record["max_count"] = maxCount
	} else if !partial || record["max_count"] != nil {
		record["max_count"] = 0
	}
	if _, ok := optionFieldTypes[fieldType]; ok {
		if _, exists := record["options"]; exists {
			record["options"] = normalizeDataFieldOptions(record["options"])
		}
	} else if !partial || record["options"] != nil {
		record["options"] = []map[string]any{}
	}
	defaultDataTemplateStatus(record, partial)
	if fallbackSort > 0 && util.ToIntDefault(record["sort"], 0) <= 0 {
		record["sort"] = fallbackSort
	} else {
		defaultDataTemplateSort(record, partial)
	}
	defaultDataTemplateCreatedAt(record)
}

func normalizeDataFieldOptions(rawOptions any) []map[string]any {
	rows := dataTemplateRows(rawOptions)
	options := make([]map[string]any, 0, len(rows))
	seen := map[string]struct{}{}
	for index, row := range rows {
		if blankDataFieldOptionRow(row) {
			continue
		}
		name := util.ToStringTrimmed(row["name"])
		value := util.ToStringTrimmed(row["value"])
		if name == "" {
			panic(frontaction.NewFieldError("form.options", "选项名不能为空。"))
		}
		if value == "" {
			panic(frontaction.NewFieldError("form.options", "选项值不能为空。"))
		}
		if _, exists := seen[value]; exists {
			panic(frontaction.NewFieldError("form.options", "选项值不能重复。"))
		}
		seen[value] = struct{}{}
		option := util.CloneMap(row)
		option["name"] = name
		option["value"] = value
		option["sort"] = util.ToIntDefault(row["sort"], (index+1)*10)
		options = append(options, option)
	}
	return options
}

func dataTemplateRows(value any) []map[string]any {
	switch rows := value.(type) {
	case nil:
		return nil
	case []map[string]any:
		return rows
	case []any:
		result := make([]map[string]any, 0, len(rows))
		for _, item := range rows {
			row, ok := item.(map[string]any)
			if !ok || row == nil {
				continue
			}
			result = append(result, row)
		}
		return result
	default:
		return []map[string]any{}
	}
}

func blankDataTemplateFieldRow(row map[string]any) bool {
	return util.ToUint64(row["id"]) == 0 &&
		util.ToStringTrimmed(row["name"]) == "" &&
		util.ToStringTrimmed(row["field_key"]) == "" &&
		util.ToStringTrimmed(row["field_type"]) == "" &&
		util.ToStringTrimmed(row["default_value"]) == "" &&
		util.ToStringTrimmed(row["placeholder"]) == "" &&
		util.ToStringTrimmed(row["help_text"]) == "" &&
		len(dataTemplateRows(row["options"])) == 0
}

func blankDataFieldOptionRow(row map[string]any) bool {
	return util.ToUint64(row["id"]) == 0 &&
		util.ToStringTrimmed(row["name"]) == "" &&
		util.ToStringTrimmed(row["value"]) == ""
}

func dataTemplateFormRecord(params []any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	payload, ok := params[0].(map[string]any)
	if !ok {
		return nil
	}
	if record, ok := payload["record"].(map[string]any); ok {
		return util.CloneMap(record)
	}
	return util.CloneMap(payload)
}

func attachDataFieldOptions(c *server.Context, record map[string]any) {
	fields := dataTemplateRows(record["fields"])
	if fields == nil {
		return
	}
	ctx := dataTemplateContext(c)
	for _, field := range fields {
		fieldID := util.ToUint64(field["id"])
		if fieldID == 0 {
			if options := dataTemplateRows(field["options"]); options != nil {
				field["options"] = options
			} else if _, exists := field["options"]; !exists {
				field["options"] = []map[string]any{}
			}
			continue
		}
		field["options"] = frontmodel.NewDataFieldOptionModel().SelectMap(ctx, map[string]any{
			"data_field_id": fieldID,
		})
	}
	record["fields"] = fields
}

func deleteTemplateFields(ctx context.Context, templateID uint64) {
	fields := frontmodel.NewDataFieldModel().SelectMap(ctx, map[string]any{
		"data_template_id": templateID,
	})
	for _, field := range fields {
		fieldID := util.ToUint64(field["id"])
		if fieldID > 0 {
			frontmodel.NewDataFieldOptionModel().Delete(ctx, map[string]any{"data_field_id": fieldID})
		}
	}
	frontmodel.NewDataFieldModel().Delete(ctx, map[string]any{"data_template_id": templateID})
}

func ensureDataTemplateFieldsMutable(c *server.Context, templateID uint64, nextFields []map[string]any) {
	if templateID == 0 {
		return
	}
	ctx := dataTemplateContext(c)
	if !dataTemplateHasRecords(ctx, templateID) {
		return
	}

	currentFields := frontmodel.NewDataFieldModel().SelectMap(ctx, map[string]any{
		"data_template_id": templateID,
	})
	currentByID := map[uint64]map[string]any{}
	fieldIDs := make([]any, 0, len(currentFields))
	for _, field := range currentFields {
		fieldID := util.ToUint64(field["id"])
		if fieldID == 0 {
			continue
		}
		currentByID[fieldID] = field
		fieldIDs = append(fieldIDs, fieldID)
	}

	nextByID := map[uint64]map[string]any{}
	for _, field := range nextFields {
		fieldID := util.ToUint64(field["id"])
		if fieldID > 0 {
			nextByID[fieldID] = field
		}
	}

	for fieldID, current := range currentByID {
		next, exists := nextByID[fieldID]
		if !exists {
			panic(frontaction.NewFieldError("form.fields", "当前模板已有填写数据，不能删除字段。"))
		}
		if util.ToStringTrimmed(current["field_type"]) != util.ToStringTrimmed(next["field_type"]) {
			panic(frontaction.NewFieldError("form.fields", "当前模板已有填写数据，不能修改字段类型。"))
		}
		if util.ToStringTrimmed(current["field_key"]) != util.ToStringTrimmed(next["field_key"]) {
			panic(frontaction.NewFieldError("form.fields", "当前模板已有填写数据，不能修改字段Key。"))
		}
	}

	ensureDataTemplateOptionsMutable(ctx, fieldIDs, nextByID)
}

func ensureDataTemplateOptionsMutable(ctx context.Context, fieldIDs []any, nextFields map[uint64]map[string]any) {
	if len(fieldIDs) == 0 {
		return
	}
	currentOptions := frontmodel.NewDataFieldOptionModel().SelectMap(ctx, map[string]any{
		"data_field_id": fieldIDs,
	})
	nextOptionsByField := map[uint64]map[uint64]map[string]any{}
	for fieldID, field := range nextFields {
		rows := dataTemplateRows(field["options"])
		if len(rows) == 0 {
			continue
		}
		nextOptionsByField[fieldID] = map[uint64]map[string]any{}
		for _, row := range rows {
			optionID := util.ToUint64(row["id"])
			if optionID > 0 {
				nextOptionsByField[fieldID][optionID] = row
			}
		}
	}

	for _, current := range currentOptions {
		fieldID := util.ToUint64(current["data_field_id"])
		optionID := util.ToUint64(current["id"])
		next := nextOptionsByField[fieldID][optionID]
		if next == nil {
			panic(frontaction.NewFieldError("form.fields", "当前模板已有填写数据，不能删除字段选项。"))
		}
		if util.ToStringTrimmed(current["value"]) != util.ToStringTrimmed(next["value"]) {
			panic(frontaction.NewFieldError("form.fields", "当前模板已有填写数据，不能修改选项值。"))
		}
	}
}

func ensureDataFieldMutable(c *server.Context, record map[string]any) {
	fieldID := util.ToUint64(record["id"])
	if fieldID == 0 {
		return
	}
	ctx := dataTemplateContext(c)
	current := frontmodel.NewDataFieldModel().FindMap(ctx, map[string]any{"id": fieldID})
	if len(current) == 0 {
		return
	}
	templateID := util.ToUint64(current["data_template_id"])
	if templateID == 0 || !dataTemplateHasRecords(ctx, templateID) {
		return
	}
	if _, exists := record["field_type"]; exists &&
		util.ToStringTrimmed(record["field_type"]) != util.ToStringTrimmed(current["field_type"]) {
		panic(frontaction.NewFieldError("form.field_type", "当前模板已有填写数据，不能修改字段类型。"))
	}
	if _, exists := record["field_key"]; exists &&
		util.ToStringTrimmed(record["field_key"]) != util.ToStringTrimmed(current["field_key"]) {
		panic(frontaction.NewFieldError("form.field_key", "当前模板已有填写数据，不能修改字段Key。"))
	}
}

func ensureDataFieldKeyUnique(c *server.Context, record map[string]any) {
	fieldKey := util.ToStringTrimmed(record["field_key"])
	if fieldKey == "" {
		return
	}
	ctx := dataTemplateContext(c)
	fieldID := util.ToUint64(record["id"])
	templateID := util.ToUint64(record["data_template_id"])
	if templateID == 0 && fieldID > 0 {
		current := frontmodel.NewDataFieldModel().FindMap(ctx, map[string]any{"id": fieldID})
		templateID = util.ToUint64(current["data_template_id"])
	}
	if templateID == 0 {
		return
	}
	existing := frontmodel.NewDataFieldModel().FindMap(ctx, map[string]any{
		"data_template_id": templateID,
		"field_key":        fieldKey,
	})
	if len(existing) > 0 && util.ToUint64(existing["id"]) != fieldID {
		panic(frontaction.NewFieldError("form.field_key", "同一模板下字段Key不能重复。"))
	}
}

func ensureDataFieldOptionMutable(c *server.Context, record map[string]any) {
	optionID := util.ToUint64(record["id"])
	if optionID == 0 {
		return
	}
	ctx := dataTemplateContext(c)
	current := frontmodel.NewDataFieldOptionModel().FindMap(ctx, map[string]any{"id": optionID})
	if len(current) == 0 {
		return
	}
	fieldID := util.ToUint64(current["data_field_id"])
	if fieldID == 0 {
		return
	}
	field := frontmodel.NewDataFieldModel().FindMap(ctx, map[string]any{"id": fieldID})
	templateID := util.ToUint64(field["data_template_id"])
	if templateID == 0 || !dataTemplateHasRecords(ctx, templateID) {
		return
	}
	if _, exists := record["value"]; exists &&
		util.ToStringTrimmed(record["value"]) != util.ToStringTrimmed(current["value"]) {
		panic(frontaction.NewFieldError("form.value", "当前模板已有填写数据，不能修改选项值。"))
	}
}

func dataTemplateHasRecords(ctx context.Context, templateID uint64) bool {
	if templateID == 0 {
		return false
	}
	return frontmodel.NewDataRecordModel().Count(ctx, map[string]any{
		"data_template_id": templateID,
	}) > 0
}

func normalizeDataTemplateKey(c *server.Context, record map[string]any, partial bool) {
	if partial {
		if _, exists := record["template_key"]; !exists {
			return
		}
	}
	templateKey := util.ToStringTrimmed(record["template_key"])
	if templateKey == "" {
		if !partial {
			panic(frontaction.NewFieldError("form.template_key", "模板Key不能为空。"))
		}
		return
	}
	if !dataTemplateKeyPattern.MatchString(templateKey) {
		panic(frontaction.NewFieldError("form.template_key", "模板Key只能包含字母、数字、下划线、点和短横线。"))
	}
	ctx := dataTemplateContext(c)
	templateID := util.ToUint64(record["id"])
	if templateID > 0 && dataTemplateHasRecords(ctx, templateID) {
		current := frontmodel.NewDataTemplateModel().FindMap(ctx, map[string]any{"id": templateID})
		if util.ToStringTrimmed(current["template_key"]) != "" &&
			util.ToStringTrimmed(current["template_key"]) != templateKey {
			panic(frontaction.NewFieldError("form.template_key", "当前模板已有填写数据，不能修改模板Key。"))
		}
	}
	if existing := frontmodel.NewDataTemplateModel().FindMap(ctx, map[string]any{"template_key": templateKey}); len(existing) > 0 &&
		util.ToUint64(existing["id"]) != templateID {
		panic(frontaction.NewFieldError("form.template_key", "模板Key已存在。"))
	}
	record["template_key"] = templateKey
}

func normalizeDataFieldKey(record map[string]any, partial bool) {
	if partial {
		if _, exists := record["field_key"]; !exists {
			return
		}
	}
	fieldKey := util.ToStringTrimmed(record["field_key"])
	if fieldKey == "" {
		if !partial {
			panic(frontaction.NewFieldError("form.field_key", "字段Key不能为空。"))
		}
		return
	}
	if !dataTemplateKeyPattern.MatchString(fieldKey) {
		panic(frontaction.NewFieldError("form.field_key", "字段Key只能包含字母、数字、下划线、点和短横线。"))
	}
	record["field_key"] = fieldKey
}

func ensureSubmittedFieldKeysUnique(fields []map[string]any) {
	seen := map[string]struct{}{}
	for _, field := range fields {
		fieldKey := util.ToStringTrimmed(field["field_key"])
		if fieldKey == "" {
			continue
		}
		if _, exists := seen[fieldKey]; exists {
			panic(frontaction.NewFieldError("form.fields", "字段Key不能重复。"))
		}
		seen[fieldKey] = struct{}{}
	}
}
