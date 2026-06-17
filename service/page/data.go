package page

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var dataContainerKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type DataContainerPayload struct {
	Key    string `json:"key"`
	Data   any    `json:"data"`
	Option any    `json:"option,omitempty"`
}

func ExtractDataContainer(schema Schema, key string) (DataContainerPayload, error) {
	key = strings.TrimSpace(key)
	if !dataContainerKeyPattern.MatchString(key) {
		return DataContainerPayload{}, fmt.Errorf("data key 不合法")
	}

	var root map[string]any
	if len(schema.Data) > 0 {
		if err := json.Unmarshal(schema.Data, &root); err != nil {
			return DataContainerPayload{}, fmt.Errorf("页面 data 解析失败")
		}
	}
	if root == nil {
		root = map[string]any{}
	}

	value, ok := root[key]
	if !ok {
		return DataContainerPayload{}, fmt.Errorf("data.%s 不存在", key)
	}

	payload := DataContainerPayload{
		Key:  key,
		Data: value,
	}
	if option, ok := root["option"]; ok {
		payload.Option = option
	}
	return payload, nil
}
