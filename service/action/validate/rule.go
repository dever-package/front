package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	actionpayload "my/package/front/service/action/internal/payload"
	frontcall "my/package/front/service/internal/call"
	frontpage "my/package/front/service/page"
	frontrecord "my/package/front/service/record"
)

var validateEmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func validateRuleValue(
	c *server.Context,
	pathValue string,
	submitModelName string,
	item validateItem,
	rule validateRule,
	form map[string]any,
) (*Failure, error) {
	if !shouldRunBackendRule(rule, form) {
		return nil, nil
	}

	fieldPath := normalizeFormPath(item.Value)
	value := readFormPathValue(form, fieldPath)
	normalized := normalizeValidationValue(value)

	switch strings.ToLower(strings.TrimSpace(rule.Type)) {
	case "required":
		if isEmptyValidationValue(normalized) {
			return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, "该字段不能为空。")}, nil
		}
		if strings.TrimSpace(item.Type) == "form-cascader" {
			requiredLevels := resolveCascaderRequiredLevels(item)
			if requiredLevels > 0 && isSliceValue(normalized) && sliceLen(normalized) < requiredLevels {
				return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, "该字段不能为空。")}, nil
			}
		}
		return nil, nil
	case "email":
		if !frontrecord.HasValue(normalized) {
			return nil, nil
		}
		if validateEmailPattern.MatchString(fmt.Sprint(normalized)) {
			return nil, nil
		}
		return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, "请输入有效的邮箱地址。")}, nil
	case "pattern":
		if !frontrecord.HasValue(normalized) || strings.TrimSpace(rule.Pattern) == "" {
			return nil, nil
		}
		matched, err := regexp.MatchString(rule.Pattern, fmt.Sprint(normalized))
		if err != nil {
			return nil, fmt.Errorf("validate.pattern 配置错误")
		}
		if matched {
			return nil, nil
		}
		return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, "输入格式不正确。")}, nil
	case "min":
		if normalized == nil || normalized == "" || rule.Min == nil {
			return nil, nil
		}
		if validateMinRule(normalized, *rule.Min) {
			return nil, nil
		}
		return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, fmt.Sprintf("长度不能少于 %.0f。", *rule.Min))}, nil
	case "max":
		if normalized == nil || normalized == "" || rule.Max == nil {
			return nil, nil
		}
		if validateMaxRule(normalized, *rule.Max) {
			return nil, nil
		}
		return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, fmt.Sprintf("长度不能超过 %.0f。", *rule.Max))}, nil
	case "sameas":
		if strings.TrimSpace(rule.Target) == "" {
			return nil, nil
		}
		targetValue := readFormPathValue(form, rule.Target)
		if fmt.Sprint(normalized) == fmt.Sprint(normalizeValidationValue(targetValue)) {
			return nil, nil
		}
		return &Failure{Field: fieldPath, Message: util.FirstNonEmpty(rule.Message, "两次输入不一致。")}, nil
	case "model":
		return validateModelRule(c, pathValue, submitModelName, item, rule, form)
	case "service":
		return validateServiceRule(c, item, rule, form)
	default:
		return nil, nil
	}
}

func validateModelRule(
	c *server.Context,
	pathValue string,
	submitModelName string,
	item validateItem,
	rule validateRule,
	form map[string]any,
) (*Failure, error) {
	fieldPath := normalizeFormPath(item.Value)
	value := normalizeValidationValue(readFormPathValue(form, fieldPath))
	if !frontrecord.HasValue(value) {
		return nil, nil
	}

	operator := strings.ToLower(strings.TrimSpace(rule.Operator))
	if operator == "" {
		operator = "unique"
	}
	if operator != "unique" {
		return nil, fmt.Errorf("validate.model.operator 不支持: %s", rule.Operator)
	}

	modelName := strings.TrimSpace(rule.Model)
	if modelName == "" {
		modelName = strings.TrimSpace(submitModelName)
	}
	if modelName == "" {
		modelName = frontpage.DefaultModelName(pathValue)
	}

	modelValue := frontrecord.Resolve(modelName)
	if modelValue == nil {
		return nil, fmt.Errorf("validate.model 不支持 Count")
	}

	columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
	fieldName := strings.TrimSpace(rule.Field)
	if fieldName == "" {
		fieldName = lastPathSegment(fieldPath)
	}
	columnName := frontrecord.ResolveColumnName(columnLookup, fieldName)
	if columnName == "" {
		columnName = util.ToSnake(fieldName)
	}

	filters := map[string]any{
		columnName: value,
	}

	exceptValue := actionpayload.ResolveSaveReference(rule.Except, form)
	if frontrecord.HasValue(exceptValue) {
		pkColumn := frontrecord.ResolveColumnName(columnLookup, "id")
		if pkColumn == "" {
			pkColumn = "id"
		}
		filters[pkColumn] = map[string]any{"!=": actionpayload.NormalizeIdentifier(exceptValue)}
	}

	if modelValue.Count(c.Context(), filters) == 0 {
		return nil, nil
	}

	return &Failure{
		Field:   fieldPath,
		Message: util.FirstNonEmpty(rule.Message, "该字段已存在，请更换。"),
	}, nil
}

func validateServiceRule(
	c *server.Context,
	item validateItem,
	rule validateRule,
	form map[string]any,
) (*Failure, error) {
	serviceName := strings.TrimSpace(rule.Service)
	if serviceName == "" {
		return nil, fmt.Errorf("validate.service 不能为空")
	}

	payload := any(form)
	if rule.Params != nil {
		payload = actionpayload.ResolveTemplateValue(rule.Params, form)
	}

	result, err := frontcall.Service(c, serviceName, payload)
	if err != nil {
		return nil, err
	}

	valid, field, message := parseServiceValidationResult(result)
	if valid {
		return nil, nil
	}

	if strings.TrimSpace(field) == "" {
		field = normalizeFormPath(item.Value)
	}

	return &Failure{
		Field:   field,
		Message: util.FirstNonEmpty(message, rule.Message, "字段校验失败。"),
	}, nil
}

func parseServiceValidationResult(result any) (bool, string, string) {
	switch current := result.(type) {
	case nil:
		return true, "", ""
	case bool:
		return current, "", ""
	case string:
		if strings.TrimSpace(current) == "" {
			return true, "", ""
		}
		return false, "", current
	case map[string]any:
		field := util.ToString(current["field"])
		message := util.FirstNonEmpty(util.ToString(current["message"]), util.ToString(current["msg"]))
		if valid, ok := current["valid"].(bool); ok {
			return valid, field, message
		}
		if status, ok := current["status"]; ok {
			return util.ToInt64(status) == 1, field, message
		}
		if code, ok := current["code"]; ok {
			return util.ToInt64(code) == 0, field, message
		}
		return true, field, message
	default:
		return true, "", ""
	}
}
