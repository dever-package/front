package cron

import (
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeJSONObject(raw any, fallback string) (string, error) {
	text := strings.TrimSpace(stringValue(raw))
	if text == "" {
		text = fallback
	}
	if text == "" {
		text = "{}"
	}

	var value map[string]any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return "", fmt.Errorf("JSON 格式无效: %w", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("JSON 格式无效: %w", err)
	}
	return string(data), nil
}

func decodeJSONObject(raw string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func encodeJSONValue(value any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func stringValue(value any) string {
	switch current := value.(type) {
	case nil:
		return ""
	case string:
		return current
	case []byte:
		return string(current)
	default:
		data, err := json.Marshal(current)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
