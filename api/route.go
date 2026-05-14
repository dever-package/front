package api

import (
	"github.com/shemic/dever/server"

	actionservice "my/package/front/service/action"
	optionservice "my/package/front/service/option"
	pageservice "my/package/front/service/page"
	permissionservice "my/package/front/service/permission"
)

type Route struct{}

func (Route) GetInfo(c *server.Context) error {
	pathValue := pageservice.NormalizePath(c.Input("path", "required", "页面路径"))
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

func (Route) GetOption(c *server.Context) error {
	pathValue := pageservice.NormalizePath(c.Input("path"))
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
