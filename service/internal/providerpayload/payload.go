package providerpayload

import (
	"fmt"
	"strconv"
	"strings"
)

func Map(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	switch value := params[0].(type) {
	case map[string]any:
		if value != nil {
			return value
		}
	}
	return map[string]any{}
}

func Text(payload map[string]any, keys ...string) string {
	value := First(payload, keys...)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func Int(payload map[string]any, fallback int, keys ...string) int {
	value := First(payload, keys...)
	switch current := value.(type) {
	case int:
		return current
	case int64:
		return int(current)
	case float64:
		return int(current)
	case jsonNumber:
		parsed, err := strconv.Atoi(current.String())
		if err == nil {
			return parsed
		}
	default:
		parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func Uint64(payload map[string]any, keys ...string) uint64 {
	value := First(payload, keys...)
	switch current := value.(type) {
	case uint64:
		return current
	case uint:
		return uint64(current)
	case int:
		if current > 0 {
			return uint64(current)
		}
	case int64:
		if current > 0 {
			return uint64(current)
		}
	case float64:
		if current > 0 {
			return uint64(current)
		}
	case jsonNumber:
		parsed, err := strconv.ParseUint(current.String(), 10, 64)
		if err == nil {
			return parsed
		}
	default:
		parsed, err := strconv.ParseUint(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func First(payload map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := payload[key]; exists {
			return value
		}
	}
	return nil
}

type jsonNumber interface {
	String() string
}
