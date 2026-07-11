package page

import (
	"context"
	"encoding/json"

	"github.com/dever-package/front/service/siteconfig"
)

var routeReferenceKeys = map[string]struct{}{
	"active_paths": {},
	"href":         {},
	"pageRoute":    {},
	"path":         {},
	"savePath":     {},
	"to":           {},
}

func ExternalizeSchemaRoutes(ctx context.Context, currentSchema *Schema) error {
	if currentSchema == nil {
		return nil
	}
	site, ok := siteconfig.FromContext(ctx)
	if !ok || !site.HasRouteAlias() {
		return nil
	}

	for _, current := range []*json.RawMessage{
		&currentSchema.Page,
		&currentSchema.Layout,
		&currentSchema.Nodes,
		&currentSchema.Data,
		&currentSchema.State,
		&currentSchema.Action,
	} {
		if err := externalizeRawRouteReferences(site, current); err != nil {
			return err
		}
	}
	return nil
}

func externalizeRawRouteReferences(site siteconfig.Site, raw *json.RawMessage) error {
	if raw == nil || len(*raw) == 0 {
		return nil
	}

	var payload any
	if err := json.Unmarshal(*raw, &payload); err != nil {
		return err
	}
	next, changed := externalizeRouteReferences(site, payload, "")
	if !changed {
		return nil
	}
	content, err := json.Marshal(next)
	if err != nil {
		return err
	}
	*raw = json.RawMessage(content)
	return nil
}

func externalizeRouteReferences(site siteconfig.Site, value any, parentKey string) (any, bool) {
	switch current := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range current {
			if _, ok := routeReferenceKeys[key]; ok {
				next, childChanged := externalizeRouteReferenceValue(site, child)
				if childChanged {
					current[key] = next
					changed = true
				}
				continue
			}
			next, childChanged := externalizeRouteReferences(site, child, key)
			if childChanged {
				current[key] = next
				changed = true
			}
		}
		return current, changed
	case []any:
		changed := false
		for index, child := range current {
			next, childChanged := externalizeRouteReferences(site, child, parentKey)
			if childChanged {
				current[index] = next
				changed = true
			}
		}
		return current, changed
	case string:
		if _, ok := routeReferenceKeys[parentKey]; !ok {
			return value, false
		}
		return externalizeRouteReferenceString(site, current)
	default:
		return value, false
	}
}

func externalizeRouteReferenceValue(site siteconfig.Site, value any) (any, bool) {
	switch current := value.(type) {
	case string:
		return externalizeRouteReferenceString(site, current)
	case []any:
		changed := false
		for index, child := range current {
			next, childChanged := externalizeRouteReferenceValue(site, child)
			if childChanged {
				current[index] = next
				changed = true
			}
		}
		return current, changed
	default:
		return externalizeRouteReferences(site, value, "")
	}
}

func externalizeRouteReferenceString(site siteconfig.Site, value string) (any, bool) {
	next := site.ExternalPagePath(value)
	if next == value {
		return value, false
	}
	return next, true
}
