package meta

import (
	"context"
	"strings"
	"sync"

	"github.com/shemic/dever/util"

	frontrecord "github.com/dever-package/front/service/record"
)

type ModelMeta struct {
	Options        map[string]any
	Relations      []Relation
	HiddenFields   []string
	PasswordFields []string
}

var (
	modelMetaMutex sync.RWMutex
	modelMetaStore = map[string]ModelMeta{}
)

func RegisterModelMeta(modelName string, meta ModelMeta) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return
	}

	modelMetaMutex.Lock()
	modelMetaStore[modelName] = meta
	modelMetaMutex.Unlock()
}

func getModelMeta(modelName string) (ModelMeta, bool) {
	modelMetaMutex.RLock()
	meta, ok := modelMetaStore[strings.TrimSpace(modelName)]
	modelMetaMutex.RUnlock()
	return meta, ok
}

func ResolveModelOptions(ctx context.Context, modelName string) map[string]any {
	meta, ok := getModelMeta(modelName)
	if !ok {
		return nil
	}

	result := cloneMetaOptions(meta.Options)
	for _, relation := range meta.Relations {
		result = MergeOptionMap(result, BuildRelationOptions(ctx, normalizeRelation(modelName, relation)))
	}
	return result
}

func ResolveModelRelations(modelName string) []Relation {
	meta, ok := getModelMeta(modelName)
	if !ok || len(meta.Relations) == 0 {
		return nil
	}

	relations := make([]Relation, 0, len(meta.Relations))
	for _, relation := range meta.Relations {
		relations = append(relations, normalizeRelation(modelName, relation))
	}
	return relations
}

func ResolvePasswordFields(modelName string) []string {
	meta, ok := getModelMeta(modelName)
	if !ok || len(meta.PasswordFields) == 0 {
		return nil
	}
	return append([]string(nil), meta.PasswordFields...)
}

func AttachRelations(ctx context.Context, modelName string, rows []map[string]any) []map[string]any {
	meta, ok := getModelMeta(modelName)
	if !ok || len(meta.Relations) == 0 {
		return rows
	}

	result := rows
	for _, relation := range meta.Relations {
		result = AttachRelation(ctx, result, normalizeRelation(modelName, relation))
	}
	return result
}

func HideFields(modelName string, rows []map[string]any) []map[string]any {
	if len(rows) == 0 {
		return rows
	}

	meta, ok := getModelMeta(modelName)
	if !ok || len(meta.HiddenFields) == 0 {
		return rows
	}

	hiddenFields := make(map[string]struct{}, len(meta.HiddenFields))
	for _, field := range meta.HiddenFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		hiddenFields[field] = struct{}{}
		hiddenFields[util.ToSnake(field)] = struct{}{}
	}

	if len(hiddenFields) == 0 {
		return rows
	}

	for _, row := range rows {
		for field := range hiddenFields {
			delete(row, field)
		}
	}

	return rows
}

func SaveModelRelations(ctx context.Context, modelName string, ownerID any, record map[string]any) error {
	meta, ok := getModelMeta(modelName)
	if !ok || len(meta.Relations) == 0 {
		return nil
	}

	for _, relation := range meta.Relations {
		if err := SaveRelation(ctx, ownerID, record, normalizeRelation(modelName, relation)); err != nil {
			return err
		}
	}
	return nil
}

func DeleteModelRelations(ctx context.Context, modelName string, payload any) error {
	meta, ok := getModelMeta(modelName)
	if !ok || len(meta.Relations) == 0 {
		return nil
	}

	for _, relation := range meta.Relations {
		if err := DeleteRelation(ctx, payload, normalizeRelation(modelName, relation)); err != nil {
			return err
		}
	}
	return nil
}

func cloneMetaOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return nil
	}

	result := make(map[string]any, len(options))
	for key, value := range options {
		switch typed := value.(type) {
		case []map[string]any:
			result[key] = util.CloneMapSlice(typed)
		case []any:
			cloned := make([]any, 0, len(typed))
			for _, item := range typed {
				if mapped, ok := item.(map[string]any); ok {
					cloned = append(cloned, util.CloneMap(mapped))
					continue
				}
				cloned = append(cloned, item)
			}
			result[key] = cloned
		case map[string]any:
			result[key] = util.CloneMap(typed)
		default:
			result[key] = value
		}
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
