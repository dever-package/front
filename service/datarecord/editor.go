package datarecord

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmodel "github.com/dever-package/front/model"
	frontaction "github.com/dever-package/front/service/action"
)

const (
	defaultRecordTargetID       uint64 = 0
	defaultRecordTargetRecordID uint64 = 0
)

type Editor struct{}

var optionFieldTypes = map[string]struct{}{
	"radio":        {},
	"checkbox":     {},
	"select":       {},
	"multi_select": {},
}

var uploadFieldTypes = map[string]struct{}{
	"image": {},
	"video": {},
	"audio": {},
}

var uploadKindAcceptTypes = map[string]uint64{
	"image": 1,
	"video": 2,
	"audio": 3,
}

var uploadKindAcceptValues = map[string]string{
	"image": "image/*",
	"video": "video/*",
	"audio": "audio/*",
}

func (Editor) ProviderBuildPageMeta(c *server.Context, _ []any) any {
	ctx := dataRecordContext(c)
	cateID := inputUint64(c, "cate_id", "cateId")
	title := "数据填写"
	description := "请设置基础信息。"
	if cateID > 0 {
		category := frontmodel.NewDataTemplateCateModel().FindMap(ctx, map[string]any{"id": cateID})
		if name := util.ToStringTrimmed(category["name"]); name != "" {
			title = name
		}
		if intro := util.ToStringTrimmed(category["description"]); intro != "" {
			description = intro
		}
	}
	return map[string]any{
		"title":       title,
		"description": description,
	}
}

func (Editor) ProviderBuildRecordEditor(c *server.Context, _ []any) any {
	ctx := dataRecordContext(c)
	cateID := inputUint64(c, "cate_id", "cateId")
	if cateID == 0 {
		return emptyEditorPayload()
	}

	category := frontmodel.NewDataTemplateCateModel().FindMap(ctx, map[string]any{"id": cateID})
	if len(category) == 0 {
		return emptyEditorPayloadWithMessage(cateID, "模板分类不存在")
	}

	templates := frontmodel.NewDataTemplateModel().SelectMap(ctx, map[string]any{
		"cate_id": cateID,
		"status":  frontmodel.DataStatusEnabled,
	}, map[string]any{"order": "sort asc,id asc"})
	templates = attachRecordEditorFields(ctx, templates)

	selectedTemplateID := inputUint64(c, "template_id", "templateId", "data_template_id", "dataTemplateId")
	if selectedTemplateID == 0 && len(templates) > 0 {
		selectedTemplateID = util.ToUint64(templates[0]["id"])
	}

	return map[string]any{
		"cate_id":              cateID,
		"category":             category,
		"templates":            templates,
		"records":              loadRecordEditorRecords(ctx, templates),
		"selected_template_id": selectedTemplateID,
		"upload_rules":         loadAutoUploadRules(ctx),
	}
}

func (Editor) ProviderBeforeSaveRecord(c *server.Context, params []any) any {
	payload := dataRecordPayload(params)
	templateID := util.ToUint64(payload["data_template_id"])
	if templateID == 0 {
		panic(frontaction.NewFieldError("form.data_template_id", "数据模板不能为空。"))
	}

	ctx := dataRecordContext(c)
	template := frontmodel.NewDataTemplateModel().FindMap(ctx, map[string]any{"id": templateID})
	if len(template) == 0 || util.ToIntDefault(template["status"], 0) != frontmodel.DataStatusEnabled {
		panic(frontaction.NewFieldError("form.data_template_id", "数据模板不存在或已停用。"))
	}

	fields := enabledTemplateFields(ctx, templateID)
	values := normalizeRecordValues(ctx, fields, dataRecordValues(payload))
	recordJSON, err := json.Marshal(values)
	if err != nil {
		panic(fmt.Errorf("记录数据编码失败: %w", err))
	}

	record := map[string]any{
		"data_template_id": templateID,
		"target_id":        defaultRecordTargetID,
		"target_record_id": defaultRecordTargetRecordID,
		"record_json":      string(recordJSON),
		"summary":          recordSummary(fields, values),
		"status":           frontmodel.DataStatusEnabled,
		"sort":             100,
		"updated_at":       time.Now(),
	}
	if existing := findSingleTemplateRecord(ctx, templateID); len(existing) > 0 {
		record["id"] = util.ToUint64(existing["id"])
	}
	return record
}

