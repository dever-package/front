package optionseed

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/shemic/dever/util"

	frontrecord "github.com/dever-package/front/service/record"
)

func Rows(modelName, parentField string, parentValue any) []map[string]any {
	return RowsByField(modelName, parentField, []any{parentValue})
}

func RowsByField(modelName, field string, values []any) []map[string]any {
	resource := frontrecord.ResourceName(modelName)
	if resource == "" {
		return nil
	}

	for _, path := range schemaCandidates(resource) {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var payload struct {
			Seeds []map[string]any `json:"seeds"`
		}
		if err := json.Unmarshal(content, &payload); err != nil || len(payload.Seeds) == 0 {
			continue
		}

		rows := make([]map[string]any, 0)
		for _, row := range payload.Seeds {
			if len(values) > 0 && !matchAny(row[field], values) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			return rows
		}
	}

	return nil
}

func schemaCandidates(resource string) []string {
	candidates := []string{
		filepath.Join("data", "table", "shemic_"+resource+".json"),
		filepath.Join("data", "table", resource+".json"),
		filepath.Join("data", resource+".json"),
	}
	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if cleaned == "." || cleaned == "" {
			continue
		}
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func matchAny(left any, values []any) bool {
	leftValue := util.ToKeyString(left)
	for _, value := range values {
		if leftValue == util.ToKeyString(value) {
			return true
		}
	}
	return false
}
