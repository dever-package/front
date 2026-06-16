package action

import (
	"context"

	frontpage "my/package/front/service/page"
)

func resolveSavePayload(ctx context.Context, config frontpage.ActionConfig, form map[string]any) (any, error) {
	if config.Data == nil {
		return form, nil
	}
	return frontpage.ResolveTemplateValue(config.Data, frontpage.TemplateContext{
		Context: ctx,
		Form:    form,
		Payload: form,
	}), nil
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