func (Editor) ProviderGetInfo(c *server.Context, params []any) any {
	templateKey := dataRecordTemplateKey(c, params)
	if templateKey == "" {
		panic("数据模板Key不能为空")
	}

	ctx := dataRecordContext(c)
	template := frontmodel.NewDataTemplateModel().FindMap(ctx, map[string]any{
		"template_key": templateKey,
		"status":       frontmodel.DataStatusEnabled,
	})
	if len(template) == 0 {
		panic("数据模板不存在或已停用")
	}

	templateID := util.ToUint64(template["id"])
	fields := enabledTemplateFields(ctx, templateID)
	record := findSingleTemplateRecord(ctx, templateID)
	recordPayload := map[string]any{}
	rawValues := map[string]any{}
	if len(record) > 0 {
		recordPayload = util.CloneMap(record)
		rawValues = decodeRecordJSON(util.ToString(record["record_json"]))
	}

	template = util.CloneMap(template)
	template["fields"] = fields
	return map[string]any{
		"template":   template,
		"fields":     fields,
		"record":     recordPayload,
		"values":     recordValuesByFieldKey(fields, rawValues),
		"raw_values": rawValues,
	}
}

func dataRecordContext(c *server.Context) context.Context {
	if c != nil {
		return c.Context()
	}
	return context.Background()
}

func inputUint64(c *server.Context, keys ...string) uint64 {
	if c == nil {
		return 0
	}
	for _, key := range keys {
		if value := util.ToUint64(c.Input(key)); value > 0 {
			return value
		}
	}
	return 0
}

func emptyEditorPayload() map[string]any {
	return map[string]any{
		"cate_id":              0,
		"category":             map[string]any{},
		"templates":            []map[string]any{},
		"records":              map[string]any{},
		"selected_template_id": 0,
		"upload_rules":         map[string]any{},
	}
}

func emptyEditorPayloadWithMessage(cateID uint64, message string) map[string]any {
	payload := emptyEditorPayload()
	payload["cate_id"] = cateID
	payload["message"] = message
	return payload
}

func attachRecordEditorFields(ctx context.Context, templates []map[string]any) []map[string]any {
	if len(templates) == 0 {
		return templates
	}
	templateIDs := make([]any, 0, len(templates))
	for _, template := range templates {
		if templateID := util.ToUint64(template["id"]); templateID > 0 {
			templateIDs = append(templateIDs, templateID)
		}
	}
	if len(templateIDs) == 0 {
		return templates
	}

	fields := frontmodel.NewDataFieldModel().SelectMap(ctx, map[string]any{
		"data_template_id": templateIDs,
		"status":           frontmodel.DataStatusEnabled,
	}, map[string]any{"order": "sort asc,id asc"})
	attachFieldOptions(ctx, fields)

	grouped := map[uint64][]map[string]any{}
	for _, field := range fields {
		templateID := util.ToUint64(field["data_template_id"])
		if templateID == 0 {
			continue
		}
		grouped[templateID] = append(grouped[templateID], field)
	}
	for _, template := range templates {
		templateID := util.ToUint64(template["id"])
		template["fields"] = grouped[templateID]
	}
	return templates
}

