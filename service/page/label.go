package page

import (
	"encoding/json"
	"strings"

	frontmeta "github.com/dever-package/front/service/meta"
	"github.com/shemic/dever/util"
)

func applyNodeLabels(rawNodes json.RawMessage, pathValue string, content []byte) (json.RawMessage, error) {
	if len(rawNodes) == 0 {
		return rawNodes, nil
	}

	var nodes map[string][]map[string]any
	if err := json.Unmarshal(rawNodes, &nodes); err != nil {
		return nil, err
	}
	modelName := nodeLabelModelName(content, pathValue)
	if !applyNodeLabelMap(nodes, modelName, pathValue) {
		return rawNodes, nil
	}

	content, err := json.Marshal(nodes)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(content), nil
}

func ApplyNodeLabelMap(nodes map[string][]map[string]any, modelName string) bool {
	pathValue := ""
	if strings.Contains(strings.TrimSpace(modelName), "/") {
		pathValue = modelName
		modelName = DefaultModelName(pathValue)
	}
	return applyNodeLabelMap(nodes, modelName, pathValue)
}

func applyNodeLabelMap(nodes map[string][]map[string]any, modelName, pathValue string) bool {
	if len(nodes) == 0 {
		return false
	}

	if strings.TrimSpace(modelName) == "" && strings.TrimSpace(pathValue) == "" {
		return false
	}

	changed := false
	for _, items := range nodes {
		for _, item := range items {
			if applyItemLabel(item, modelName, pathValue) {
				changed = true
			}
		}
	}
	return changed
}

func nodeLabelModelName(content []byte, pathValue string) string {
	for _, candidate := range []string{
		SubmitModelName(content, pathValue),
		explicitDataModelName(content),
		DefaultModelName(pathValue),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func explicitDataModelName(content []byte) string {
	var payload struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(content, &payload); err != nil || len(payload.Data) == 0 {
		return ""
	}

	if modelName := explicitDataContainerModelName(payload.Data["form"]); modelName != "" {
		return modelName
	}
	return explicitDataContainerModelName(payload.Data["table"])
}

func explicitDataContainerModelName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var current map[string]any
	if err := json.Unmarshal(raw, &current); err != nil || len(current) == 0 {
		return ""
	}

	if modelName := explicitFormModelName(current); modelName != "" {
		return modelName
	}
	if modelName := util.ToStringTrimmed(current["model"]); modelName != "" {
		return modelName
	}
	return ""
}

func applyItemLabel(item map[string]any, modelName, pathValue string) bool {
	if len(item) == 0 {
		return false
	}

	changed := false
	if applyItemActionTitle(item, pathValue) {
		changed = true
	}
	label := resolveItemLabel(modelName, item)
	if strings.TrimSpace(toString(item["name"])) == "" {
		if label != "" {
			item["name"] = label
			changed = true
		}
	}
	if applyItemPlaceholder(item, firstNonEmptyString(toString(item["name"]), label)) {
		changed = true
	}
	if applyItemOption(item, modelName) {
		changed = true
	}

	meta, _ := item["meta"].(map[string]any)
	if len(meta) > 0 && applyColumnDefaults(meta, modelName, pathValue) {
		item["meta"] = meta
		changed = true
	}

	children := normalizeNodeItems(item["items"])
	if len(children) == 0 {
		return changed
	}
	for _, child := range children {
		if applyItemLabel(child, modelName, pathValue) {
			changed = true
		}
	}
	item["items"] = children
	return changed
}

func applyItemOption(item map[string]any, modelName string) bool {
	if strings.TrimSpace(toString(item["option"])) != "" {
		return false
	}

	if strings.TrimSpace(toString(item["type"])) == "show-category-list" {
		if key := defaultCategoryOptionKey(item); key != "" {
			item["option"] = "option." + key
			return true
		}
		return false
	}

	if !canInferItemOption(item) {
		return false
	}
	field := normalizeItemValueField(toString(item["value"]))
	if field == "" || !frontmeta.HasModelOptionKey(modelName, field) {
		return false
	}

	item["option"] = "option." + field
	return true
}

func canInferItemOption(item map[string]any) bool {
	switch strings.TrimSpace(toString(item["type"])) {
	case "form-select", "form-radio", "form-checkbox", "show-tag", "show-select", "show-status":
		return true
	default:
		return false
	}
}

func normalizeItemValueField(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"data.", "state.", "form.", "search."} {
		value = strings.TrimPrefix(value, prefix)
	}
	return strings.TrimSpace(value)
}

