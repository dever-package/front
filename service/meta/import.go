package meta

import (
	"sort"
	"strings"

	frontrecord "my/package/front/service/record"
)

type ResolvedImportMeta struct {
	MatchFields []string
	MatchMode   string
	Fields      []ImportField
}

type ImportField struct {
	Field         string
	Label         string
	Kind          string
	Aliases       []string
	Use           string
	Multiple      bool
	MissingPolicy string
	SaveMode      string
	UploadKind    string
	UploadRuleID  int
	SourceMode    string
	BaseDir       string
	Delimiters    []string
	ParentField   string
	RootValue     any
	Tip           string
}

var defaultImportExcludedFields = map[string]struct{}{
	"id":         {},
	"created_at": {},
	"updated_at": {},
	"deleted_at": {},
}

func ResolveModelImportMeta(modelName string) (ResolvedImportMeta, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ResolvedImportMeta{}, false
	}

	config := ResolveModelConfig(modelName)

	relations := ResolveModelRelations(modelName)
	relationByField := make(map[string]Relation, len(relations))
	for _, relation := range relations {
		relationByField[strings.TrimSpace(relation.Field)] = relation
	}

	optionMap := config.Options
	fieldNames := collectImportFieldNames(modelName, relationByField, optionMap)
	if len(fieldNames) == 0 {
		return ResolvedImportMeta{}, false
	}

	result := ResolvedImportMeta{
		MatchFields: ResolveImportMatchCandidates(modelName, nil),
		MatchMode:   NormalizeImportMatchMode(""),
		Fields:      make([]ImportField, 0, len(fieldNames)),
	}
	for _, fieldName := range fieldNames {
		field := ImportField{
			Field: fieldName,
			Label: defaultImportFieldLabel(modelName, fieldName, relationByField),
		}
		result.Fields = append(result.Fields, normalizeImportField(field, relationByField, optionMap))
	}
	return result, true
}

