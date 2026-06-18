package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	deverjwt "github.com/shemic/dever/auth/jwt"
	"github.com/shemic/dever/config"
	coremiddleware "github.com/shemic/dever/middleware"
	"github.com/shemic/dever/server"

	cronservice "my/package/front/service/cron"
	permissionservice "my/package/front/service/permission"
	"my/package/front/service/siteconfig"
	"my/package/front/service/upload/openurl"
)

const siteHeader = "X-Dever-Site"
const apiKeyHeader = "X-API-Key"

var registerOnce sync.Once

type middlewareSettings struct {
	authConfig           config.Auth
	frontConfig          siteconfig.Config
	publicPaths          []string
	allowPluginDevAssets bool
}

func Register() {
	registerOnce.Do(func() {
		settings := loadMiddlewareSettings()
		if err := permissionservice.WarmupSites(context.Background(), settings.frontConfig.Sites); err != nil {
			panic(err)
		}
		cronservice.Start()
		server.OnShutdown(func(ctx context.Context) error {
			return cronservice.Stop(ctx)
		})
		coremiddleware.UseGlobalFunc(auth(settings))
		coremiddleware.UseGlobalFunc(apiScopeGuard(settings))
		coremiddleware.UseGlobalFunc(frontBootstrap(settings))
	})
}

func loadMiddlewareSettings() middlewareSettings {
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Errorf("读取配置失败: %w", err))
	}
	frontConfig, err := siteconfig.Load(nil)
	if err != nil {
		panic(fmt.Errorf("读取 front 站点配置失败: %w", err))
	}

	return middlewareSettings{
		authConfig:           cfg.Auth,
		frontConfig:          frontConfig,
		publicPaths:          frontConfig.AllPublicPaths(),
		allowPluginDevAssets: siteconfig.PluginDevEnabled(cfg.FrontSite),
	}
}

func auth(settings middlewareSettings) coremiddleware.ContextFunc {
	if err := deverjwt.Configure(settings.authConfig); err != nil {
		panic(fmt.Errorf("初始化 JWT 认证失败: %w", err))
	}

	return deverjwt.UseConfigured(deverjwt.Options{
		Allow: func(c *server.Context) bool {
			path := strings.TrimSpace(c.Path())
			return isPluginDevAssetPath(settings.allowPluginDevAssets, path) ||
				openurl.IsSignedRequest(c) ||
				siteconfig.MatchPublicPath(settings.publicPaths, path) ||
				isPublicSiteRequest(settings.frontConfig, c, path) ||
				isStaticSiteRequest(settings.frontConfig, c, path) ||
				isPublicRouteSchemaRequest(settings.frontConfig, c, path) ||
				allowAPIKeyRequest(settings.frontConfig, c, path)
		},
		AllowMissing: func(*server.Context) bool {
			return false
		},
		PublicPaths: settings.publicPaths,
		OnUnauthorized: func(c *server.Context, msg string) error {
			return abortUnauthorized(c, msg)
		},
	})
}

func frontBootstrap(settings middlewareSettings) coremiddleware.ContextFunc {
	return func(ctx any) error {
		c, ok := ctx.(*server.Context)
		if !ok || c == nil {
			return nil
		}
		path := strings.TrimSpace(c.Path())
		site, ok := siteconfig.FromContext(c.Context())
		if !ok {
			site, ok = requestSite(settings.frontConfig, c, path)
		}
		if !ok {
			return nil
		}
		return permissionservice.EnsureBootstrapForSite(c.Context(), site)
	}
}

func apiScopeGuard(settings middlewareSettings) coremiddleware.ContextFunc {
	return func(ctx any) error {
		c, ok := ctx.(*server.Context)
		if !ok || c == nil {
			return nil
		}
		path := strings.TrimSpace(c.Path())
		if isPluginDevAssetPath(settings.allowPluginDevAssets, path) ||
			isStaticSiteRequest(settings.frontConfig, c, path) {
			return nil
		}
		if openurl.IsSignedRequest(c) {
			return nil
		}
		if siteconfig.MatchPublicPath(settings.publicPaths, path) ||
			isPublicSiteRequest(settings.frontConfig, c, path) ||
			isPublicRouteSchemaRequest(settings.frontConfig, c, path) {
			if site, ok := requestSite(settings.frontConfig, c, path); ok {
				c.SetContext(siteconfig.WithSite(c.Context(), site))
			}
			return nil
		}

		site, ok := requestSite(settings.frontConfig, c, path)
		if !ok {
			return nil
		}
		c.SetContext(siteconfig.WithSite(c.Context(), site))
		if allowAPIKeyForSite(c, site) {
			return nil
		}
		if tokenAllowsSite(c, site) {
			return nil
		}
		return abortUnauthorized(c, "无权访问当前站点接口")
	}
}

