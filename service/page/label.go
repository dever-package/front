package page

import (
	"encoding/json"
	"strings"

	frontmeta "github.com/dever-package/front/service/meta"
	"github.com/shemic/dever/util"
)

func applyNodeLabels(rawNodes json.RawMessage, pathValue string) (json.RawMessage, error) {
	if len(rawNodes) == 0 {
		return rawNodes, nil
	}

	var nodes map[string][]map[string]any
	if err := json.Unmarshal(rawNodes, &nodes); err != nil {
		return nil, err
	}
	if !ApplyNodeLabelMap(nodes, pathValue) {
		return rawNodes, nil
	}

	content, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(content), nil
}

func ApplyNodeLabelMap(nodes map[string][]map[string]any, pathValue string) bool {
	if len(nodes) == 0 {
		return false
	}

	modelName := DefaultModelName(pathValue)
	if strings.TrimSpace(modelName) == "" {
		return false
	}

	changed := false
	for _, items := range nodes {
		for _, item := range items {
			if applyItemLabel(item, modelName) {
				changed = true
			}
		}
	}
	return changed
}

func applyItemLabel(item map[string]any, modelName string) bool {
	if len(item) == 0 {
		return false
	}

	changed := false
	if strings.TrimSpace(toString(item["name"])) == "" {
		if label := resolveItemLabel(modelName, item); label != "" {
			item["name"] = label
			changed = true
		}
	}

	meta, _ := item["meta"].(map[string]any)
	if len(meta) > 0 && applyColumnLabels(meta, modelName) {
		item["meta"] = meta
		changed = true
	}

	children := normalizeNodeItems(item["items"])
	if len(children) == 0 {
		return changed
	}
	for _, child := range children {
		if applyItemLabel(child, modelName) {
			changed = true
		}
	}
	item["items"] = children
	return changed
}

func applyColumnLabels(meta map[string]any, modelName string) bool {
	rawColumns, exists := meta["columns"]
	if !exists || rawColumns == nil {
		return false
	}

	columns := make([]map[string]any, 0)
	if err := decodeNodeJSONValue(rawColumns, &columns); err != nil || len(columns) == 0 {
		return false
	}

	changed := false
	for _, column := range columns {
		if strings.TrimSpace(toString(column["name"])) != "" {
			continue
		}
		value := strings.TrimSpace(toString(column["value"]))
		if value == "" {
			continue
		}
		if label := frontmeta.ResolveFieldLabel(modelName, value); label != "" {
			column["name"] = label
			changed = true
		}
	}
	if changed {
		meta["columns"] = columns
	}
	return changed
}

func resolveItemLabel(modelName string, item map[string]any) string {
	value := strings.TrimSpace(toString(item["value"]))
	if value == "" {
		return ""
	}
	return frontmeta.ResolveFieldLabel(modelName, value)
}

func normalizeNodeItems(raw any) []map[string]any {
	items := make([]map[string]any, 0)
	_ = decodeNodeJSONValue(raw, &items)
	return items
}

func decodeNodeJSONValue(raw any, target any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func toString(value any) string {
	return util.ToStringTrimmed(value)
}
