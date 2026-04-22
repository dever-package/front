package meta

import (
	"strings"

	"github.com/shemic/dever/util"

	frontrecord "github.com/dever-package/front/service/record"
)

var modelLabelCache util.ConcurrentMap[string, modelLabelMap]

func ResolveFieldLabel(modelName, field string) string {
	modelName = strings.TrimSpace(modelName)
	field = normalizeLabelField(field)
	if field == "" {
		return ""
	}

	modelLabels := resolveModelLabels(modelName)
	if label := modelLabels.label(field); label != "" {
		return label
	}

	for _, relation := range ResolveModelRelations(modelName) {
		if label := resolveRelationFieldLabel(modelLabels, relation, field); label != "" {
			return label
		}
	}

	return ""
}

type modelLabelMap map[string]string

func resolveModelLabels(modelName string) modelLabelMap {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	if cached, ok := modelLabelCache.Load(modelName); ok {
		return cached
	}

	adapter := frontrecord.ResolveAdapter(modelName)
	if adapter == nil {
		return nil
	}
	labels := modelLabelMap(adapter.Labels())
	modelLabelCache.Store(modelName, labels)
	return labels
}

func (m modelLabelMap) label(field string) string {
	if len(m) == 0 {
		return ""
	}
	field = normalizeLabelField(field)
	if field == "" {
		return ""
	}
	if label := strings.TrimSpace(m[normalizeLabelKey(field)]); label != "" {
		return label
	}
	return ""
}

func relationLabel(labels modelLabelMap, relation Relation) string {
	for _, candidate := range relationFieldCandidates(relation) {
		if label := labels.label(candidate); label != "" {
			return label
		}
	}

	if optionLabel := relatedEntityLabel(strings.TrimSpace(relation.Option)); optionLabel != "" {
		return optionLabel
	}
	if childLabel := relatedEntityLabel(strings.TrimSpace(relation.Through)); childLabel != "" {
		return childLabel
	}
	return ""
}

func resolveRelationFieldLabel(labels modelLabelMap, relation Relation, field string) string {
	field = normalizeLabelField(field)
	if field == "" {
		return ""
	}

	base := field
	childField := ""
	if idx := strings.Index(field, "."); idx != -1 {
		base = strings.TrimSpace(field[:idx])
		childField = strings.TrimSpace(field[idx+1:])
	}

	if !relationMatchesField(relation, base) {
		if relationMatchesField(relation, field) {
			return relationLabel(labels, relation)
		}
		return ""
	}

	if childField == "" {
		return relationLabel(labels, relation)
	}

	if nestedLabel := relatedModelFieldLabel(relation, childField); nestedLabel != "" {
		if childField == "name" || childField == "label" || childField == "value" || childField == "title" {
			if label := relationLabel(labels, relation); label != "" {
				return label
			}
		}
		return nestedLabel
	}

	return relationLabel(labels, relation)
}

func relationMatchesField(relation Relation, field string) bool {
	field = normalizeLabelField(field)
	if field == "" {
		return false
	}
	for _, candidate := range relationFieldCandidates(relation) {
		if normalizeLabelField(candidate) == field {
			return true
		}
	}
	return false
}

func relationFieldCandidates(relation Relation) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, 6)
	appendCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeLabelField(value)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}

	field := strings.TrimSpace(relation.Field)
	name := strings.TrimSpace(relation.Name)
	appendCandidate(field)
	appendCandidate(name)
	switch {
	case strings.HasSuffix(field, "_ids"):
		base := strings.TrimSuffix(field, "_ids")
		appendCandidate(base)
		appendCandidate(base + "_id")
		appendCandidate(base + "s")
	case strings.HasSuffix(field, "_id"):
		base := strings.TrimSuffix(field, "_id")
		appendCandidate(base)
	}
	return result
}

func relatedModelFieldLabel(relation Relation, field string) string {
	field = normalizeLabelField(field)
	if field == "" {
		return ""
	}

	for _, modelName := range []string{
		strings.TrimSpace(relation.Option),
		strings.TrimSpace(relation.Through),
	} {
		if modelName == "" {
			continue
		}
		if label := resolveModelLabels(modelName).label(field); label != "" {
			return trimEntityLabelSuffix(label)
		}
	}
	return ""
}

func relatedEntityLabel(modelName string) string {
	if strings.TrimSpace(modelName) == "" {
		return ""
	}

	labels := resolveModelLabels(modelName)
	label := labels.label("name")
	if label == "" {
		return ""
	}
	return trimEntityLabelSuffix(label)
}

func trimEntityLabelSuffix(label string) string {
	label = strings.TrimSpace(label)
	for _, suffix := range []string{"名称", "ID", "Id", "id"} {
		if strings.HasSuffix(label, suffix) {
			label = strings.TrimSpace(strings.TrimSuffix(label, suffix))
			break
		}
	}
	return label
}

func normalizeLabelKey(field string) string {
	field = normalizeLabelField(field)
	return strings.ToLower(strings.ReplaceAll(field, "_", ""))
}

func normalizeLabelField(field string) string {
	field = strings.TrimSpace(field)
	field = strings.TrimPrefix(field, "data.")
	field = strings.TrimPrefix(field, "state.")
	field = strings.TrimPrefix(field, "form.")
	field = strings.TrimPrefix(field, "search.")
	return strings.TrimSpace(field)
}