func allowAPIKeyRequest(frontConfig siteconfig.Config, c *server.Context, path string) bool {
	if !hasAPIKeyCredential(c) {
		return false
	}
	if site, ok := requestSite(frontConfig, c, path); ok {
		return allowAPIKeyForSite(c, site)
	}
	return requestPathHasPrefix(path, "/user")
}

func isPublicSiteRequest(frontConfig siteconfig.Config, c *server.Context, path string) bool {
	site, ok := requestSite(frontConfig, c, path)
	if !ok || !site.UsesPublic() {
		return false
	}
	if site.IsPublicRuntimeEndpoint(path) && isPublicSiteRuntimeRequest(c, site, path) {
		return true
	}
	if strings.EqualFold(site.APIPrefix(), siteconfig.DefaultAPI) {
		return false
	}
	return requestPathHasPrefix(path, site.APIPrefix())
}

func isPublicSiteRuntimeRequest(c *server.Context, site siteconfig.Site, path string) bool {
	switch {
	case siteconfig.IsFrontRuntimeAPIEndpoint(path, "main/info"),
		siteconfig.IsFrontRuntimeAPIEndpoint(path, "main/bootstrap"):
		return true
	case siteconfig.IsFrontRuntimeAPIEndpoint(path, "route/info"),
		siteconfig.IsFrontRuntimeAPIEndpoint(path, "route/data"):
		return isSitePagePath(site, c.Input("path")) || isSitePagePath(site, c.Input("__route"))
	case siteconfig.IsFrontRuntimeAPIEndpoint(path, "route/action"),
		siteconfig.IsFrontRuntimeAPIEndpoint(path, "route/option"):
		return isSitePagePath(site, c.Input("path"))
	case siteconfig.IsFrontRuntimeAPIEndpoint(path, "route/batch_info"):
		return batchInfoPathsBelongToSite(c, site)
	case siteconfig.IsFrontRuntimeAPIEndpoint(path, "route/batch_option"):
		return batchOptionPathsBelongToSite(c, site)
	default:
		return false
	}
}

func isSitePagePath(site siteconfig.Site, pathValue string) bool {
	pathValue = normalizePublicPagePath(pathValue)
	if pathValue == "" {
		return false
	}
	return requestPathHasPrefix(pathValue, strings.Trim(site.APIPrefix(), "/"))
}

func allowAPIKeyForSite(c *server.Context, site siteconfig.Site) bool {
	return hasAPIKeyCredential(c) && strings.TrimSpace(site.Access.AuthProvider) == "user"
}