func applyItemActionTitle(item map[string]any, pathValue string) bool {
	switch strings.TrimSpace(toString(item["type"])) {
	case "show-button":
		if strings.TrimSpace(toString(item["name"])) != "" {
			return false
		}
		actionKey := modalStateKeyFromAction(item["action"])
		if actionKey == "" {
			return false
		}
		title := DefaultActionTitle(actionKey, pathValue)
		if title == "" {
			return false
		}
		item["name"] = title
		return true
	case "feedback-modal", "feedback-drawer":
		meta, _ := item["meta"].(map[string]any)
		if len(meta) == 0 || strings.TrimSpace(toString(meta["title"])) != "" {
			return false
		}
		routePath := firstNonEmptyString(toString(meta["pageRoute"]), pathValue)
		title := DefaultActionTitle(toString(meta["stateKey"]), routePath)
		if title == "" {
			return false
		}
		meta["title"] = title
		item["meta"] = meta
		return true
	default:
		return false
	}
}

func modalStateKeyFromAction(raw any) string {
	actionMap, _ := raw.(map[string]any)
	if len(actionMap) == 0 {
		return ""
	}
	return modalStateKeyFromActionValue(actionMap["click"])
}

func modalStateKeyFromActionValue(raw any) string {
	switch current := raw.(type) {
	case map[string]any:
		if strings.TrimSpace(toString(current["type"])) == "modal" {
			return toString(current["key"])
		}
		for _, value := range current {
			if key := modalStateKeyFromActionValue(value); key != "" {
				return key
			}
		}
	case []any:
		for _, value := range current {
			if key := modalStateKeyFromActionValue(value); key != "" {
				return key
			}
		}
	}
	return ""
}

func applyItemPlaceholder(item map[string]any, label string) bool {
	label = strings.TrimSpace(label)
	if label == "" || !isFormNodeType(toString(item["type"])) {
		return false
	}
	if hasPlaceholder(item["placeholder"]) {
		return false
	}

	prefix := formPlaceholderPrefix(toString(item["type"]))
	if prefix == "" {
		return false
	}
	item["placeholder"] = prefix + label
	return true
}

func hasPlaceholder(value any) bool {
	if value == nil {
		return false
	}
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current) != ""
	default:
		return true
	}
}

func isFormNodeType(nodeType string) bool {
	return strings.HasPrefix(strings.TrimSpace(nodeType), "form-")
}

func formPlaceholderPrefix(nodeType string) string {
	switch strings.TrimSpace(nodeType) {
	case "form-select", "form-radio", "form-checkbox", "form-date", "form-tree", "form-cascader", "form-icon":
		return "请选择"
	case "form-upload":
		return "请上传"
	case "form-switch":
		return ""
	default:
		return "请输入"
	}
}

func applyColumnDefaults(meta map[string]any, modelName, pathValue string) bool {
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
		if applyColumnLabel(column, modelName) {
			changed = true
		}
		if applyColumnTip(column) {
			changed = true
		}
		if applyColumnActionDescriptions(column, pathValue) {
			changed = true
		}
	}
	if changed {
		meta["columns"] = columns
	}
	return changed
}

func applyColumnLabel(column map[string]any, modelName string) bool {
	if strings.TrimSpace(toString(column["name"])) != "" {
		return false
	}
	value := strings.TrimSpace(toString(column["value"]))
	if value == "" {
		return false
	}
	label := frontmeta.ResolveFieldLabel(modelName, value)
	if label == "" {
		return false
	}
	column["name"] = label
	return true
}

func applyColumnTip(column map[string]any) bool {
	if strings.TrimSpace(toString(column["tip"])) != "" || strings.TrimSpace(toString(column["info"])) != "" {
		return false
	}
	if strings.TrimSpace(toString(column["type"])) != "form-switch" {
		return false
	}
	column["tip"] = "点击开关将自动提交数据"
	return true
}

func applyColumnActionDescriptions(column map[string]any, pathValue string) bool {
	columnType := strings.TrimSpace(toString(column["type"]))
	if columnType != "show-button" && columnType != "show-link" {
		return false
	}

	meta, _ := column["meta"].(map[string]any)
	if len(meta) == 0 {
		return false
	}

	rawButtons, exists := meta["buttons"]
	if !exists || rawButtons == nil {
		return false
	}

	buttons := make([]map[string]any, 0)
	if err := decodeNodeJSONValue(rawButtons, &buttons); err != nil || len(buttons) == 0 {
		return false
	}

	changed := false
	for _, button := range buttons {
		if strings.TrimSpace(toString(button["description"])) != "" {
			continue
		}
		description := inferActionButtonDescription(button, pathValue)
		if description == "" {
			continue
		}
		button["description"] = description
		changed = true
	}
	if changed {
		meta["buttons"] = buttons
		column["meta"] = meta
	}
	return changed
}

func inferActionButtonDescription(button map[string]any, pathValue string) string {
	if to := strings.TrimSpace(toString(button["to"])); to != "" {
		if title := DefaultPageTitle(to); title != "" {
			return title
		}
	}

	if actionKey := modalStateKeyFromAction(button["action"]); actionKey != "" {
		return DefaultActionTitle(actionKey, pathValue)
	}

	return ""
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
