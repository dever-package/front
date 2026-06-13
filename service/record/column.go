package record

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/shemic/dever/util"
)

var columnLookupCache util.ConcurrentMap[string, map[string]string]
var columnOrderCache util.ConcurrentMap[string, []string]

func SanitizeRecord(record map[string]any, columnLookup map[string]string) map[string]any {
	result := make(map[string]any, len(record))
	for key, value := range record {
		if key == "" || value == nil {
			continue
		}
		switch value.(type) {
		case map[string]any, []any:
			continue
		}
		column := ResolveColumnName(columnLookup, key)
		if column == "" {
			continue
		}
		result[column] = value
	}
	return result
}

func ApplyCreatedAt(record map[string]any, columnLookup map[string]string) {
	if createdAtColumn := ResolveColumnName(columnLookup, "created_at"); createdAtColumn != "" {
		if !HasValue(record[createdAtColumn]) {
			record[createdAtColumn] = time.Now()
		}
	}
}

func ResolveColumnLookup(modelName string, modelValue any) map[string]string {
	modelName = strings.TrimSpace(modelName)
	if modelName != "" {
		if cached, ok := columnLookupCache.Load(modelName); ok {
			return cached
		}
	}

	if lookup := extractModelColumns(modelValue); len(lookup) > 0 {
		if modelName != "" {
			columnLookupCache.Store(modelName, lookup)
		}
		return lookup
	}

	lookup := loadModelColumnsFromSchema(modelName)
	if modelName != "" && len(lookup) > 0 {
		columnLookupCache.Store(modelName, lookup)
	}
	return lookup
}

func ResolveOrderedColumns(modelName string, modelValue any) []string {
	modelName = strings.TrimSpace(modelName)
	if modelName != "" {
		if cached, ok := columnOrderCache.Load(modelName); ok && len(cached) > 0 {
			return append([]string(nil), cached...)
		}
	}

	order := extractModelColumnOrder(modelValue)
	if len(order) == 0 {
		order = loadModelColumnOrderFromSchema(modelName)
	}
	if len(order) == 0 {
		return nil
	}

	if modelName != "" {
		columnOrderCache.Store(modelName, append([]string(nil), order...))
	}
	return append([]string(nil), order...)
}

func ResolveColumnName(columnLookup map[string]string, key string) string {
	if len(columnLookup) == 0 {
		return ""
	}
	return columnLookup[normalizeColumnKey(key)]
}

func ReadValue(record map[string]any, key string) (any, bool) {
	target := normalizeColumnKey(key)
	for currentKey, value := range record {
		if normalizeColumnKey(currentKey) == target {
			return value, true
		}
	}
	return nil, false
}

func HasValue(value any) bool {
	switch current := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(current) != ""
	case int:
		return current != 0
	case int8:
		return current != 0
	case int16:
		return current != 0
	case int32:
		return current != 0
	case int64:
		return current != 0
	case uint:
		return current != 0
	case uint8:
		return current != 0
	case uint16:
		return current != 0
	case uint32:
		return current != 0
	case uint64:
		return current != 0
	case float32:
		return current != 0
	case float64:
		return current != 0
	default:
		return true
	}
}

func extractModelColumns(modelValue any) map[string]string {
	current := reflect.ValueOf(modelValue)
	if !current.IsValid() {
		return nil
	}
	if current.Kind() == reflect.Pointer {
		if current.IsNil() {
			return nil
		}
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return nil
	}

	coreField := current.FieldByName("modelCore")
	if !coreField.IsValid() && current.NumField() > 0 {
		coreField = current.Field(0)
	}
	if !coreField.IsValid() {
		return nil
	}
	if coreField.Kind() == reflect.Pointer {
		if coreField.IsNil() {
			return nil
		}
		coreField = coreField.Elem()
	}
	if coreField.Kind() != reflect.Struct {
		return nil
	}

	schemaField := coreField.FieldByName("schema")
	if !schemaField.IsValid() || schemaField.IsNil() {
		return nil
	}
	schemaValue := schemaField.Elem()
	columnsField := schemaValue.FieldByName("Columns")
	if !columnsField.IsValid() || columnsField.Kind() != reflect.Slice {
		return nil
	}

	lookup := make(map[string]string, columnsField.Len())
	for index := 0; index < columnsField.Len(); index++ {
		column := columnsField.Index(index)
		nameField := column.FieldByName("Name")
		if !nameField.IsValid() {
			continue
		}
		name := strings.TrimSpace(nameField.String())
		if name == "" {
			continue
		}
		lookup[normalizeColumnKey(name)] = name
	}
	return lookup
}