func ApplyImportFieldOverrides(base []ImportField, overrides []ImportField) []ImportField {
	if len(base) == 0 {
		return nil
	}
	if len(overrides) == 0 {
		return append([]ImportField(nil), base...)
	}

	overrideByField := buildImportFieldOverrideMap(overrides)
	result := make([]ImportField, 0, len(base))
	for _, field := range base {
		merged := field
		if override, ok := overrideByField[field.Field]; ok {
			merged = mergeImportField(field, override)
		}
		result = append(result, normalizeImportField(merged, nil, nil))
	}

	for fieldName, override := range overrideByField {
		exists := false
		for _, field := range base {
			if field.Field == fieldName {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		result = append(result, normalizeImportField(override, nil, nil))
	}
	return result
}

func buildImportFieldOverrideMap(fields []ImportField) map[string]ImportField {
	if len(fields) == 0 {
		return nil
	}

	result := make(map[string]ImportField, len(fields))
	for _, field := range fields {
		fieldName := strings.TrimSpace(field.Field)
		if fieldName == "" {
			continue
		}
		field.Field = fieldName
		result[fieldName] = field
	}
	return result
}

func collectImportFieldNames(
	modelName string,
	relationByField map[string]Relation,
	optionMap map[string]any,
) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	appendField := func(field string) {
		field = strings.TrimSpace(field)
		if field == "" || shouldSkipImportField(field) {
			return
		}
		if _, exists := seen[field]; exists {
			return
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, frontrecord.Resolve(modelName))
	columnFields := make([]string, 0, len(columnLookup))
	for _, columnName := range columnLookup {
		columnFields = append(columnFields, strings.TrimSpace(columnName))
	}
	sort.Strings(columnFields)
	for _, columnName := range columnFields {
		appendField(columnName)
	}

	relationFields := make([]string, 0, len(relationByField))
	for field := range relationByField {
		relationFields = append(relationFields, field)
	}
	sort.Strings(relationFields)
	for _, field := range relationFields {
		appendField(field)
	}

	optionFields := make([]string, 0, len(optionMap))
	for field := range optionMap {
		optionFields = append(optionFields, strings.TrimSpace(field))
	}
	sort.Strings(optionFields)
	for _, field := range optionFields {
		appendField(field)
	}

	return result
}

func shouldSkipImportField(field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return true
	}
	_, exists := defaultImportExcludedFields[field]
	return exists
}

func defaultImportFieldLabel(modelName, field string, relationByField map[string]Relation) string {
	field = strings.TrimSpace(field)
	if label := ResolveFieldLabel(modelName, field); label != "" {
		return label
	}
	if relation, ok := relationByField[field]; ok {
		if label := strings.TrimSpace(relation.Name); label != "" {
			return label
		}
	}

	label := field
	switch {
	case strings.HasSuffix(label, "_ids"):
		label = strings.TrimSuffix(label, "_ids")
	case strings.HasSuffix(label, "_id"):
		label = strings.TrimSuffix(label, "_id")
	}
	label = strings.ReplaceAll(label, "_", " ")
	label = strings.TrimSpace(label)
	if label == "" {
		return field
	}
	return label
}

func mergeImportField(base ImportField, override ImportField) ImportField {
	field := base
	if label := strings.TrimSpace(override.Label); label != "" {
		field.Label = label
	}
	if kind := strings.TrimSpace(override.Kind); kind != "" {
		field.Kind = kind
	}
	if use := strings.TrimSpace(override.Use); use != "" {
		field.Use = use
	}
	if override.Multiple {
		field.Multiple = true
	}
	if policy := strings.TrimSpace(override.MissingPolicy); policy != "" {
		field.MissingPolicy = policy
	}
	if saveMode := strings.TrimSpace(override.SaveMode); saveMode != "" {
		field.SaveMode = saveMode
	}
	if uploadKind := strings.TrimSpace(override.UploadKind); uploadKind != "" {
		field.UploadKind = uploadKind
	}
	if override.UploadRuleID > 0 {
		field.UploadRuleID = override.UploadRuleID
	}
	if sourceMode := strings.TrimSpace(override.SourceMode); sourceMode != "" {
		field.SourceMode = sourceMode
	}
	if baseDir := strings.TrimSpace(override.BaseDir); baseDir != "" {
		field.BaseDir = baseDir
	}
	if aliases := append([]string(nil), override.Aliases...); len(aliases) > 0 {
		field.Aliases = aliases
	}
	if delimiters := append([]string(nil), override.Delimiters...); len(delimiters) > 0 {
		field.Delimiters = delimiters
	}
	if parentField := strings.TrimSpace(override.ParentField); parentField != "" {
		field.ParentField = parentField
	}
	if override.RootValue != nil {
		field.RootValue = override.RootValue
	}
	if tip := strings.TrimSpace(override.Tip); tip != "" {
		field.Tip = tip
	}
	return field
}

func ResolveImportMatchCandidates(modelName string, configured []string) []string {
	fields := NormalizeImportMatchFields(configured)
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		seen[field] = struct{}{}
	}

	adapter := frontrecord.ResolveAdapter(modelName)
	if adapter == nil {
		return fields
	}

	indexes := adapter.UniqueIndexes()
	if len(indexes) == 0 {
		return fields
	}

	result := append([]string(nil), fields...)
	for _, group := range indexes {
		for _, field := range group {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			if _, exists := seen[field]; exists {
				continue
			}
			seen[field] = struct{}{}
			result = append(result, field)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func NormalizeImportMatchFields(fields []string) []string {
	result := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}

func NormalizeImportMatchMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "all":
		return "all"
	default:
		return "any"
	}
}

func normalizeImportField(
	field ImportField,
	relationByField map[string]Relation,
	optionMap map[string]any,
) ImportField {
	field.Field = strings.TrimSpace(field.Field)
	field.Label = strings.TrimSpace(field.Label)
	field.Kind = strings.ToLower(strings.TrimSpace(field.Kind))
	field.Use = strings.TrimSpace(field.Use)
	field.ParentField = strings.TrimSpace(field.ParentField)
	field.Tip = strings.TrimSpace(field.Tip)
	field.BaseDir = strings.TrimSpace(field.BaseDir)

	if field.Label == "" {
		field.Label = field.Field
	}

	if relation, ok := relationByField[field.Field]; ok {
		if field.Kind == "" {
			field.Kind = inferImportFieldKind(relation)
		}
		if !field.Multiple {
			field.Multiple = strings.TrimSpace(relation.Mode) == "multiple" || field.Kind == "children" || field.Kind == "cascade"
		}
	} else if field.Kind == "" {
		if _, ok := optionMap[field.Field]; ok {
			field.Kind = "option"
		} else {
			field.Kind = "scalar"
		}
	}

	field.UploadKind = normalizeImportUploadKind(field)
	field.UploadRuleID = normalizeImportUploadRuleID(field.UploadRuleID, field.UploadKind)
	field.SaveMode = normalizeImportSaveMode(field.Kind, field.SaveMode)
	field.SourceMode = normalizeImportSourceMode(field.Kind, field.SourceMode)
	field.MissingPolicy = NormalizeImportMissingPolicy(field.Kind, field.MissingPolicy)
	field.Aliases = normalizeImportAliases(field.Field, field.Label, field.Aliases)
	field.Delimiters = normalizeImportDelimiters(field.Kind, field.Multiple, field.Delimiters)
	return field
}

func inferImportFieldKind(relation Relation) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(relation.Option), "front.NewUploadFileModel"):
		return "upload"
	case strings.TrimSpace(relation.Through) != "" && strings.TrimSpace(relation.Option) == "":
		return "children"
	case strings.TrimSpace(relation.Option) != "":
		if relationHasCascadeOption(relation.Option) {
			return "cascade"
		}
		return "relation"
	default:
		return "scalar"
	}
}

