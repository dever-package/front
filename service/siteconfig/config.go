package siteconfig

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/shemic/dever/util"
)

const (
	DefaultSiteKey      = "admin"
	DefaultPage         = "admin"
	DefaultAPI          = "front"
	DefaultAccessMode   = "rbac"
	DefaultAuthProvider = "front"
	AccessModeRBAC      = "rbac"
	AccessModeLogin     = "login"
)

var (
	defaultGlobalPublic = []string{
		"/upload/*",
		"/bot/energon/request",
		"/bot/energon/demo",
		"/site/info",
		"/qiniu/callback",
	}
	defaultSitePublic   = []string{"auth/login"}
	defaultSiteAPIRoots = []string{"main", "route", "upload", "resource", "import", "export"}
	loadOnce            sync.Once
	loadedConfig        Config
	loadedErr           error
)

type Config struct {
	Public []string
	Sites  []Site
}

type Site struct {
	Key         string
	Path        string
	Page        string
	Name        string
	Subtitle    string
	Description string
	URL         string
	API         string
	APIRoots    []string
	Public      []string
	Assets      SiteAssets
	Setting     SiteSetting
	Access      Access
	Auth        []AuthSeed
	Entry       string
}

type SiteAssets struct {
	Logo    string `json:"logo"`
	Favicon string `json:"favicon"`
}

type SiteSetting struct {
	Appearance AppearanceSetting `json:"appearance"`
	Runtime    RuntimeSetting    `json:"runtime"`
}

type AppearanceSetting struct {
	Theme     string `json:"theme"`
	Sidebar   string `json:"sidebar"`
	Layout    string `json:"layout"`
	Direction string `json:"direction"`
}

type RuntimeSetting struct {
	Skin       string   `json:"skin"`
	RouterMode string   `json:"routerMode"`
	Plugins    []string `json:"plugins,omitempty"`
}

type Access struct {
	Mode         string `json:"mode"`
	AuthProvider string `json:"authProvider"`
}

type AuthSeed struct {
	Key      string            `json:"key"`
	ID       string            `json:"id"`
	Path     string            `json:"path"`
	Name     string            `json:"name"`
	Icon     string            `json:"icon"`
	Parent   string            `json:"parent"`
	Type     int               `json:"type"`
	Sort     int               `json:"sort"`
	Query    map[string]string `json:"query"`
	Children []AuthSeed        `json:"children"`
}

type contextKey struct{}

type rawConfig struct {
	Public []string           `json:"public"`
	Sites  map[string]rawSite `json:"sites"`
	Auth   []AuthSeed         `json:"auth"`
	Entry  string             `json:"entry"`
}

type rawSite struct {
	Name        string      `json:"name"`
	Subtitle    string      `json:"subtitle"`
	Description string      `json:"description"`
	URL         string      `json:"url"`
	Page        string      `json:"page"`
	API         string      `json:"api"`
	APIRoots    []string    `json:"apiRoots"`
	Public      []string    `json:"public"`
	Assets      SiteAssets  `json:"assets"`
	Setting     SiteSetting `json:"setting"`
	Access      Access      `json:"access"`
	Auth        []AuthSeed  `json:"auth"`
	Entry       string      `json:"entry"`
}

func Load(context.Context) (Config, error) {
	loadOnce.Do(func() {
		loadedConfig, loadedErr = load()
	})
	return loadedConfig, loadedErr
}

func load() (Config, error) {
	content, _, err := util.ReadJSONCFile(
		filepath.Join("config", "front.jsonc"),
		filepath.Join("config", "front.json"),
	)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return Config{}, err
	}

	var payload rawConfig
	if err := util.UnmarshalNormalizedJSON(content, &payload); err != nil {
		return Config{}, err
	}
	return normalizeConfig(payload), nil
}

func MustLoad() Config {
	cfg, err := Load(context.Background())
	if err != nil {
		panic(fmt.Errorf("读取 front 站点配置失败: %w", err))
	}
	return cfg
}

func LoadPageNames() map[string]struct{} {
	cfg, err := Load(context.Background())
	if err != nil {
		return nil
	}
	return cfg.PageNames()
}

func WithSite(ctx context.Context, site Site) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, site)
}

func FromContext(ctx context.Context) (Site, bool) {
	if ctx == nil {
		return Site{}, false
	}
	site, ok := ctx.Value(contextKey{}).(Site)
	return site, ok && site.Key != ""
}

func SiteKeyFromContext(ctx context.Context) string {
	if site, ok := FromContext(ctx); ok {
		return site.Key
	}
	return DefaultSiteKey
}

