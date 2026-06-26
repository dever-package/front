package api

import (
	"context"
	"net/url"
	"strings"

	dlog "github.com/shemic/dever/log"
	"github.com/shemic/dever/observe"
	"github.com/shemic/dever/server"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	actionservice "github.com/dever-package/front/service/action"
	optionservice "github.com/dever-package/front/service/option"
	pageservice "github.com/dever-package/front/service/page"
	permissionservice "github.com/dever-package/front/service/permission"
	"github.com/dever-package/front/service/siteconfig"
)

type Route struct{}

const (
	maxBatchInfoItems   = 50
	maxBatchOptionItems = 100
)

type batchInfoRequest struct {
	Paths []batchInfoItem `json:"paths"`
}

type batchInfoItem struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
}

type batchOptionRequest struct {
	Options []map[string]string `json:"options"`
}

type batchResult struct {
	Code    int    `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func (Route) GetInfo(c *server.Context) error {
	rawPath := c.Input("path", "required", "页面路径")
	pathValue, pathQuery := normalizeOptionPathInput(rawPath)
	query := mergeRouteQuery(pathQuery, collectRequestQuery(c))
	var accessScope *permissionservice.AccessScope
	currentSchema, err := buildRouteInfo(c, pathValue, &accessScope, requestInputLookup(pathQuery, c.Input), query)
	if err != nil {
		if permissionservice.IsPermissionDenied(err) {
			return permissionDeniedPayload(c, err)
		}
		logRouteRuntimeError(c, "front.route.info_error", pathValue, err, nil)
		return c.Error(err)
	}

	return c.JSON(currentSchema)
}

func (Route) GetData(c *server.Context) error {
	rawPath := c.Input("__route", "required", "页面路径")
	pathValue, pathQuery := normalizeOptionPathInput(rawPath)
	dataKey := strings.TrimSpace(c.Input("__dataKey", "required", "数据键"))
	query := mergeRouteQuery(pathQuery, collectRequestQuery(c))
	delete(query, "__route")
	delete(query, "__dataKey")

	var accessScope *permissionservice.AccessScope
	currentSchema, err := buildRouteInfo(c, pathValue, &accessScope, requestInputLookup(pathQuery, c.Input), query)
	if err != nil {
		if permissionservice.IsPermissionDenied(err) {
			return permissionDeniedPayload(c, err)
		}
		logRouteRuntimeError(c, "front.route.data_error", pathValue, err, dlog.Fields{
			"data_key": dataKey,
		})
		return c.Error(err)
	}

	payload, err := pageservice.ExtractDataContainer(currentSchema, dataKey)
	if err != nil {
		logRouteRuntimeError(c, "front.route.data_error", pathValue, err, dlog.Fields{
			"data_key": dataKey,
		})
		return c.Error(err)
	}
	return c.JSON(payload)
}

func logRouteRuntimeError(c *server.Context, event string, routePath string, err error, extra dlog.Fields) {
	if err == nil {
		return
	}

	fields := dlog.Fields{
		"route_path": routePath,
		"error":      dlog.ErrorValue(err),
	}
	if c != nil {
		ctx := c.Context()
		observe.RecordError(ctx, err)
		fields["trace_id"] = observe.TraceID(ctx)
		fields["span_id"] = observe.SpanID(ctx)
		fields["method"] = c.Method()
		fields["path"] = c.Path()
		if origin := c.Header("Origin"); origin != "" {
			fields["origin"] = origin
		}
		if referer := c.Header("Referer"); referer != "" {
			fields["referer"] = referer
		}
		if clientPage := c.Header("X-Client-Page"); clientPage != "" {
			fields["client_page"] = clientPage
		}
	}
	for key, value := range extra {
		fields[key] = value
	}
	dlog.ErrorFields(event, "front route runtime failed", fields)
}

func permissionDeniedPayload(c *server.Context, err error) error {
	return c.JSONPayload(403, map[string]any{
		"code":   403,
		"status": 2,
		"msg":    err.Error(),
		"data":   nil,
	})
}

func (Route) PostBatchInfo(c *server.Context) error {
	var request batchInfoRequest
	if err := c.BindJSON(&request); err != nil {
		return c.Error("请求体格式错误")
	}
	if len(request.Paths) == 0 {
		return c.JSON([]batchResult{})
	}
	if len(request.Paths) > maxBatchInfoItems {
		return c.Error("批量页面配置数量超过限制")
	}

	results := make([]batchResult, 0, len(request.Paths))
	var accessScope *permissionservice.AccessScope
	for _, item := range request.Paths {
		pathValue := frontpagepath.NormalizePath(item.Path)
		if pathValue == "" {
			results = append(results, batchResult{Code: 400, Message: "页面路径不能为空"})
			continue
		}
		if len(item.Query) > 0 {
			results = append(results, batchResult{
				Code:    400,
				Path:    pathValue,
				Message: "批量页面配置暂不支持独立查询参数",
			})
			continue
		}

		currentSchema, err := buildRouteInfo(c, pathValue, &accessScope, nil, nil)
		if err != nil {
			results = append(results, batchResult{
				Code:    routeInfoErrorCode(err),
				Path:    pathValue,
				Message: err.Error(),
			})
			continue
		}
		results = append(results, batchResult{
			Code: 0,
			Path: pathValue,
			Data: currentSchema,
		})
	}

	return c.JSON(results)
}

func isSystemPageInfoPath(ctx context.Context, pathValue string) bool {
	pathValue = frontpagepath.NormalizePath(pathValue)
	if pathValue == "" {
		return false
	}
	if site, ok := siteconfig.FromContext(ctx); ok {
		return isSiteSystemPagePath(site, pathValue)
	}

	cfg, err := siteconfig.Load(ctx)
	if err == nil {
		for _, site := range cfg.Sites {
			if isSiteSystemPagePath(site, pathValue) {
				return true
			}
		}
	}
	return isSiteSystemPagePath(siteconfig.Site{API: siteconfig.DefaultAPI}, pathValue)
}

func isSiteSystemPagePath(site siteconfig.Site, pathValue string) bool {
	return pathValue == site.SystemPagePath("login") ||
		pathValue == site.SystemPagePath("main")
}

func (Route) GetOption(c *server.Context) error {
	pathValue, pathQuery := normalizeOptionPathInput(c.Input("path"))
	if pathValue == "" {
		return c.Error("option.path 不能为空")
	}
	if strings.TrimSpace(c.Input("key")) == "" {
		return c.Error("option.key 不能为空")
	}
	lookup := requestInputLookup(pathQuery, c.Input)
	var accessScope *permissionservice.AccessScope
	if err := ensureRouteOptionAccess(c, &accessScope, pathValue, lookup); err != nil {
		return permissionDeniedPayload(c, err)
	}

	return optionservice.Get(c)
}

func (Route) PostBatchOption(c *server.Context) error {
	var request batchOptionRequest
	if err := c.BindJSON(&request); err != nil {
		return c.Error("请求体格式错误")
	}
	if len(request.Options) == 0 {
		return c.JSON([]batchResult{})
	}
	if len(request.Options) > maxBatchOptionItems {
		return c.Error("批量选项数量超过限制")
	}

	results := make([]batchResult, 0, len(request.Options))
	var accessScope *permissionservice.AccessScope
	for _, params := range request.Options {
		pathValue, optionParams := normalizeOptionParams(params)
		lookup := mapStringLookup(optionParams)
		if pathValue == "" || strings.TrimSpace(optionParams["key"]) == "" {
			results = append(results, batchResult{Code: 400, Path: pathValue, Message: "option.path 和 option.key 不能为空"})
			continue
		}
		if err := ensureRouteOptionAccess(c, &accessScope, pathValue, lookup); err != nil {
			results = append(results, batchResult{
				Code:    403,
				Path:    pathValue,
				Message: err.Error(),
			})
			continue
		}

		items, err := optionservice.GetByInput(c, mapStringLookup(optionParams))
		if err != nil {
			results = append(results, batchResult{
				Code:    routeInfoErrorCode(err),
				Path:    pathValue,
				Message: err.Error(),
			})
			continue
		}
		results = append(results, batchResult{
			Code: 0,
			Path: pathValue,
			Data: items,
		})
	}

	return c.JSON(results)
}

func ensureRouteOptionAccess(
	c *server.Context,
	accessScope **permissionservice.AccessScope,
	pathValue string,
	lookup permissionservice.InputLookup,
) error {
	if *accessScope == nil {
		scope, err := permissionservice.NewAccessScope(c.Context())
		if err != nil {
			return err
		}
		*accessScope = scope
	}
	return (*accessScope).EnsurePageAccess(c.Context(), pathValue, lookup)
}

func (Route) PostAction(c *server.Context) error {
	return actionservice.PostAction(c)
}

func buildRouteInfo(
	c *server.Context,
	pathValue string,
	accessScope **permissionservice.AccessScope,
	lookup permissionservice.InputLookup,
	query map[string]string,
) (pageservice.Schema, error) {
	if isSystemPageInfoPath(c.Context(), pathValue) {
		return pageservice.BuildInfo(c, pathValue)
	}

	if *accessScope == nil {
		scope, err := permissionservice.NewAccessScope(c.Context())
		if err != nil {
			return pageservice.Schema{}, err
		}
		*accessScope = scope
	}
	if err := (*accessScope).EnsurePageAccess(c.Context(), pathValue, lookup); err != nil {
		return pageservice.Schema{}, err
	}

	currentSchema, err := pageservice.BuildInfo(c, pathValue)
	if err != nil {
		return pageservice.Schema{}, err
	}
	if err := (*accessScope).FilterPageSchema(c.Context(), pathValue, &currentSchema, lookup, query); err != nil {
		return pageservice.Schema{}, err
	}
	return currentSchema, nil
}

func collectRequestQuery(c *server.Context) map[string]string {
	if c == nil {
		return nil
	}
	return parseQueryFromURL(routeOriginalURL(c))
}

func mergeRouteQuery(pathQuery, requestQuery map[string]string) map[string]string {
	if len(pathQuery) == 0 {
		return requestQuery
	}
	if len(requestQuery) == 0 {
		return pathQuery
	}

	result := make(map[string]string, len(pathQuery)+len(requestQuery))
	for key, value := range pathQuery {
		result[key] = value
	}
	for key, value := range requestQuery {
		result[key] = value
	}
	return result
}

func routeOriginalURL(c *server.Context) string {
	if c == nil || c.Raw == nil {
		return ""
	}
	if raw, ok := c.Raw.(interface{ OriginalURL() string }); ok {
		return raw.OriginalURL()
	}
	return c.Path()
}

func parseQueryFromURL(rawURL string) map[string]string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	rawQuery := ""
	if parsed, err := url.Parse(rawURL); err == nil {
		rawQuery = parsed.RawQuery
	}
	if rawQuery == "" {
		_, rawQuery, _ = strings.Cut(rawURL, "?")
	}
	if rawQuery == "" {
		return nil
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil || len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, items := range values {
		key = strings.TrimSpace(key)
		if key == "" || len(items) == 0 {
			continue
		}
		if value := strings.TrimSpace(items[0]); value != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func routeInfoErrorCode(err error) int {
	if err == nil {
		return 0
	}
	if permissionservice.IsPermissionDenied(err) {
		return 403
	}
	return 400
}

func mapStringLookup(values map[string]string) permissionservice.InputLookup {
	return func(key string) string {
		return values[key]
	}
}

func requestInputLookup(pathQuery map[string]string, fallback func(string, ...string) string) permissionservice.InputLookup {
	return func(key string) string {
		if fallback != nil {
			if value := strings.TrimSpace(fallback(key)); value != "" {
				return value
			}
		}
		return pathQuery[key]
	}
}

func normalizeOptionParams(params map[string]string) (string, map[string]string) {
	rawPath := strings.TrimSpace(params["path"])
	pathValue, pathQuery := normalizeOptionPathInput(rawPath)
	if len(pathQuery) == 0 && pathValue == rawPath {
		return pathValue, params
	}

	result := make(map[string]string, len(params)+len(pathQuery))
	for key, value := range params {
		if key == "path" {
			continue
		}
		result[key] = value
	}
	for key, value := range pathQuery {
		if strings.TrimSpace(result[key]) == "" {
			result[key] = value
		}
	}
	if rawPath != "" {
		result["path"] = rawPath
	} else if pathValue != "" {
		result["path"] = pathValue
	}
	return pathValue, result
}

func normalizeOptionPathInput(rawPath string) (string, map[string]string) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", nil
	}

	pathValue := rawPath
	rawQuery := ""
	if index := strings.IndexAny(rawPath, "?#"); index >= 0 {
		pathValue = rawPath[:index]
		if rawPath[index] == '?' {
			rawQuery = rawPath[index+1:]
			if hashIndex := strings.Index(rawQuery, "#"); hashIndex >= 0 {
				rawQuery = rawQuery[:hashIndex]
			}
		}
	}

	return frontpagepath.NormalizePath(pathValue), parseOptionPathQuery(rawQuery)
}

func parseOptionPathQuery(rawQuery string) map[string]string {
	if strings.TrimSpace(rawQuery) == "" {
		return nil
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil || len(values) == 0 {
		return nil
	}

	result := make(map[string]string, len(values))
	for key, items := range values {
		if key == "" || len(items) == 0 {
			continue
		}
		result[key] = items[0]
	}
	return result
}
