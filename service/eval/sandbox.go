package eval

import (
	"encoding/json"
	"fmt"
)

func cloneJSONValue(value any) (any, error) {
	return cloneJSONValueWithLimit(value, 0, 0, 0)
}

func cloneJSONValueWithLimit(value any, maxBytes int, maxDepth int, maxArrayLength int) (any, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && len(raw) > maxBytes {
		return nil, fmt.Errorf("JSON 数据超过 %d 字节限制", maxBytes)
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	if err := validateJSONShape(cloned, 0, maxDepth, maxArrayLength); err != nil {
		return nil, err
	}
	return cloned, nil
}

func validateJSONShape(value any, depth int, maxDepth int, maxArrayLength int) error {
	if maxDepth > 0 && depth > maxDepth {
		return fmt.Errorf("JSON 数据超过 %d 层深度限制", maxDepth)
	}
	switch current := value.(type) {
	case map[string]any:
		for _, item := range current {
			if err := validateJSONShape(item, depth+1, maxDepth, maxArrayLength); err != nil {
				return err
			}
		}
	case []any:
		if maxArrayLength > 0 && len(current) > maxArrayLength {
			return fmt.Errorf("JSON 数组超过 %d 项限制", maxArrayLength)
		}
		for _, item := range current {
			if err := validateJSONShape(item, depth+1, maxDepth, maxArrayLength); err != nil {
				return err
			}
		}
	}
	return nil
}

func jsonEqual(left any, right any) (bool, error) {
	leftValue, err := cloneJSONValue(left)
	if err != nil {
		return false, err
	}
	rightValue, err := cloneJSONValue(right)
	if err != nil {
		return false, err
	}
	leftRaw, err := json.Marshal(leftValue)
	if err != nil {
		return false, err
	}
	rightRaw, err := json.Marshal(rightValue)
	if err != nil {
		return false, err
	}
	return string(leftRaw) == string(rightRaw), nil
}
