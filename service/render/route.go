package render

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	devercache "github.com/shemic/dever/cache"
	"github.com/shemic/dever/server"

	frontpage "github.com/dever-package/front/service/page"
	"github.com/dever-package/front/service/runtimecache"
	"github.com/dever-package/front/service/siteconfig"
)

var routeCache = devercache.New[string, []TemplateRoute](
	devercache.WithTTL(5*time.Minute),
	devercache.WithMaxEntries(128),
)

func init() {
	runtimecache.Register("front.render.routes", clearRouteCache, clearRouteCache)
}

func clearRouteCache() {
	routeCache.Clear()
}

func RegisterRoutes(s server.Server, site siteconfig.Site) {
	routes, err := RoutesForSite(site)
	if err != nil {
		panic(fmt.Errorf("读取模板路由失败: %w", err))
	}
	registerTemplateRoutes(s, site, routes)
}

func RegisterSite(s server.Server, site siteconfig.Site) {
	routes, err := RoutesForSite(site)
	if err != nil {
		panic(fmt.Errorf("读取模板路由失败: %w", err))
	}
	if len(routes) == 0 {
		return
	}
	RegisterAssets(s, site)
	registerTemplateRoutes(s, site, routes)
}

func registerTemplateRoutes(s server.Server, site siteconfig.Site, routes []TemplateRoute) {
	for _, route := range routes {
		current := route
		s.Get(current.Pattern, func(c *server.Context) error {
			c.SetContext(siteconfig.WithSite(c.Context(), site))
			return Render(c, site, current)
		})
	}
}

func RoutesForSite(site siteconfig.Site) ([]TemplateRoute, error) {
	cacheKey := site.Key + ":" + site.Page + ":" + site.Path
	return routeCache.GetOrSet(cacheKey, func() ([]TemplateRoute, error) {
		return loadRoutesForSite(site)
	})
}

func TryRenderRequest(c *server.Context, site siteconfig.Site) (bool, error) {
	if c == nil {
		return false, nil
	}
	routes, err := RoutesForSite(site)
	if err != nil {
		return false, err
	}
	requestPath := cleanAbsPath(c.Path())
	for _, route := range routes {
		routeValues, ok := matchTemplateRoute(requestPath, route)
		if !ok {
			continue
		}
		return true, renderWithRouteValues(c, site, route, routeValues)
	}
	return false, nil
}

func loadRoutesForSite(site siteconfig.Site) ([]TemplateRoute, error) {
	routes := []TemplateRoute{}
	err := frontpage.WalkComponentPages(site.Page, func(item frontpage.ComponentPage) error {
		var envelope pageEnvelope
		if err := json.Unmarshal(item.Content, &envelope); err != nil {
			return fmt.Errorf("页面配置解析失败: %s", item.Path)
		}
		if !strings.EqualFold(strings.TrimSpace(envelope.Page.Render), "template") {
			return nil
		}
		if strings.TrimSpace(envelope.Template.Route) == "" || strings.TrimSpace(envelope.Template.View) == "" {
			return fmt.Errorf("模板页面必须声明 template.route 和 template.view: %s", item.Path)
		}
		route := buildTemplateRoute(site, item.Path, item.Content, envelope)
		if route.Pattern == "" {
			return fmt.Errorf("模板页面 route 不合法: %s", item.Path)
		}
		if isReservedTemplateRoute(route.Route, site) {
			return fmt.Errorf("模板页面 route 占用系统路径: %s", item.Path)
		}
		routes = append(routes, route)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(routes, func(i, j int) bool {
		return routeSpecificity(routes[i]) > routeSpecificity(routes[j])
	})
	return routes, nil
}

func buildTemplateRoute(site siteconfig.Site, pagePath string, content []byte, envelope pageEnvelope) TemplateRoute {
	relativeRoute := cleanAbsPath(envelope.Template.Route)
	if relativeRoute == "" {
		return TemplateRoute{}
	}
	routePath := cleanAbsPath(path.Join(site.Path, strings.Trim(relativeRoute, "/")))
	pattern, params := compileTemplateRoutePattern(routePath)
	return TemplateRoute{
		SiteKey:  site.Key,
		PageName: site.Page,
		Path:     pagePath,
		Route:    routePath,
		Config:   envelope.Template,
		Page:     envelope.Page,
		Content:  content,
		Params:   params,
		Pattern:  pattern,
	}
}

func compileTemplateRoutePattern(routePath string) (string, []string) {
	routePath = cleanAbsPath(routePath)
	if routePath == "" {
		return "", nil
	}
	if routePath == "/" {
		return "/", nil
	}
	parts := strings.Split(strings.Trim(routePath, "/"), "/")
	params := []string{}
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, ":") {
			name := strings.Trim(strings.TrimPrefix(part, ":"), " ")
			if name == "" {
				return "", nil
			}
			params = append(params, name)
			parts[index] = ":" + name
			continue
		}
		if part == "" || strings.ContainsAny(part, "*?[]{}") {
			return "", nil
		}
		parts[index] = part
	}
	if len(parts) == 0 {
		return "/", params
	}
	return "/" + strings.Join(parts, "/"), params
}

func routeSpecificity(route TemplateRoute) int {
	score := len(strings.Split(strings.Trim(route.Route, "/"), "/")) * 10
	score -= len(route.Params) * 2
	return score
}

func isReservedTemplateRoute(routePath string, site siteconfig.Site) bool {
	routePath = cleanAbsPath(routePath)
	prefix := cleanAbsPath(site.Path)
	if routePath == prefix {
		return false
	}
	rel := ""
	if prefix == "/" {
		rel = strings.Trim(routePath, "/")
	} else {
		if !strings.HasPrefix(routePath, prefix+"/") {
			return false
		}
		rel = strings.Trim(strings.TrimPrefix(routePath, prefix+"/"), "/")
	}
	if rel == "" {
		return false
	}
	root := rel
	if index := strings.Index(root, "/"); index >= 0 {
		root = root[:index]
	}
	switch root {
	case "main", "route", "upload", "resource", "import", "export", "runtime.js", "assets":
		return true
	default:
		return false
	}
}

func matchTemplateRoute(requestPath string, route TemplateRoute) (map[string]any, bool) {
	requestParts := routePathParts(requestPath)
	routeParts := routePathParts(route.Route)
	if len(requestParts) != len(routeParts) {
		return nil, false
	}

	values := map[string]any{}
	for index, routePart := range routeParts {
		requestPart := requestParts[index]
		if strings.HasPrefix(routePart, ":") {
			name := strings.TrimSpace(strings.TrimPrefix(routePart, ":"))
			if name == "" || requestPart == "" {
				return nil, false
			}
			values[name] = requestPart
			continue
		}
		if routePart != requestPart {
			return nil, false
		}
	}
	return values, true
}

func routePathParts(value string) []string {
	value = strings.Trim(cleanAbsPath(value), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func routeParams(c *server.Context, route TemplateRoute) map[string]any {
	params := make(map[string]any, len(route.Params))
	for _, name := range route.Params {
		value := strings.TrimSpace(c.Input(name))
		if value != "" {
			params[name] = value
		}
	}
	return params
}
