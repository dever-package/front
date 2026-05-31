package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	deverjwt "github.com/shemic/dever/auth/jwt"
	"github.com/shemic/dever/config"
	coremiddleware "github.com/shemic/dever/middleware"
	"github.com/shemic/dever/server"

	permissionservice "my/package/front/service/permission"
	"my/package/front/service/siteconfig"
)

var registerOnce sync.Once

func Register() {
	registerOnce.Do(func() {
		coremiddleware.UseGlobalFunc(auth())
		coremiddleware.UseGlobalFunc(apiScopeGuard())
		coremiddleware.UseGlobalFunc(frontBootstrap())
	})
}

func auth() coremiddleware.ContextFunc {
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Errorf("读取配置失败: %w", err))
	}
	if err := deverjwt.Configure(cfg.Auth); err != nil {
		panic(fmt.Errorf("初始化 JWT 认证失败: %w", err))
	}
	frontConfig, err := siteconfig.Load(nil)
	if err != nil {
		panic(fmt.Errorf("读取 front 站点配置失败: %w", err))
	}
	publicPaths := frontConfig.AllPublicPaths()
	allowPluginDevAssets := siteconfig.PluginDevEnabled(cfg.FrontSite)

	return deverjwt.UseConfigured(deverjwt.Options{
		Allow: func(c *server.Context) bool {
			path := strings.TrimSpace(c.Path())
			_, isStaticSitePath := frontConfig.FindByStaticSitePath(path)
			return isPluginDevAssetPath(allowPluginDevAssets, path) ||
				siteconfig.MatchPublicPath(publicPaths, path) ||
				isStaticSitePath ||
				isPublicRouteInfoRequest(frontConfig, c, path)
		},
		AllowMissing: func(*server.Context) bool {
			return false
		},
		PublicPaths: publicPaths,
		OnUnauthorized: func(c *server.Context, msg string) error {
			return abortUnauthorized(c, msg)
		},
	})
}

func frontBootstrap() coremiddleware.ContextFunc {
	frontConfig := siteconfig.MustLoad()

	return func(ctx any) error {
		c, ok := ctx.(*server.Context)
		if !ok || c == nil {
			return nil
		}
		path := strings.TrimSpace(c.Path())
		site, ok := siteconfig.FromContext(c.Context())
		if !ok {
			site, ok = frontConfig.FindByAPIRequestPath(path)
		}
		if !ok {
			return nil
		}
		return permissionservice.EnsureBootstrapForSite(c.Context(), site)
	}
}

func apiScopeGuard() coremiddleware.ContextFunc {
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Errorf("读取配置失败: %w", err))
	}
	frontConfig := siteconfig.MustLoad()
	publicPaths := frontConfig.AllPublicPaths()
	allowPluginDevAssets := siteconfig.PluginDevEnabled(cfg.FrontSite)

	return func(ctx any) error {
		c, ok := ctx.(*server.Context)
		if !ok || c == nil {
			return nil
		}
		path := strings.TrimSpace(c.Path())
		_, isStaticSitePath := frontConfig.FindByStaticSitePath(path)
		if isPluginDevAssetPath(allowPluginDevAssets, path) || isStaticSitePath {
			return nil
		}
		if siteconfig.MatchPublicPath(publicPaths, path) ||
			isPublicRouteInfoRequest(frontConfig, c, path) {
			attachRequestSite(c, frontConfig, path)
			return nil
		}

		site, ok := frontConfig.FindByAPIRequestPath(path)
		if !ok {
			return nil
		}
		c.SetContext(siteconfig.WithSite(c.Context(), site))
		if tokenAllowsSite(c, site) {
			return nil
		}
		return abortUnauthorized(c, "无权访问当前站点接口")
	}
}

func attachRequestSite(c *server.Context, frontConfig siteconfig.Config, path string) {
	if site, ok := frontConfig.FindByRequestPath(path); ok {
		c.SetContext(siteconfig.WithSite(c.Context(), site))
	}
}

func tokenAllowsSite(c *server.Context, site siteconfig.Site) bool {
	claims := deverjwt.Claims(c.Context())
	scope := claimString(claims["scope"])
	siteKey := claimString(claims["site"])
	provider := strings.TrimSpace(site.Access.AuthProvider)

	if siteKey == site.Key || scope == provider {
		return true
	}
	if siteKey == "" && scope == "" && provider == siteconfig.DefaultAuthProvider {
		return true
	}
	return false
}

func claimString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func isPluginDevAssetPath(enabled bool, path string) bool {
	return enabled && siteconfig.IsPluginDevProxyPath(path)
}

func isPublicRouteInfoRequest(frontConfig siteconfig.Config, c *server.Context, requestPath string) bool {
	site, ok := frontConfig.FindByAPIRequestPath(requestPath)
	if !ok {
		return false
	}
	if cleanRequestPath(requestPath) != cleanRequestPath(site.APIPrefix()+"/route/info") {
		return false
	}
	return normalizePublicPagePath(c.Query("path")) == site.SystemPagePath("login")
}

func normalizePublicPagePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	value = strings.ReplaceAll(value, "\\", "/")
	return value
}

func cleanRequestPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return "/" + strings.Trim(value, "/")
}

func abortUnauthorized(c *server.Context, msg string) error {
	if c != nil {
		return c.Error(msg, http.StatusUnauthorized)
	}
	return fmt.Errorf("%s", msg)
}
