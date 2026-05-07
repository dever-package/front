package validate

import (
	"fmt"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontpage "my/package/front/service/page"
)

func Form(
	c *server.Context,
	content []byte,
	pathValue string,
	form map[string]any,
) ([]Failure, error) {
	items, err := parseValidationItems(content)
	if err != nil {
		return nil, err
	}

	submitModelName := frontpage.SubmitModelName(content, pathValue)
	failures := make([]Failure, 0)
	partialFieldSet := collectPartialFields(form)
	for _, item := range items {
		if item.Mode != "" && item.Mode != "form" {
			continue
		}
		if len(partialFieldSet) > 0 && !shouldValidatePartialField(item, partialFieldSet) {
			continue
		}
		for _, rule := range item.Validate {
			failure, err := validateRuleValue(c, pathValue, submitModelName, item, rule, form)
			if err != nil {
				return nil, err
			}
			if failure != nil {
				failures = append(failures, *failure)
				break
			}
		}
	}

	return failures, nil
}

func parseValidationItems(content []byte) ([]validateItem, error) {
	signature := frontpage.Signature(content)
	if cached, ok := validateItemsCache.Load(signature); ok {
		return cached, nil
	}

	var envelope validateEnvelope
	if err := util.UnmarshalNormalizedJSON(content, &envelope); err != nil {
		return nil, fmt.Errorf("页面 validate 配置解析失败")
	}

	items := make([]validateItem, 0)
	for _, layoutItems := range envelope.Nodes {
		for _, item := range layoutItems {
			if len(item.Validate) == 0 {
				continue
			}
			items = append(items, item)
		}
	}

	validateItemsCache.Store(signature, items)
	return items, nil
}