func PageFromContext(ctx context.Context) string {
	if site, ok := FromContext(ctx); ok {
		if site.Page != "" {
			return site.Page
		}
		return site.Key
	}
	return DefaultPage
}

func (cfg Config) FindBySiteKey(siteKey string) (Site, bool) {
	siteKey = normalizeSiteKey(siteKey)
	for _, site := range cfg.Sites {
		if site.Key == siteKey {
			return site, true
		}
	}
	return Site{}, false
}

func (cfg Config) FindByAPIPrefix(apiPrefix string) (Site, bool) {
	requestPath := cleanAbsPath(apiPrefix)
	return cfg.findByPath(requestPath, func(site Site) string {
		return site.APIPrefix()
	})
}

func (cfg Config) FindByAPIRequestPath(requestPath string) (Site, bool) {
	requestPath = cleanAbsPath(requestPath)
	site, ok := cfg.FindByAPIPrefix(requestPath)
	if !ok {
		return Site{}, false
	}
	if site.APIPrefix() != site.Path {
		return site, true
	}
	if isSiteReservedAPIRoot(site, requestPath) {
		return site, true
	}
	return Site{}, false
}

func IsFrontRuntimeAPIPath(requestPath string) bool {
	requestPath = cleanAbsPath(requestPath)
	root := requestRootAfterPrefix(requestPath, cleanAbsPath(DefaultAPI))
	if root == "" {
		return false
	}
	for _, item := range defaultSiteAPIRoots {
		if root == item {
			return true
		}
	}
	return false
}

func FrontRuntimeAPIPath(parts ...string) string {
	values := append([]string{DefaultAPI}, parts...)
	return cleanAbsPath(path.Join(values...))
}

func FrontRuntimeAPIURL(endpoint string, query url.Values) string {
	apiPath := FrontRuntimeAPIPath(endpoint)
	if len(query) == 0 {
		return apiPath
	}
	return apiPath + "?" + query.Encode()
}

func IsFrontRuntimeAPIEndpoint(requestPath, endpoint string) bool {
	return cleanAbsPath(requestPath) == FrontRuntimeAPIPath(endpoint)
}

func (cfg Config) FindByStaticSitePath(requestPath string) (Site, bool) {
	requestPath = cleanAbsPath(requestPath)
	site, ok := cfg.FindBySitePath(requestPath)
	if !ok {
		return Site{}, false
	}
	if site.APIPrefix() == site.Path && isSiteReservedAPIRoot(site, requestPath) {
		return Site{}, false
	}
	return site, true
}

func (cfg Config) FindBySitePath(requestPath string) (Site, bool) {
	requestPath = cleanAbsPath(requestPath)
	return cfg.findByPath(requestPath, func(site Site) string {
		return site.Path
	})
}

func (cfg Config) PageNames() map[string]struct{} {
	names := make(map[string]struct{}, len(cfg.Sites))
	for _, site := range cfg.Sites {
		pageName := cleanRelativePath(site.Page)
		if pageName != "" {
			names[pageName] = struct{}{}
		}
	}
	return names
}

func (cfg Config) AllPublicPaths() []string {
	paths := make([]string, 0, len(cfg.Public)+len(cfg.Sites)*2)
	paths = append(paths, cfg.Public...)
	for _, site := range cfg.Sites {
		prefix := site.APIPrefix()
		for _, item := range site.Public {
			if item == "" {
				continue
			}
			paths = append(paths, cleanAbsPath(path.Join(prefix, item)))
		}
	}
	return uniqueStrings(paths)
}

func (cfg Config) AllSitePaths() []string {
	paths := make([]string, 0, len(cfg.Sites)*3)
	for _, site := range cfg.Sites {
		paths = append(paths, site.Path)
		if site.Path == "/" {
			paths = append(paths, "/*", "/runtime.js")
		} else {
			paths = append(paths, site.Path+"/*", site.Path+"/runtime.js")
		}
	}
	return uniqueStrings(paths)
}

func (cfg Config) IsPublicPath(requestPath string) bool {
	return MatchPublicPath(cfg.AllPublicPaths(), requestPath)
}

func MatchPublicPath(publicPaths []string, requestPath string) bool {
	requestPath = cleanAbsPath(requestPath)
	for _, publicPath := range publicPaths {
		if matchPath(publicPath, requestPath) {
			return true
		}
	}
	return false
}