func extractModelColumnOrder(modelValue any) []string {
	current := reflect.ValueOf(modelValue)
	if !current.IsValid() {
		return nil
	}
	if current.Kind() == reflect.Pointer {
		if current.IsNil() {
			return nil
		}
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return nil
	}

	coreField := current.FieldByName("modelCore")
	if !coreField.IsValid() && current.NumField() > 0 {
		coreField = current.Field(0)
	}
	if !coreField.IsValid() {
		return nil
	}
	if coreField.Kind() == reflect.Pointer {
		if coreField.IsNil() {
			return nil
		}
		coreField = coreField.Elem()
	}
	if coreField.Kind() != reflect.Struct {
		return nil
	}

	schemaField := coreField.FieldByName("schema")
	if !schemaField.IsValid() || schemaField.IsNil() {
		return nil
	}
	schemaValue := schemaField.Elem()
	columnsField := schemaValue.FieldByName("Columns")
	if !columnsField.IsValid() || columnsField.Kind() != reflect.Slice {
		return nil
	}

	result := make([]string, 0, columnsField.Len())
	for index := 0; index < columnsField.Len(); index++ {
		column := columnsField.Index(index)
		nameField := column.FieldByName("Name")
		if !nameField.IsValid() {
			continue
		}
		name := strings.TrimSpace(nameField.String())
		if name == "" {
			continue
		}
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func loadModelColumnsFromSchema(modelName string) map[string]string {
	resourceName := ResourceName(modelName)
	if resourceName == "" {
		return nil
	}

	entries, err := os.ReadDir(filepath.Join("data", "table"))
	if err != nil {
		return nil
	}

	type schemaColumn struct {
		Name string `json:"name"`
	}
	type tableSchemaFile struct {
		Columns []schemaColumn `json:"columns"`
	}

	bestFileName := ""
	bestRank := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		rank, ok := matchSchemaFileName(name, resourceName)
		if !ok {
			continue
		}
		if bestRank >= 0 && rank >= bestRank {
			continue
		}
		bestFileName = name
		bestRank = rank
	}

	if bestFileName == "" {
		return nil
	}

	content, readErr := os.ReadFile(filepath.Join("data", "table", bestFileName))
	if readErr != nil {
		return nil
	}

	var schema tableSchemaFile
	if jsonErr := json.Unmarshal(content, &schema); jsonErr != nil {
		return nil
	}

	lookup := make(map[string]string, len(schema.Columns))
	for _, column := range schema.Columns {
		if strings.TrimSpace(column.Name) == "" {
			continue
		}
		lookup[normalizeColumnKey(column.Name)] = column.Name
	}

	if len(lookup) == 0 {
		return nil
	}

	return lookup
}

func loadModelColumnOrderFromSchema(modelName string) []string {
	resourceName := ResourceName(modelName)
	if resourceName == "" {
		return nil
	}

	entries, err := os.ReadDir(filepath.Join("data", "table"))
	if err != nil {
		return nil
	}

	type schemaColumn struct {
		Name string `json:"name"`
	}
	type tableSchemaFile struct {
		Columns []schemaColumn `json:"columns"`
	}

	bestFileName := ""
	bestRank := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		rank, ok := matchSchemaFileName(name, resourceName)
		if !ok {
			continue
		}
		if bestRank >= 0 && rank >= bestRank {
			continue
		}
		bestFileName = name
		bestRank = rank
	}

	if bestFileName == "" {
		return nil
	}

	content, readErr := os.ReadFile(filepath.Join("data", "table", bestFileName))
	if readErr != nil {
		return nil
	}

	var schema tableSchemaFile
	if jsonErr := json.Unmarshal(content, &schema); jsonErr != nil {
		return nil
	}

	result := make([]string, 0, len(schema.Columns))
	for _, column := range schema.Columns {
		name := strings.TrimSpace(column.Name)
		if name == "" {
			continue
		}
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func ResourceName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ""
	}
	parts := strings.Split(modelName, ".")
	if len(parts) > 0 {
		modelFactory := strings.TrimSpace(parts[len(parts)-1])
		modelFactory = strings.TrimPrefix(modelFactory, "New")
		modelFactory = strings.TrimSuffix(modelFactory, "Model")
		if modelFactory != "" {
			return util.ToSnake(modelFactory)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	moduleName := strings.TrimSpace(parts[0])
	if moduleName == "" {
		return ""
	}
	return util.ToSnake(moduleName)
}

func normalizeColumnKey(name string) string {
	return strings.TrimSpace(strings.ToLower(util.ToSnake(name)))
}

func matchSchemaFileName(fileName, resourceName string) (int, bool) {
	fileName = strings.TrimSpace(fileName)
	resourceName = strings.TrimSpace(resourceName)
	if fileName == "" || resourceName == "" {
		return 0, false
	}

	if fileName == resourceName+".json" {
		return 0, true
	}

	baseName := strings.TrimSuffix(fileName, ".json")
	suffix := "_" + resourceName
	if !strings.HasSuffix(baseName, suffix) {
		return 0, false
	}

	prefix := strings.TrimSuffix(baseName, suffix)
	if prefix == "" {
		return 1, true
	}

	return strings.Count(prefix, "_") + 1, true
}
