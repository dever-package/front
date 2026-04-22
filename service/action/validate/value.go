package validate

import (
	"fmt"
	"reflect"
	"strings"
)

func normalizeValidationValue(value any) any {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return value
}

func validateMinRule(value any, min float64) bool {
	switch current := value.(type) {
	case int:
		return float64(current) >= min
	case int8:
		return float64(current) >= min
	case int16:
		return float64(current) >= min
	case int32:
		return float64(current) >= min
	case int64:
		return float64(current) >= min
	case uint:
		return float64(current) >= min
	case uint8:
		return float64(current) >= min
	case uint16:
		return float64(current) >= min
	case uint32:
		return float64(current) >= min
	case uint64:
		return float64(current) >= min
	case float32:
		return float64(current) >= min
	case float64:
		return current >= min
	default:
		return float64(len(fmt.Sprint(current))) >= min
	}
}

func validateMaxRule(value any, max float64) bool {
	switch current := value.(type) {
	case int:
		return float64(current) <= max
	case int8:
		return float64(current) <= max
	case int16:
		return float64(current) <= max
	case int32:
		return float64(current) <= max
	case int64:
		return float64(current) <= max
	case uint:
		return float64(current) <= max
	case uint8:
		return float64(current) <= max
	case uint16:
		return float64(current) <= max
	case uint32:
		return float64(current) <= max
	case uint64:
		return float64(current) <= max
	case float32:
		return float64(current) <= max
	case float64:
		return current <= max
	default:
		return float64(len(fmt.Sprint(current))) <= max
	}
}

func lastPathSegment(path string) string {
	normalized := normalizeFormPath(path)
	if strings.HasPrefix(normalized, "form.") {
		normalized = strings.TrimPrefix(normalized, "form.")
	}
	parts := strings.Split(normalized, ".")
	if len(parts) == 0 {
		return normalized
	}
	return parts[len(parts)-1]
}

func isSliceValue(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.ValueOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

func sliceLen(value any) int {
	if value == nil {
		return 0
	}
	return reflect.ValueOf(value).Len()
}

func isEmptyValidationValue(value any) bool {
	switch current := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(current) == ""
	default:
		return isSliceValue(current) && sliceLen(current) == 0
	}
}

func resolveCascaderRequiredLevels(item validateItem) int {
	if item.Meta != nil {
		switch current := item.Meta["requiredLevels"].(type) {
		case int:
			if current > 0 {
				return current
			}
		case int64:
			if current > 0 {
				return int(current)
			}
		case float64:
			if current > 0 {
				return int(current)
			}
		}

		if placeholders, ok := item.Meta["placeholder"].([]any); ok && len(placeholders) > 0 {
			return len(placeholders)
		}
	}

	return 0
}