func attachFieldOptions(ctx context.Context, fields []map[string]any) {
	fieldIDs := make([]any, 0, len(fields))
	for _, field := range fields {
		if _, ok := optionFieldTypes[util.ToStringTrimmed(field["field_type"])]; !ok {
			field["options"] = []map[string]any{}
			continue
		}
		if fieldID := util.ToUint64(field["id"]); fieldID > 0 {
			fieldIDs = append(fieldIDs, fieldID)
		}
	}
	if len(fieldIDs) == 0 {
		return
	}

	options := frontmodel.NewDataFieldOptionModel().SelectMap(ctx, map[string]any{
		"data_field_id": fieldIDs,
	}, map[string]any{"order": "sort asc,id asc"})
	grouped := map[uint64][]map[string]any{}
	for _, option := range options {
		fieldID := util.ToUint64(option["data_field_id"])
		grouped[fieldID] = append(grouped[fieldID], option)
	}
	for _, field := range fields {
		fieldID := util.ToUint64(field["id"])
		if _, ok := optionFieldTypes[util.ToStringTrimmed(field["field_type"])]; ok {
			field["options"] = grouped[fieldID]
		}
	}
}

func loadRecordEditorRecords(ctx context.Context, templates []map[string]any) map[string]any {
	templateIDs := make([]any, 0, len(templates))
	for _, template := range templates {
		if templateID := util.ToUint64(template["id"]); templateID > 0 {
			templateIDs = append(templateIDs, templateID)
		}
	}
	if len(templateIDs) == 0 {
		return map[string]any{}
	}

	rows := frontmodel.NewDataRecordModel().SelectMap(ctx, map[string]any{
		"data_template_id": templateIDs,
		"target_id":        defaultRecordTargetID,
		"target_record_id": defaultRecordTargetRecordID,
	}, map[string]any{"order": "id desc"})
	records := map[string]any{}
	for _, row := range rows {
		templateID := util.ToUint64(row["data_template_id"])
		key := strconv.FormatUint(templateID, 10)
		if _, exists := records[key]; exists {
			continue
		}
		record := util.CloneMap(row)
		record["values"] = decodeRecordJSON(util.ToString(row["record_json"]))
		records[key] = record
	}
	return records
}

func loadAutoUploadRules(ctx context.Context) map[string]any {
	rows := frontmodel.NewUploadRuleModel().SelectMap(ctx, map[string]any{
		"status": 1,
	}, map[string]any{"order": "id asc"})
	result := map[string]any{}
	for _, row := range rows {
		acceptTypeID := util.ToUint64(row["accept_type_id"])
		for kind, targetAcceptTypeID := range uploadKindAcceptTypes {
			if _, exists := result[kind]; exists || acceptTypeID != targetAcceptTypeID {
				continue
			}
			rule := util.CloneMap(row)
			rule["accept"] = uploadKindAcceptValues[kind]
			rule["kind"] = kind
			result[kind] = rule
		}
	}
	return result
}

func dataRecordPayload(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	record, _ := params[0].(map[string]any)
	if record == nil {
		return map[string]any{}
	}
	return util.CloneMap(record)
}

func dataRecordTemplateKey(c *server.Context, params []any) string {
	payload := dataRecordPayload(params)
	for _, key := range []string{"template_key", "templateKey", "key", "data_template_key", "dataTemplateKey"} {
		if value := util.ToStringTrimmed(payload[key]); value != "" {
			return value
		}
	}
	if len(params) > 0 {
		if value, ok := params[0].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := params[0].([]byte); ok && strings.TrimSpace(string(value)) != "" {
			return strings.TrimSpace(string(value))
		}
		if value, ok := params[0].(fmt.Stringer); ok && strings.TrimSpace(value.String()) != "" {
			return strings.TrimSpace(value.String())
		}
	}
	if c == nil {
		return ""
	}
	for _, key := range []string{"template_key", "templateKey", "key", "data_template_key", "dataTemplateKey"} {
		if value := util.ToStringTrimmed(c.Input(key)); value != "" {
			return value
		}
	}
	return ""
}

func dataRecordValues(payload map[string]any) map[string]any {
	for _, key := range []string{"values", "record", "record_json", "recordJSON"} {
		if values := mapValue(payload[key]); values != nil {
			return values
		}
	}
	return map[string]any{}
}

