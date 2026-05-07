package meta

import (
	"context"
	"strings"

	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"

	frontrecord "my/package/front/service/record"
)

func ResolveModelConfig(modelName string) orm.ModelConfig {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return orm.ModelConfig{}
	}
	return frontrecord.ResolveConfig(modelName, frontrecord.LoadSafe(modelName))
}

func ResolveModelName(modelName string) string {
	return strings.TrimSpace(ResolveModelConfig(modelName).Name)
}

func ResolveModelOptionKeys(modelName string) map[string]struct{} {
	config := ResolveModelConfig(modelName)
	if len(config.Options) == 0 && len(config.Relations) == 0 {
		return nil
	}

	result := map[string]struct{}{}
	for key := range config.Options {
		addOptionKey(result, key)
	}
	for _, relation := range config.Relations {
		for _, key := range relationOptionKeys(normalizeRelation(modelName, relation)) {
			addOptionKey(result, key)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func HasModelOptionKey(modelName, key string) bool {
	keys := ResolveModelOptionKeys(modelName)
	if len(keys) == 0 {
		return false
	}
	_, ok := keys[strings.TrimSpace(key)]
	return ok
}

func addOptionKey(keys map[string]struct{}, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	keys[key] = struct{}{}
}

func ResolveModelOptions(ctx context.Context, modelName string) map[string]any {
	config := ResolveModelConfig(modelName)
	result := config.Options
	for _, relation := range config.Relations {
		result = MergeOptionMap(result, BuildRelationOptions(ctx, normalizeRelation(modelName, relation)))
	}
	return result
}

func ResolveModelRelations(modelName string) []Relation {
	config := ResolveModelConfig(modelName)
	if len(config.Relations) == 0 {
		return nil
	}

	relations := make([]Relation, 0, len(config.Relations))
	for _, relation := range config.Relations {
		relations = append(relations, normalizeRelation(modelName, relation))
	}
	return relations
}

func ResolveModelFieldsByType(modelName, fieldType string) []string {
	return modelFieldsByTypes(ResolveModelConfig(modelName), fieldType)
}

func AttachRelations(ctx context.Context, modelName string, rows []map[string]any) []map[string]any {
	relations := ResolveModelRelations(modelName)
	if len(relations) == 0 {
		return rows
	}

	result := rows
	for _, relation := range relations {
		result = AttachRelation(ctx, result, relation)
	}
	return result
}

func HideFields(modelName string, rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}

	hiddenFields := hiddenModelFields(modelName)
	if len(hiddenFields) == 0 {
		return rows
	}

	hiddenFieldLookup := make(map[string]struct{}, len(hiddenFields))
	for _, field := range hiddenFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		hiddenFieldLookup[field] = struct{}{}
		hiddenFieldLookup[util.ToSnake(field)] = struct{}{}
	}

	if len(hiddenFieldLookup) == 0 {
		return rows
	}

	for _, row := range rows {
		for field := range hiddenFieldLookup {
			delete(row, field)
		}
	}

	return rows
}

func SaveModelRelations(ctx context.Context, modelName string, ownerID any, record map[string]any) error {
	relations := ResolveModelRelations(modelName)
	if len(relations) == 0 {
		return nil
	}

	for _, relation := range relations {
		if err := SaveRelation(ctx, ownerID, record, relation); err != nil {
			return err
		}
	}
	return nil
}

func DeleteModelRelations(ctx context.Context, modelName string, payload any) error {
	relations := ResolveModelRelations(modelName)
	if len(relations) == 0 {
		return nil
	}

	for _, relation := range relations {
		if err := DeleteRelation(ctx, payload, relation); err != nil {
			return err
		}
	}
	return nil
}

func hiddenModelFields(modelName string) []string {
	return modelFieldsByTypes(ResolveModelConfig(modelName), orm.FieldTypeHidden, orm.FieldTypePassword)
}

func modelFieldsByTypes(config orm.ModelConfig, fieldTypes ...string) []string {
	if len(config.Fields) == 0 || len(fieldTypes) == 0 {
		return nil
	}

	typeLookup := make(map[string]struct{}, len(fieldTypes))
	for _, fieldType := range fieldTypes {
		fieldType = strings.ToLower(strings.TrimSpace(fieldType))
		if fieldType == "" {
			continue
		}
		typeLookup[fieldType] = struct{}{}
	}
	if len(typeLookup) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0)
	for field, fieldConfig := range config.Fields {
		if _, ok := typeLookup[strings.ToLower(strings.TrimSpace(fieldConfig.Type))]; !ok {
			continue
		}
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

func normalizeRelation(modelName string, relation Relation) Relation {
	if strings.TrimSpace(relation.Kind) == "" {
		if strings.TrimSpace(relation.Option) != "" {
			relation.Kind = "option"
		} else {
			relation.Kind = "children"
		}
	}
	if strings.TrimSpace(relation.Field) == "" {
		relation.Field = strings.TrimSpace(relation.TargetField)
	}
	if strings.TrimSpace(relation.Mode) == "" {
		if relation.Kind == "children" {
			relation.Mode = "multiple"
		} else if strings.HasSuffix(strings.TrimSpace(relation.Field), "_ids") {
			relation.Mode = "multiple"
		} else {
			relation.Mode = "single"
		}
	}
	if strings.TrimSpace(relation.RowKey) == "" {
		relation.RowKey = "id"
	}
	if strings.TrimSpace(relation.OwnerField) == "" {
		resource := frontrecord.ResourceName(modelName)
		if resource != "" {
			relation.OwnerField = resource + "_id"
		}
	}
	if strings.TrimSpace(relation.TargetField) == "" {
		field := strings.TrimSpace(relation.Field)
		if relation.Kind == "option" {
			switch {
			case strings.HasSuffix(field, "_ids"):
				relation.TargetField = strings.TrimSuffix(field, "_ids") + "_id"
			default:
				relation.TargetField = field
			}
		}
	}
	if strings.TrimSpace(relation.Name) == "" {
		field := strings.TrimSpace(relation.Field)
		if relation.Kind == "children" {
			relation.Name = field
		} else {
			switch {
			case strings.HasSuffix(field, "_ids"):
				relation.Name = strings.TrimSuffix(field, "_ids") + "s"
			case strings.HasSuffix(field, "_id"):
				relation.Name = strings.TrimSuffix(field, "_id")
			default:
				relation.Name = field
			}
		}
	}
	if strings.TrimSpace(relation.OptionValueField) == "" {
		relation.OptionValueField = "id"
	}
	if strings.TrimSpace(relation.OptionLabelField) == "" {
		relation.OptionLabelField = "name"
	}
	if relation.Kind == "option" && relation.OptionKeys == nil {
		if baseKey := relationOptionBaseKey(relation); baseKey != "" {
			relation.OptionKeys = []string{baseKey}
		}
	}
	if relation.EmptyValue == nil {
		if relation.Mode == "multiple" || relation.Kind == "children" {
			relation.EmptyValue = []any{}
		} else {
			relation.EmptyValue = ""
		}
	}
	if relation.Kind == "children" && strings.TrimSpace(relation.Order) == "" {
		relation.Order = "id asc"
	}
	return relation
}

func MergeOptionMap(target map[string]any, incoming map[string]any) map[string]any {
	if target == nil {
		target = map[string]any{}
	}

	for key, value := range incoming {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		target[key] = value
	}

	return target
}
