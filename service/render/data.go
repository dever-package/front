package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontcall "github.com/dever-package/front/service/internal/call"
	frontmeta "github.com/dever-package/front/service/meta"
	frontpage "github.com/dever-package/front/service/page"
	frontrecord "github.com/dever-package/front/service/record"
	"github.com/dever-package/front/service/siteconfig"
)

func resolveRenderData(c *server.Context, site siteconfig.Site, route TemplateRoute, routeValues map[string]any) (map[string]any, SEO, error) {
	var schema rawSchema
	if err := json.Unmarshal(route.Content, &schema); err != nil {
		return nil, SEO{}, fmt.Errorf("页面配置解析失败")
	}

	data := map[string]any{}
	if len(schema.Data) > 0 {
		if err := json.Unmarshal(schema.Data, &data); err != nil {
			return nil, SEO{}, fmt.Errorf("页面 data 解析失败")
		}
	}

	query := requestQuery(c)
	siteData := frontpage.SiteTemplateData(c.Context())
	resolved := map[string]any{}
	for key, value := range data {
		if key == "seo" {
			continue
		}
		item, err := resolveDataItem(c, route, key, value, routeValues, query, siteData, resolved)
		if err != nil {
			return nil, SEO{}, err
		}
		resolved[key] = item
	}

	seo, err := resolveSEO(data["seo"], c, site, routeValues, query, siteData, resolved)
	if err != nil {
		return nil, SEO{}, err
	}
	if _, exists := resolved["seo"]; !exists {
		resolved["seo"] = map[string]any{
			"title":       seo.Title,
			"description": seo.Description,
			"image":       seo.Image,
			"canonical":   seo.Canonical,
		}
	}
	return resolved, seo, nil
}

func resolveDataItem(
	c *server.Context,
	route TemplateRoute,
	key string,
	value any,
	routeValues map[string]any,
	query map[string]any,
	siteData map[string]any,
	data map[string]any,
) (any, error) {
	container, ok := value.(map[string]any)
	if !ok {
		return frontpage.ResolveTemplateValue(value, templateContext(c, routeValues, query, siteData, data)), nil
	}

	resolvedContainer, _ := frontpage.ResolveTemplateValue(container, templateContext(c, routeValues, query, siteData, data)).(map[string]any)
	if resolvedContainer == nil {
		resolvedContainer = map[string]any{}
	}

	serviceName := util.ToStringTrimmed(resolvedContainer["service"])
	modelName := modelNameFromContainer(resolvedContainer)
	if modelName != "" && serviceName != "" {
		return nil, fmt.Errorf("模板 data.%s 不能同时声明 model 和 service", key)
	}
	if serviceName != "" {
		return resolveServiceContainer(c, route, key, serviceName, resolvedContainer, routeValues, query, data)
	}
	if modelName != "" {
		return resolveModelContainer(c, key, modelName, resolvedContainer)
	}
	return resolveNestedData(c, route, resolvedContainer, routeValues, query, siteData, data)
}

func resolveNestedData(
	c *server.Context,
	route TemplateRoute,
	container map[string]any,
	routeValues map[string]any,
	query map[string]any,
	siteData map[string]any,
	data map[string]any,
) (map[string]any, error) {
	result := make(map[string]any, len(container))
	for key, value := range container {
		resolved, err := resolveDataItem(c, route, key, value, routeValues, query, siteData, data)
		if err != nil {
			return nil, err
		}
		result[key] = resolved
	}
	return result, nil
}

func resolveModelContainer(c *server.Context, key string, modelName string, container map[string]any) (any, error) {
	model := frontrecord.Resolve(modelName)
	if model == nil {
		return nil, fmt.Errorf("model 未注册: %s", modelName)
	}

	filters := buildFilters(container)
	order := util.ToStringTrimmed(container["order"])
	options := map[string]any{}
	if order != "" {
		options["order"] = order
	}

	if util.ToBool(container["one"]) {
		row := model.FindMap(c.Context(), filters, options)
		if len(row) == 0 {
			if util.ToBool(container["required"]) {
				return nil, errNotFound
			}
			return map[string]any{}, nil
		}
		rows := frontmeta.AttachRelations(c.Context(), modelName, []map[string]any{row})
		rows = frontmeta.HideFields(modelName, rows)
		return rows[0], nil
	}

	pageSize := util.ToIntDefault(container["pageSize"], 10)
	if pageSize <= 0 {
		pageSize = 10
	}
	page := util.ToIntDefault(container["page"], 1)
	if page <= 0 {
		page = 1
	}
	options["page"] = page
	options["pageSize"] = pageSize

	rows := model.SelectMap(c.Context(), filters, options)
	total := model.Count(c.Context(), filters)
	rows = frontmeta.AttachRelations(c.Context(), modelName, rows)
	rows = frontmeta.HideFields(modelName, rows)

	result := make(map[string]any, len(container)+4)
	for itemKey, itemValue := range container {
		if isDataMetaField(itemKey) {
			continue
		}
		result[itemKey] = itemValue
	}
	result["list"] = rows
	result["total"] = total
	result["page"] = page
	result["pageSize"] = pageSize
	if key != "" && result["key"] == nil {
		result["key"] = key
	}
	return result, nil
}

func resolveServiceContainer(
	c *server.Context,
	route TemplateRoute,
	key string,
	serviceName string,
	container map[string]any,
	routeValues map[string]any,
	query map[string]any,
	data map[string]any,
) (any, error) {
	result, err := frontcall.Service(c, serviceName, map[string]any{
		"key":       key,
		"path":      route.Path,
		"route":     routeValues,
		"query":     query,
		"data":      data,
		"container": container,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func templateContext(
	c *server.Context,
	routeValues map[string]any,
	query map[string]any,
	siteData map[string]any,
	data map[string]any,
) frontpage.TemplateContext {
	return frontpage.TemplateContext{
		Context: c.Context(),
		Data:    data,
		Route:   routeValues,
		Query:   query,
		Site:    siteData,
		Payload: map[string]any{"data": data},
	}
}

func modelNameFromContainer(container map[string]any) string {
	return util.ToStringTrimmed(container["model"])
}

func buildFilters(container map[string]any) any {
	if filters, ok := container["defaultFilters"].(map[string]any); ok {
		return util.CloneMap(filters)
	}
	return map[string]any{}
}

func isDataMetaField(key string) bool {
	switch strings.TrimSpace(key) {
	case "model", "one", "required", "defaultFilters", "filterFields", "searchFields", "order", "service":
		return true
	default:
		return false
	}
}

func requestQuery(c *server.Context) map[string]any {
	result := map[string]any{}
	if c == nil || c.Raw == nil {
		return result
	}
	raw, ok := c.Raw.(interface {
		Queries() map[string]string
	})
	if !ok {
		return result
	}
	for key, value := range raw.Queries() {
		result[key] = value
	}
	return result
}
