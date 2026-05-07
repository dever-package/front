package page

import (
	"strings"

	frontmeta "my/package/front/service/meta"
)

func DefaultModelLabel(pathValue string) string {
	modelName := strings.TrimSpace(DefaultModelName(pathValue))
	if modelName == "" {
		return ""
	}
	return frontmeta.ResolveModelName(modelName)
}

func DefaultPageTitle(pathValue string) string {
	label := DefaultModelLabel(pathValue)
	if label == "" {
		return ""
	}
	return composeActionTitle(defaultRouteAction(pathValue), label)
}

func DefaultActionTitle(actionKey, pathValue string) string {
	label := DefaultModelLabel(pathValue)
	if label == "" {
		return ""
	}

	action := defaultActionFromKey(actionKey)
	if action == "" {
		action = defaultRouteAction(pathValue)
	}
	return composeActionTitle(action, label)
}

func defaultRouteAction(pathValue string) string {
	segments := splitPathSegments(pathValue)
	if len(segments) == 0 {
		return ""
	}
	return defaultActionFromKey(segments[len(segments)-1])
}

func defaultActionFromKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '/' || r == ' '
	})
	for i := len(parts) - 1; i >= 0; i-- {
		switch parts[i] {
		case "create", "add", "new":
			return "create"
		case "edit", "update":
			return "edit"
		case "detail", "view", "info":
			return "detail"
		}
	}
	return ""
}

func composeActionTitle(action, label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}

	switch strings.TrimSpace(action) {
	case "create":
		return "新增" + label
	case "edit":
		return "编辑" + label
	case "detail":
		return label + "详情"
	default:
		return label
	}
}
