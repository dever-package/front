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

func (Route) GetInfo(c *server.Context) error {
	pathValue := frontpagepath.NormalizePath(c.Input("path", "required", "页面路径"))
	if isSystemPageInfoPath(c.Context(), pathValue) {
		currentSchema, err := pageservice.BuildInfo(c, pathValue)
		if err != nil {
			return c.Error(err)
		}
		return c.JSON(currentSchema)
	}

	accessScope, err := permissionservice.NewAccessScope(c.Context())
	if err != nil {
		return c.Error(err)
	}
	if err := accessScope.EnsurePageAccess(c.Context(), pathValue, func(key string) string {
		return c.Input(key)
	}); err != nil {
		return c.JSONPayload(403, map[string]any{
			"code":   403,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}

	currentSchema, err := pageservice.BuildInfo(c, pathValue)
	if err != nil {
		return c.Error(err)
	}
	if err := accessScope.FilterPageSchema(pathValue, &currentSchema); err != nil {
		return c.Error(err)
	}

	return c.JSON(currentSchema)
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
			return c.JSONPayload(403, map[string]any{
				"code":   403,
				"status": 2,
				"msg":    err.Error(),
				"data":   nil,
			})
		}
	}

	return optionservice.Get(c)
}

func (Route) PostAction(c *server.Context) error {
	return actionservice.PostAction(c)
}
