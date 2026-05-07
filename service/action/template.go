package action

import (
	actionpayload "my/package/front/service/action/payload"
	frontpage "my/package/front/service/page"
)

func resolveSavePayload(config frontpage.ActionConfig, form map[string]any) (any, error) {
	if config.Data == nil {
		return form, nil
	}
	return actionpayload.ResolveTemplateValue(config.Data, form), nil
}

func resolveDeletePayload(config frontpage.ActionConfig, payload any) any {
	if config.Filters == nil {
		return payload
	}

	values, ok := payload.(map[string]any)
	if !ok {
		return config.Filters
	}
	return actionpayload.ResolveTemplateValue(config.Filters, values)
}