func mapValue(value any) map[string]any {
	switch current := value.(type) {
	case map[string]any:
		return util.CloneMap(current)
	case string:
		return decodeRecordJSON(current)
	default:
		return nil
	}
}

func decodeRecordJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return map[string]any{}
	}
	if values == nil {
		return map[string]any{}
	}
	return values
}

func enabledTemplateFields(ctx context.Context, templateID uint64) []map[string]any {
	fields := frontmodel.NewDataFieldModel().SelectMap(ctx, map[string]any{
		"data_template_id": templateID,
		"status":           frontmodel.DataStatusEnabled,
	}, map[string]any{"order": "sort asc,id asc"})
	attachFieldOptions(ctx, fields)
	return fields
}

func normalizeRecordValues(ctx context.Context, fields []map[string]any, rawValues map[string]any) map[string]any {
	fieldLookup := map[string]struct{}{}
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		fieldID := util.ToUint64(field["id"])
		if fieldID == 0 {
			continue
		}
		key := strconv.FormatUint(fieldID, 10)
		fieldLookup[key] = struct{}{}
		fieldKey := util.ToStringTrimmed(field["field_key"])
		if fieldKey != "" {
			fieldLookup[fieldKey] = struct{}{}
		}
		rawValue, exists := rawValues[key]
		if !exists && fieldKey != "" {
			rawValue, exists = rawValues[fieldKey]
		}
		values[key] = normalizeRecordFieldValue(ctx, field, rawValue, exists)
	}
	for key := range rawValues {
		if _, exists := fieldLookup[key]; exists {
			continue
		}
		panic(frontaction.NewFieldError("form.values."+key, "字段不存在或已停用。"))
	}
	return values
}

func recordValuesByFieldKey(fields []map[string]any, rawValues map[string]any) map[string]any {
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		fieldID := util.ToUint64(field["id"])
		fieldKey := util.ToStringTrimmed(field["field_key"])
		if fieldID == 0 || fieldKey == "" {
			continue
		}
		value, exists := rawValues[strconv.FormatUint(fieldID, 10)]
		if exists {
			values[fieldKey] = value
		}
	}
	return values
}

func normalizeRecordFieldValue(ctx context.Context, field map[string]any, rawValue any, exists bool) any {
	fieldType := util.ToStringTrimmed(field["field_type"])
	if !exists && util.ToStringTrimmed(field["default_value"]) != "" {
		rawValue = field["default_value"]
		exists = true
	}

	var normalized any
	switch fieldType {
	case "text", "textarea":
		normalized = util.ToStringTrimmed(rawValue)
	case "editor":
		normalized = util.ToString(rawValue)
	case "date":
		normalized = normalizeDateValue(field, rawValue)
	case "datetime":
		normalized = normalizeDateTimeValue(field, rawValue)
	case "boolean":
		if !exists && fieldRequired(field) {
			panicRecordFieldError(field, "请选择"+fieldName(field)+"。")
		}
		normalized = util.ToBool(rawValue)
	case "radio", "select":
		normalized = normalizeOptionValue(field, rawValue)
	case "checkbox", "multi_select":
		normalized = normalizeMultiOptionValue(field, rawValue)
	case "image", "video", "audio":
		normalized = normalizeUploadValue(ctx, field, rawValue)
	default:
		panicRecordFieldError(field, "字段类型不正确。")
	}

	if fieldRequired(field) && recordValueIsEmpty(normalized) {
		panicRecordFieldError(field, fieldName(field)+"不能为空。")
	}
	return normalized
}

func normalizeDateValue(field map[string]any, rawValue any) string {
	text := util.ToStringTrimmed(rawValue)
	if text == "" {
		return ""
	}
	if parsed, err := time.Parse("2006-01-02", text); err == nil {
		return parsed.Format("2006-01-02")
	}
	panicRecordFieldError(field, fieldName(field)+"格式不正确。")
	return ""
}

