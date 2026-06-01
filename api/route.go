package api

import (
	"context"

	"github.com/shemic/dever/server"

	frontpagepath "my/package/front/internal/pagepath"
	actionservice "my/package/front/service/action"
	optionservice "my/package/front/service/option"
	pageservice "my/package/front/service/page"
	permissionservice "my/package/front/service/permission"
	"my/package/front/service/siteconfig"
)

type Route struct{}

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
	pathValue := frontpagepath.NormalizePath(c.Input("path", "required", "页面路径"))
	var accessScope *permissionservice.AccessScope
	currentSchema, err := buildRouteInfo(c, pathValue, &accessScope, func(key string) string {
		return c.Input(key)
	})
	if err != nil {
		if permissionservice.IsPermissionDenied(err) {
			return permissionDeniedPayload(c, err)
		}
		return c.Error(err)
	}

	return c.JSON(currentSchema)
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

		currentSchema, err := buildRouteInfo(c, pathValue, &accessScope, nil)
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
	pathValue := frontpagepath.NormalizePath(c.Input("path"))
	if pathValue != "" {
		if err := permissionservice.EnsurePageAccessWithInput(c.Context(), pathValue, func(key string) string {
			return c.Input(key)
		}); err != nil {
			return permissionDeniedPayload(c, err)
		}
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

	results := make([]batchResult, 0, len(request.Options))
	for _, params := range request.Options {
		pathValue := frontpagepath.NormalizePath(params["path"])
		if pathValue != "" {
			if err := permissionservice.EnsurePageAccessWithInput(c.Context(), pathValue, mapStringLookup(params)); err != nil {
				results = append(results, batchResult{
					Code:    403,
					Path:    pathValue,
					Message: err.Error(),
				})
				continue
			}
		}

		items, err := optionservice.GetByInput(c, mapStringLookup(params))
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

func (Route) PostAction(c *server.Context) error {
	return actionservice.PostAction(c)
}

func buildRouteInfo(
	c *server.Context,
	pathValue string,
	accessScope **permissionservice.AccessScope,
	lookup permissionservice.InputLookup,
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
	if err := (*accessScope).FilterPageSchema(pathValue, &currentSchema); err != nil {
		return pageservice.Schema{}, err
	}
	return currentSchema, nil
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