func (cfg Config) findByPath(requestPath string, prefix func(Site) string) (Site, bool) {
	var matched Site
	matchedLen := -1
	for _, site := range cfg.Sites {
		value := cleanAbsPath(prefix(site))
		if !matchPathPrefix(value, requestPath) {
			continue
		}
		if len(value) > matchedLen {
			matched = site
			matchedLen = len(value)
		}
	}
	return matched, matchedLen >= 0
}

func (site Site) APIPrefix() string {
	return cleanAbsPath(site.API)
}

func isSiteReservedAPIRoot(site Site, requestPath string) bool {
	root := requestRootAfterPrefix(requestPath, site.APIPrefix())
	if root == "" {
		return false
	}
	for _, item := range defaultSiteAPIRoots {
		if root == item {
			return true
		}
	}
	for _, item := range site.APIRoots {
		if root == item {
			return true
		}
	}
	for _, item := range site.Public {
		if root == firstPathSegment(item) {
			return true
		}
	}
	return false
}

func requestRootAfterPrefix(requestPath string, prefix string) string {
	requestPath = cleanAbsPath(requestPath)
	prefix = cleanAbsPath(prefix)
	if !matchPathPrefix(prefix, requestPath) {
		return ""
	}
	rel := strings.Trim(strings.TrimPrefix(requestPath, prefix), "/")
	return firstPathSegment(rel)
}

func firstPathSegment(value string) string {
	value = cleanRelativePath(value)
	if value == "" {
		return ""
	}
	if index := strings.Index(value, "/"); index >= 0 {
		return value[:index]
	}
	return value
}

func (site Site) SystemPagePath(pageName string) string {
	pageName = cleanRelativePath(pageName)
	apiPrefix := strings.Trim(site.APIPrefix(), "/")
	if apiPrefix == "" {
		apiPrefix = DefaultAPI
	}
	if pageName == "" {
		return apiPrefix
	}
	return path.Join(apiPrefix, pageName)
}

func (site Site) AssetURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if isExternalAssetURL(value) || strings.HasPrefix(value, "/") {
		return value
	}
	return cleanAbsPath(path.Join(site.Path, "assets", trimAssetPrefix(value)))
}

func (site Site) LogoURL() string {
	return site.AssetURL(site.Assets.Logo)
}

func (site Site) FaviconURL() string {
	return site.AssetURL(site.Assets.Favicon)
}

func (site Site) UsesRBAC() bool {
	return strings.EqualFold(site.Access.Mode, AccessModeRBAC)
}

func (site Site) UsesLogin() bool {
	return strings.EqualFold(site.Access.Mode, AccessModeLogin)
}

func defaultConfig() Config {
	return normalizeConfig(rawConfig{
		Public: append([]string(nil), defaultGlobalPublic...),
		Sites: map[string]rawSite{
			DefaultSiteKey: {
				Name:   "管理后台",
				API:    DefaultAPI,
				Public: append([]string(nil), defaultSitePublic...),
				Access: Access{
					Mode:         DefaultAccessMode,
					AuthProvider: DefaultAuthProvider,
				},
			},
		},
	})
}