func hasAPIKeyCredential(c *server.Context) bool {
	if c == nil {
		return false
	}
	if strings.TrimSpace(c.Header(apiKeyHeader)) != "" {
		return true
	}
	auth := strings.TrimSpace(c.Header("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		auth = strings.TrimSpace(auth[len("bearer "):])
	}
	return strings.HasPrefix(auth, "uapi_")
}

func requestSite(frontConfig siteconfig.Config, c *server.Context, path string) (siteconfig.Site, bool) {
	if c != nil {
		if site, ok := siteconfig.FromContext(c.Context()); ok {
			return site, true
		}
		if site, ok := requestSiteFromHeader(frontConfig, c); ok {
			return site, true
		}
		if siteconfig.IsFrontRuntimeAPIPath(path) {
			if siteKey := requestSiteKey(c); siteKey != "" {
				return frontConfig.FindBySiteKey(siteKey)
			}
		}
	}
	if hasSiteContextHeader(c) {
		return frontConfig.FindByAPIPrefix(path)
	}
	if site, ok := frontConfig.FindByAPIRequestPath(path); ok {
		return site, true
	}
	return frontConfig.FindBySitePath(path)
}

func requestSiteFromHeader(frontConfig siteconfig.Config, c *server.Context) (siteconfig.Site, bool) {
	if c == nil {
		return siteconfig.Site{}, false
	}
	siteKey := strings.TrimSpace(c.Header(siteHeader))
	if siteKey == "" {
		return siteconfig.Site{}, false
	}
	return frontConfig.FindBySiteKey(siteKey)
}

func requestSiteKey(c *server.Context) string {
	siteKey := strings.TrimSpace(c.Header(siteHeader))
	if siteKey == "" {
		siteKey = claimString(deverjwt.Claims(c.Context())["site"])
	}
	return siteKey
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

func isStaticSiteRequest(frontConfig siteconfig.Config, c *server.Context, path string) bool {
	if hasSiteContextHeader(c) {
		return false
	}
	_, ok := frontConfig.FindByStaticSitePath(path)
	return ok
}

func hasSiteContextHeader(c *server.Context) bool {
	return c != nil && strings.TrimSpace(c.Header(siteHeader)) != ""
}

func requestPathHasPrefix(requestPath, prefix string) bool {
	requestPath = cleanRequestPath(requestPath)
	prefix = cleanRequestPath(prefix)
	return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
}

func isPublicRouteSchemaRequest(frontConfig siteconfig.Config, c *server.Context, requestPath string) bool {
	site, ok := requestSite(frontConfig, c, requestPath)
	if !ok {
		return false
	}
	if isFrontRouteEndpoint(requestPath, site, "route/info") {
		return normalizePublicPagePath(c.Query("path")) == site.SystemPagePath("login")
	}
	if isFrontRouteEndpoint(requestPath, site, "route/batch_info") {
		return isPublicBatchInfoRequest(c, site)
	}
	return false
}

func isFrontRouteEndpoint(requestPath string, site siteconfig.Site, endpoint string) bool {
	requestPath = cleanRequestPath(requestPath)
	if siteconfig.IsFrontRuntimeAPIPath(requestPath) {
		return requestPath == siteconfig.FrontRuntimeAPIPath(endpoint)
	}
	return requestPath == cleanRequestPath(site.APIPrefix()+"/"+endpoint)
}

func isPublicBatchInfoRequest(c *server.Context, site siteconfig.Site) bool {
	paths, ok := batchInfoRequestPaths(c)
	if !ok || len(paths) == 0 {
		return false
	}

	loginPath := site.SystemPagePath("login")
	for _, pathValue := range paths {
		if normalizePublicPagePath(pathValue) != loginPath {
			return false
		}
	}
	return true
}

func batchInfoPathsBelongToSite(c *server.Context, site siteconfig.Site) bool {
	paths, ok := batchInfoRequestPaths(c)
	if !ok || len(paths) == 0 {
		return false
	}
	for _, pathValue := range paths {
		if !isSitePagePath(site, pathValue) {
			return false
		}
	}
	return true
}

func batchInfoRequestPaths(c *server.Context) ([]string, bool) {
	body, ok := requestBody(c)
	if !ok || len(body) == 0 {
		return nil, false
	}

	var request struct {
		Paths []struct {
			Path string `json:"path"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(body, &request); err != nil || len(request.Paths) == 0 {
		return nil, false
	}

	paths := make([]string, 0, len(request.Paths))
	for _, item := range request.Paths {
		pathValue := strings.TrimSpace(item.Path)
		if pathValue == "" {
			return nil, false
		}
		paths = append(paths, pathValue)
	}
	return paths, true
}

func batchOptionPathsBelongToSite(c *server.Context, site siteconfig.Site) bool {
	body, ok := requestBody(c)
	if !ok || len(body) == 0 {
		return false
	}

	var request struct {
		Options []map[string]string `json:"options"`
	}
	if err := json.Unmarshal(body, &request); err != nil || len(request.Options) == 0 {
		return false
	}

	for _, item := range request.Options {
		if !isSitePagePath(site, item["path"]) {
			return false
		}
	}
	return true
}

func requestBody(c *server.Context) ([]byte, bool) {
	if c == nil || c.Raw == nil {
		return nil, false
	}
	raw, ok := c.Raw.(interface{ Body() []byte })
	if !ok {
		return nil, false
	}
	return raw.Body(), true
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
		_ = c.Error(msg, http.StatusUnauthorized)
		panic(server.Abort{Err: fmt.Errorf("%s", msg)})
	}
	return fmt.Errorf("%s", msg)
}
