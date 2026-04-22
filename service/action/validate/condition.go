package validate

import (
	"fmt"
	"strings"

	actionpayload "github.com/dever-package/front/service/action/payload"
)

func shouldRunBackendRule(rule validateRule, form map[string]any) bool {
	if len(rule.When) == 0 {
		return true
	}

	results := make([]bool, 0, len(rule.When))
	for _, condition := range rule.When {
		results = append(results, matchBackendCondition(condition, form))
	}

	if strings.EqualFold(strings.TrimSpace(rule.Condition), "any") {
		for _, matched := range results {
			if matched {
				return true
			}
		}
		return false
	}

	for _, matched := range results {
		if !matched {
			return false
		}
	}
	return true
}

func matchBackendCondition(condition validateCondition, form map[string]any) bool {
	current := normalizeValidationValue(readFormPathValue(form, condition.Path))
	operator := strings.ToLower(strings.TrimSpace(condition.Operator))
	if operator == "" {
		operator = "equals"
	}

	switch operator {
	case "empty":
		return isEmptyValidationValue(current)
	case "notempty":
		return !isEmptyValidationValue(current)
	case "notequals":
		return fmt.Sprint(current) != fmt.Sprint(condition.Value)
	default:
		return fmt.Sprint(current) == fmt.Sprint(condition.Value)
	}
}

func readFormPathValue(form map[string]any, path string) any {
	normalized := normalizeFormPath(path)
	if normalized == "" || normalized == "form" {
		return form
	}
	if strings.HasPrefix(normalized, "form.") {
		return actionpayload.GetByDotPath(form, strings.TrimPrefix(normalized, "form."))
	}
	return actionpayload.GetByDotPath(form, normalized)
}

func normalizeFormPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "data.")
	return path
}

func collectPartialFields(form map[string]any) map[string]struct{} {
	flag, _ := form["_partial"].(bool)
	if !flag {
		return nil
	}

	fields := make(map[string]struct{})
	for key := range form {
		if strings.TrimSpace(key) == "" || strings.HasPrefix(key, "_") {
			continue
		}
		normalized := normalizeFormPath(key)
		fields[normalized] = struct{}{}
		fields["form."+normalized] = struct{}{}
	}
	return fields
}

func shouldValidatePartialField(item validateItem, fieldSet map[string]struct{}) bool {
	if len(fieldSet) == 0 {
		return true
	}

	fieldPath := normalizeFormPath(item.Value)
	if fieldPath == "" {
		return false
	}

	_, ok := fieldSet[fieldPath]
	return ok
}