func normalizeDateTimeValue(field map[string]any, rawValue any) string {
	raw := util.ToStringTrimmed(rawValue)
	if raw == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.Format("2006-01-02 15:04:05")
	}
	text := strings.ReplaceAll(raw, "T", " ")
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.Format("2006-01-02 15:04:05")
		}
	}
	panicRecordFieldError(field, fieldName(field)+"格式不正确。")
	return ""
}

func normalizeOptionValue(field map[string]any, rawValue any) string {
	value := util.ToStringTrimmed(rawValue)
	if value == "" {
		return ""
	}
	if _, exists := fieldOptionValueSet(field)[value]; !exists {
		panicRecordFieldError(field, fieldName(field)+"选项不正确。")
	}
	return value
}

func normalizeMultiOptionValue(field map[string]any, rawValue any) []string {
	values := normalizeStringList(rawValue)
	if len(values) == 0 {
		return []string{}
	}
	allowed := fieldOptionValueSet(field)
	for _, value := range values {
		if _, exists := allowed[value]; !exists {
			panicRecordFieldError(field, fieldName(field)+"选项不正确。")
		}
	}
	return values
}

func normalizeStringList(rawValue any) []string {
	switch current := rawValue.(type) {
	case []string:
		return uniqueStrings(trimStringSlice(current))
	case []any:
		values := make([]string, 0, len(current))
		for _, value := range current {
			text := util.ToStringTrimmed(value)
			if text != "" {
				values = append(values, text)
			}
		}
		return uniqueStrings(values)
	case string:
		values := []string{}
		for _, token := range strings.FieldsFunc(current, func(r rune) bool {
			return r == ',' || r == ';'
		}) {
			if text := strings.TrimSpace(token); text != "" {
				values = append(values, text)
			}
		}
		return uniqueStrings(values)
	default:
		return []string{}
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func trimStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func normalizeUploadValue(ctx context.Context, field map[string]any, rawValue any) []map[string]any {
	items := uploadItems(rawValue)
	if len(items) == 0 {
		return []map[string]any{}
	}
	maxCount := util.ToIntDefault(field["max_count"], 1)
	if maxCount <= 0 {
		maxCount = 1
	}
	if len(items) > maxCount {
		panicRecordFieldError(field, fieldName(field)+"最多上传"+strconv.Itoa(maxCount)+"个文件。")
	}

	values := make([]map[string]any, 0, len(items))
	for _, item := range items {
		file := normalizeUploadItem(ctx, item, field)
		if len(file) > 0 {
			values = append(values, file)
		}
	}
	return values
}

func uploadItems(rawValue any) []map[string]any {
	switch current := rawValue.(type) {
	case []map[string]any:
		return util.CloneMapSlice(current)
	case []any:
		items := make([]map[string]any, 0, len(current))
		for _, item := range current {
			if file, ok := item.(map[string]any); ok && file != nil {
				items = append(items, util.CloneMap(file))
			}
		}
		return items
	case map[string]any:
		return []map[string]any{util.CloneMap(current)}
	case string:
		raw := strings.TrimSpace(current)
		if raw == "" {
			return nil
		}
		var items []map[string]any
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			return util.CloneMapSlice(items)
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(raw), &item); err == nil && item != nil {
			return []map[string]any{util.CloneMap(item)}
		}
		return nil
	default:
		return nil
	}
}

func normalizeUploadItem(ctx context.Context, item map[string]any, field map[string]any) map[string]any {
	fieldType := util.ToStringTrimmed(field["field_type"])
	fileID := util.ToUint64(item["id"])
	if fileID > 0 {
		if row := frontmodel.NewUploadFileModel().FindMap(ctx, map[string]any{"id": fileID}); len(row) > 0 {
			mergeUploadFileRow(item, row)
		}
	}

	kind := util.ToStringTrimmed(item["kind"])
	mime := util.ToStringTrimmed(item["mime"])
	if kind == "" {
		kind = uploadKindFromMime(mime)
	}
	if kind != "" && kind != fieldType {
		panicRecordFieldError(field, "上传文件类型不正确。")
	}
	if kind == "" {
		kind = fieldType
	}

	result := map[string]any{
		"kind": kind,
	}
	for _, key := range []string{"id", "name", "mime", "size", "path", "url", "download", "thumbnail", "open_url"} {
		if value, exists := item[key]; exists {
			result[key] = value
		}
	}
	return result
}

func mergeUploadFileRow(target map[string]any, row map[string]any) {
	for _, key := range []string{"id", "name", "mime", "size", "path", "kind"} {
		if value, exists := row[key]; exists && !recordValueIsEmpty(value) {
			target[key] = value
		}
	}
}

func uploadKindFromMime(mime string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "image/"):
		return "image"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "video/"):
		return "video"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "audio/"):
		return "audio"
	default:
		return ""
	}
}

