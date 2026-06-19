package action

import (
	"context"

	"github.com/shemic/dever/util"

	frontpage "github.com/dever-package/front/service/page"
)

func resolveSavePayload(ctx context.Context, config frontpage.ActionConfig, form map[string]any) (any, error) {
	if config.Data == nil {
		return form, nil
	}
	resolved := frontpage.ResolveTemplateValue(config.Data, frontpage.TemplateContext{
		Context: ctx,
		Form:    form,
		Payload: form,
	})

	if !util.ToBool(form["_partial"]) {
		return resolved, nil
	}

	resolvedMap, ok := resolved.(map[string]any)
	if !ok {
		return resolved, nil
	}

	filtered := make(map[string]any, len(resolvedMap))
	for key, val := range resolvedMap {
		if _, exists := form[key]; exists {
			filtered[key] = val
		}
	}
	return filtered, nil
}

func resolveDeletePayload(ctx context.Context, config frontpage.ActionConfig, payload any) any {
	if config.Filters == nil {
		return payload
	}

	values, ok := payload.(map[string]any)
	if !ok {
		return config.Filters
	}
	return frontpage.ResolveTemplateValue(config.Filters, frontpage.TemplateContext{
		Context: ctx,
		Form:    values,
		Payload: payload,
	})
}