func relationHasCascadeOption(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, frontrecord.Resolve(modelName))
	return frontrecord.ResolveColumnName(columnLookup, "parent_id") != ""
}

func NormalizeImportMissingPolicy(kind, policy string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	policy = strings.ToLower(strings.TrimSpace(policy))

	switch policy {
	case "create":
		if kind == "relation" || kind == "cascade" || kind == "service" {
			return "create"
		}
	case "skip":
		if kind == "upload" {
			return "skip"
		}
	}

	if kind == "relation" || kind == "cascade" || kind == "service" {
		return "error"
	}
	if kind == "upload" {
		return "error"
	}
	return ""
}

func normalizeImportUploadKind(field ImportField) string {
	if strings.TrimSpace(field.Kind) != "upload" {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(field.UploadKind)) {
	case "image", "video", "audio", "file":
		return strings.ToLower(strings.TrimSpace(field.UploadKind))
	}

	fieldName := strings.ToLower(strings.TrimSpace(field.Field))
	switch {
	case strings.Contains(fieldName, "avatar"), strings.Contains(fieldName, "image"), strings.Contains(fieldName, "picture"):
		return "image"
	case strings.Contains(fieldName, "video"):
		return "video"
	case strings.Contains(fieldName, "audio"):
		return "audio"
	default:
		return "file"
	}
}

func normalizeImportUploadRuleID(ruleID int, uploadKind string) int {
	if ruleID > 0 {
		return ruleID
	}

	switch strings.ToLower(strings.TrimSpace(uploadKind)) {
	case "image":
		return 1
	case "video":
		return 2
	case "audio":
		return 3
	default:
		return 6
	}
}

func normalizeImportSourceMode(kind, mode string) string {
	if strings.ToLower(strings.TrimSpace(kind)) != "upload" {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "embed", "path":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "auto"
	}
}

func normalizeImportSaveMode(kind, mode string) string {
	if strings.ToLower(strings.TrimSpace(kind)) != "upload" {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "url":
		return "url"
	default:
		return "id"
	}
}

func normalizeImportAliases(field, label string, aliases []string) []string {
	result := make([]string, 0, len(aliases)+2)
	seen := map[string]struct{}{}
	appendAlias := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	appendAlias(field)
	appendAlias(label)
	for _, alias := range aliases {
		appendAlias(alias)
	}
	return result
}

func normalizeImportDelimiters(kind string, multiple bool, delimiters []string) []string {
	if !multiple {
		return nil
	}

	result := make([]string, 0, len(delimiters)+6)
	seen := map[string]struct{}{}
	appendDelimiter := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	for _, delimiter := range delimiters {
		appendDelimiter(delimiter)
	}
	defaultDelimiters := []string{"/", "、", ",", "，", ";", "；", "\n"}
	if strings.ToLower(strings.TrimSpace(kind)) == "upload" {
		defaultDelimiters = []string{"、", ",", "，", ";", "；", "\n", "|"}
	}
	for _, delimiter := range defaultDelimiters {
		appendDelimiter(delimiter)
	}
	return result
}