func fieldOptionValueSet(field map[string]any) map[string]struct{} {
	options := map[string]struct{}{}
	for _, option := range mapRows(field["options"]) {
		value := util.ToStringTrimmed(option["value"])
		if value != "" {
			options[value] = struct{}{}
		}
	}
	return options
}

func mapRows(value any) []map[string]any {
	switch rows := value.(type) {
	case []map[string]any:
		return rows
	case []any:
		result := make([]map[string]any, 0, len(rows))
		for _, item := range rows {
			if row, ok := item.(map[string]any); ok && row != nil {
				result = append(result, row)
			}
		}
		return result
	default:
		return nil
	}
}

func fieldRequired(field map[string]any) bool {
	return util.ToBool(field["required"])
}

func fieldName(field map[string]any) string {
	name := util.ToStringTrimmed(field["name"])
	if name != "" {
		return name
	}
	return "字段"
}

func panicRecordFieldError(field map[string]any, message string) {
	fieldID := util.ToUint64(field["id"])
	fieldPath := "form.values"
	if fieldID > 0 {
		fieldPath += "." + strconv.FormatUint(fieldID, 10)
	}
	panic(frontaction.NewFieldError(fieldPath, message))
}

func recordValueIsEmpty(value any) bool {
	switch current := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(current) == ""
	case []string:
		return len(current) == 0
	case []map[string]any:
		return len(current) == 0
	case []any:
		return len(current) == 0
	default:
		return false
	}
}

func findSingleTemplateRecord(ctx context.Context, templateID uint64) map[string]any {
	rows := frontmodel.NewDataRecordModel().SelectMap(ctx, map[string]any{
		"data_template_id": templateID,
		"target_id":        defaultRecordTargetID,
		"target_record_id": defaultRecordTargetRecordID,
	}, map[string]any{"order": "id desc"})
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func recordSummary(fields []map[string]any, values map[string]any) string {
	for _, field := range fields {
		key := strconv.FormatUint(util.ToUint64(field["id"]), 10)
		text := summaryValueText(field, values[key])
		if strings.TrimSpace(text) == "" {
			continue
		}
		summary := fieldName(field) + ": " + text
		if len([]rune(summary)) > 255 {
			return string([]rune(summary)[:255])
		}
		return summary
	}
	return ""
}

func summaryValueText(field map[string]any, value any) string {
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	case bool:
		if current {
			return "是"
		}
		return "否"
	case []string:
		return strings.Join(optionLabels(field, current), "、")
	case []map[string]any:
		names := make([]string, 0, len(current))
		for _, item := range current {
			if name := util.ToStringTrimmed(item["name"]); name != "" {
				names = append(names, name)
			}
		}
		return strings.Join(names, "、")
	default:
		return ""
	}
}

func optionLabels(field map[string]any, values []string) []string {
	labelByValue := map[string]string{}
	for _, option := range mapRows(field["options"]) {
		value := util.ToStringTrimmed(option["value"])
		name := util.ToStringTrimmed(option["name"])
		if value != "" {
			labelByValue[value] = util.FirstNonEmpty(name, value)
		}
	}
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, util.FirstNonEmpty(labelByValue[value], value))
	}
	return labels
}
