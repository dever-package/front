package eval

import "encoding/json"

func cloneJSONValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
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