func normalizeConfig(payload rawConfig) Config {
	publicPaths := normalizeGlobalPublic(payload.Public)
	if len(publicPaths) == 0 {
		publicPaths = append([]string(nil), defaultGlobalPublic...)
	}

	rawSites := payload.Sites
	if len(rawSites) == 0 {
		rawSites = map[string]rawSite{
			DefaultSiteKey: {
				Name:   "管理后台",
				API:    DefaultAPI,
				Public: append([]string(nil), defaultSitePublic...),
				Access: Access{
					Mode:         DefaultAccessMode,
					AuthProvider: DefaultAuthProvider,
				},
				Auth:  payload.Auth,
				Entry: payload.Entry,
			},
		}
	} else if len(payload.Auth) > 0 {
		admin := rawSites[DefaultSiteKey]
		if len(admin.Auth) == 0 {
			admin.Auth = payload.Auth
		}
		if strings.TrimSpace(admin.Entry) == "" {
			admin.Entry = payload.Entry
		}
		rawSites[DefaultSiteKey] = admin
	}

	keys := make([]string, 0, len(rawSites))
	for key := range rawSites {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sites := make([]Site, 0, len(keys))
	for _, key := range keys {
		siteKey := normalizeSiteKey(key)
		if siteKey == "" {
			continue
		}
		site := normalizeSite(siteKey, rawSites[key])
		sites = append(sites, site)
	}
	if len(sites) == 0 {
		sites = defaultConfig().Sites
	}

	return Config{
		Public: publicPaths,
		Sites:  sites,
	}
}

func normalizeSite(siteKey string, raw rawSite) Site {
	api := cleanAPI(raw.API)
	if api == "" {
		if siteKey == DefaultSiteKey {
			api = DefaultAPI
		} else {
			api = siteKey
		}
	}
	if api == "" {
		api = DefaultAPI
	}

	return Site{
		Key:         siteKey,
		Path:        cleanSitePath(siteKey),
		Page:        cleanPage(raw.Page, siteKey),
		Name:        strings.TrimSpace(raw.Name),
		Subtitle:    strings.TrimSpace(raw.Subtitle),
		Description: strings.TrimSpace(raw.Description),
		URL:         strings.TrimSpace(raw.URL),
		API:         api,
		APIRoots:    normalizeAPIRoots(raw.APIRoots),
		Public:      normalizeSitePublic(raw.Public),
		Assets:      normalizeAssets(raw.Assets),
		Setting:     normalizeSetting(raw.Setting),
		Access:      normalizeAccess(raw.Access),
		Auth:        raw.Auth,
		Entry:       strings.TrimSpace(raw.Entry),
	}
}

func normalizeAssets(assets SiteAssets) SiteAssets {
	return SiteAssets{
		Logo:    cleanAssetValue(assets.Logo),
		Favicon: cleanAssetValue(assets.Favicon),
	}
}

func normalizeSetting(setting SiteSetting) SiteSetting {
	return SiteSetting{
		Appearance: AppearanceSetting{
			Theme:     strings.TrimSpace(setting.Appearance.Theme),
			Sidebar:   strings.TrimSpace(setting.Appearance.Sidebar),
			Layout:    strings.TrimSpace(setting.Appearance.Layout),
			Direction: strings.TrimSpace(setting.Appearance.Direction),
		},
		Runtime: RuntimeSetting{
			Skin:       strings.TrimSpace(setting.Runtime.Skin),
			RouterMode: strings.TrimSpace(setting.Runtime.RouterMode),
			Plugins:    normalizeStringList(setting.Runtime.Plugins),
		},
	}
}

func normalizeAccess(access Access) Access {
	mode := strings.ToLower(strings.TrimSpace(access.Mode))
	if mode == "" {
		mode = DefaultAccessMode
	}
	provider := strings.Trim(strings.TrimSpace(access.AuthProvider), "/")
	if provider == "" {
		provider = DefaultAuthProvider
	}
	return Access{
		Mode:         mode,
		AuthProvider: provider,
	}
}

func normalizeGlobalPublic(items []string) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		item = cleanAbsPath(item)
		if item == "" {
			continue
		}
		paths = append(paths, item)
	}
	return uniqueStrings(paths)
}

func normalizeSitePublic(items []string) []string {
	if len(items) == 0 {
		items = defaultSitePublic
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		item = cleanRelativePath(item)
		if item == "" {
			continue
		}
		paths = append(paths, item)
	}
	return uniqueStrings(paths)
}

func normalizeAPIRoots(items []string) []string {
	roots := make([]string, 0, len(items))
	for _, item := range items {
		root := firstPathSegment(item)
		if root == "" {
			continue
		}
		roots = append(roots, root)
	}
	return uniqueStrings(roots)
}

func normalizeSiteKey(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func cleanSitePath(siteKey string) string {
	return cleanAbsPath("/" + siteKey)
}

func cleanAPI(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func cleanPage(value string, siteKey string) string {
	value = cleanRelativePath(value)
	if value == "" {
		value = siteKey
	}
	if value == "" {
		return DefaultPage
	}
	return value
}

func cleanAssetValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || isExternalAssetURL(value) || strings.HasPrefix(value, "/") {
		return value
	}
	return cleanRelativePath(value)
}

func trimAssetPrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	return strings.TrimPrefix(value, "assets/")
}

func isExternalAssetURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:") ||
		strings.HasPrefix(value, "blob:")
}

func cleanAbsPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = "/" + strings.Trim(value, "/")
	return path.Clean(value)
}

func cleanRelativePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	if value == "" || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return ""
	}
	value = path.Clean(value)
	if value == "." {
		return ""
	}
	return value
}

func matchPath(pattern, requestPath string) bool {
	pattern = cleanAbsPath(pattern)
	requestPath = cleanAbsPath(requestPath)
	if pattern == "" || requestPath == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
	}
	return requestPath == pattern
}

func matchPathPrefix(prefix, requestPath string) bool {
	prefix = cleanAbsPath(prefix)
	requestPath = cleanAbsPath(requestPath)
	if prefix == "" || requestPath == "" {
		return false
	}
	if prefix == "/" {
		return true
	}
	return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func normalizeStringList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	return uniqueStrings(items)
}
