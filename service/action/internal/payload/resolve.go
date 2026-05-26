package payload

import (
	"strconv"
	"strings"

	"github.com/shemic/dever/util"
)

func ResolveTemplateValue(value any, form map[string]any) any {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, item := range current {
			result[key] = ResolveTemplateValue(item, form)
		}
		return result
	case []any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			result = append(result, ResolveTemplateValue(item, form))
		}
		return result
	case string:
		return ResolveSaveReference(current, form)
	default:
		return value
	}
}

func ResolveSaveReference(value string, form map[string]any) any {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "$") {
		return value
	}
	if trimmed == "$form" {
		return form
	}
	if strings.HasPrefix(trimmed, "$form.") {
		return GetByDotPath(form, strings.TrimPrefix(trimmed, "$form."))
	}
	return nil
}

func GetByDotPath(source any, path string) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return source
	}

	current := source
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" || current == nil {
			return nil
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[segment]
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}

	return current
}

func NormalizeIdentifier(value any) any {
	if number, ok := util.ParseInt64(value); ok {
		return number
	}
	return value
}
