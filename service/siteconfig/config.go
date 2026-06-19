package siteconfig

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/shemic/dever/component"
	"github.com/shemic/dever/util"
)

const (
	DefaultSiteKey      = "admin"
	DefaultPage         = "admin"
	DefaultAPI          = "front"
	DefaultAccessMode   = "rbac"
	DefaultAuthProvider = "front"
	DefaultTheme        = "light"
	DefaultSidebar      = "floating"
	DefaultLayout       = "compact"
	DefaultDirection    = "ltr"
	DefaultSkin         = "default"
	DefaultRouterMode   = "history"
	ShellModeApp        = "app"
	ShellModeBlank      = "blank"
	DefaultShell        = ShellModeApp
	AccessModeRBAC      = "rbac"
	AccessModeLogin     = "login"
	AccessModePublic    = "public"
	projectConfigPath   = "config/front.json"
)

var (
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
	Key     string
	Path    string
	Page    string
	API     string
	Public  []string
	Config  SiteConfig
	Setting SiteSetting
	Access  Access
	Auth    []AuthSeed
	Entry   string
}

type SiteConfig struct {
	Name        string `json:"name"`
	Subtitle    string `json:"subtitle"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Logo        string `json:"logo"`
	Favicon     string `json:"favicon"`
}

type projectConfig struct {
	Sites map[string]SiteConfig `json:"sites"`
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
	Shell      string   `json:"shell"`
	Plugins    []string `json:"plugins,omitempty"`
}

type Access struct {
	Mode         string `json:"mode"`
	AuthProvider string `json:"authProvider"`
}

type AuthSeed = component.AuthSeed

type contextKey struct{}

func Load(context.Context) (Config, error) {
	loadOnce.Do(func() {
		loadedConfig, loadedErr = load()
	})
	return loadedConfig, loadedErr
}

func load() (Config, error) {
	cfg, err := loadFromComponents(component.Active())
	if err != nil {
		return Config{}, err
	}
	return applyProjectConfig(cfg)
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
		if site.UsesPublic() && !site.UsesDefaultAPI() {
			paths = append(paths, cleanAbsPath(path.Join(prefix, "*")))
		}
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

func (site Site) UsesDefaultAPI() bool {
	return strings.EqualFold(site.APIPrefix(), cleanAbsPath(DefaultAPI))
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
	return cleanAbsPath(path.Join(site.Path, "assets", value))
}

func (site Site) LogoURL() string {
	return site.AssetURL(site.Config.Logo)
}

func (site Site) FaviconURL() string {
	return site.AssetURL(site.Config.Favicon)
}

func (site Site) UsesRBAC() bool {
	return strings.EqualFold(site.Access.Mode, AccessModeRBAC)
}

func (site Site) UsesLogin() bool {
	return strings.EqualFold(site.Access.Mode, AccessModeLogin)
}

func (site Site) UsesPublic() bool {
	return strings.EqualFold(site.Access.Mode, AccessModePublic)
}

func (site Site) RequiresAuth() bool {
	return !site.UsesPublic()
}

func (site Site) IsPublicRuntimeEndpoint(requestPath string) bool {
	if !site.UsesPublic() {
		return false
	}
	if IsFrontRuntimeAPIEndpoint(requestPath, "main/info") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "main/bootstrap") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "route/info") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "route/data") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "route/batch_info") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "route/option") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "route/batch_option") ||
		IsFrontRuntimeAPIEndpoint(requestPath, "route/action") {
		return true
	}
	return false
}

func loadFromComponents(components []component.Component) (Config, error) {
	if len(components) == 0 {
		return Config{}, fmt.Errorf("front siteconfig: no active components registered; run `dever component` or `dever init --skip-tidy`")
	}

	publicPaths := []string{}
	sites := map[string]Site{}
	owners := map[string]string{}

	for _, current := range components {
		publicPaths = append(publicPaths, current.Manifest.Front.Public...)
		for siteKey, contribution := range current.Manifest.Front.Sites {
			siteKey = normalizeSiteKey(siteKey)
			if siteKey == "" {
				continue
			}
			_, absoluteSitePublic := splitComponentSitePublic(contribution.Public)
			publicPaths = append(publicPaths, absoluteSitePublic...)

			site, ok := sites[siteKey]
			if !ok {
				site = defaultSite(siteKey)
			}
			if err := mergeComponentSite(&site, owners, current, contribution); err != nil {
				return Config{}, err
			}
			sites[siteKey] = site
		}
	}

	if len(sites) == 0 {
		sites[DefaultSiteKey] = defaultSite(DefaultSiteKey)
	}
	for key := range sites {
		if key != DefaultSiteKey && owners[key] == "" {
			delete(sites, key)
		}
	}
	if len(sites) == 0 {
		sites[DefaultSiteKey] = defaultSite(DefaultSiteKey)
	}

	keys := make([]string, 0, len(sites))
	for key := range sites {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	resultSites := make([]Site, 0, len(keys))
	for _, key := range keys {
		resultSites = append(resultSites, normalizeSite(sites[key]))
	}

	return Config{
		Public: normalizeGlobalPublic(publicPaths),
		Sites:  resultSites,
	}, nil
}

func applyProjectConfig(cfg Config) (Config, error) {
	project, err := loadProjectConfig()
	if err != nil {
		return Config{}, err
	}
	if len(project.Sites) == 0 {
		return cfg, nil
	}

	sites := make([]Site, len(cfg.Sites))
	copy(sites, cfg.Sites)
	for siteKey, config := range project.Sites {
		siteKey = normalizeSiteKey(siteKey)
		if siteKey == "" {
			continue
		}
		found := false
		for index := range sites {
			if sites[index].Key != siteKey {
				continue
			}
			sites[index].Config = mergeSiteConfig(sites[index].Config, config)
			found = true
			break
		}
		if !found {
			return Config{}, fmt.Errorf("%s 定义了未知站点 %q", projectConfigPath, siteKey)
		}
	}
	cfg.Sites = sites
	return cfg, nil
}

func loadProjectConfig() (projectConfig, error) {
	content, err := os.ReadFile(projectConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return projectConfig{}, nil
		}
		return projectConfig{}, err
	}
	if err := validateProjectConfig(content); err != nil {
		return projectConfig{}, err
	}
	var config projectConfig
	if err := util.UnmarshalJSONC(content, &config); err != nil {
		return projectConfig{}, err
	}
	config = normalizeProjectConfig(config)
	if err := validateProjectSiteConfigs(config); err != nil {
		return projectConfig{}, err
	}
	return config, nil
}

func validateProjectConfig(content []byte) error {
	var root map[string]map[string]map[string]any
	if err := util.UnmarshalJSONC(content, &root); err != nil {
		return err
	}
	for key := range root {
		if key != "sites" {
			return fmt.Errorf("%s 只允许 sites 字段，不允许 %q", projectConfigPath, key)
		}
	}
	for siteKey, siteConfig := range root["sites"] {
		for key := range siteConfig {
			switch key {
			case "name", "subtitle", "description", "url", "logo", "favicon":
			default:
				return fmt.Errorf("%s sites.%s 只允许 name/subtitle/description/url/logo/favicon，不允许 %q", projectConfigPath, siteKey, key)
			}
		}
	}
	return nil
}

func normalizeProjectConfig(config projectConfig) projectConfig {
	if len(config.Sites) == 0 {
		return projectConfig{}
	}
	sites := make(map[string]SiteConfig, len(config.Sites))
	for key, siteConfig := range config.Sites {
		key = normalizeSiteKey(key)
		if key == "" {
			continue
		}
		sites[key] = normalizeSiteConfig(siteConfig)
	}
	return projectConfig{Sites: sites}
}

func validateProjectSiteConfigs(config projectConfig) error {
	for siteKey, siteConfig := range config.Sites {
		if err := validateSiteConfig(siteConfig, fmt.Sprintf("%s sites.%s", projectConfigPath, siteKey)); err != nil {
			return err
		}
	}
	return nil
}

func mergeSiteConfig(base SiteConfig, override SiteConfig) SiteConfig {
	if override.Name != "" {
		base.Name = override.Name
	}
	if override.Subtitle != "" {
		base.Subtitle = override.Subtitle
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.URL != "" {
		base.URL = override.URL
	}
	if override.Logo != "" {
		base.Logo = override.Logo
	}
	if override.Favicon != "" {
		base.Favicon = override.Favicon
	}
	return normalizeSiteConfig(base)
}

func manifestSiteConfig(config component.ManifestSiteConfig) SiteConfig {
	return SiteConfig{
		Name:        config.Name,
		Subtitle:    config.Subtitle,
		Description: config.Description,
		URL:         config.URL,
		Logo:        config.Logo,
		Favicon:     config.Favicon,
	}
}

func validateSiteConfig(config SiteConfig, source string) error {
	if err := validateAssetRefValue(config.Logo); err != nil {
		return fmt.Errorf("%s.logo 不合法: %w", source, err)
	}
	if err := validateAssetRefValue(config.Favicon); err != nil {
		return fmt.Errorf("%s.favicon 不合法: %w", source, err)
	}
	return nil
}

func validateAssetRefValue(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || isExternalAssetURL(value) || strings.HasPrefix(value, "/") {
		return nil
	}
	value = cleanRelativePath(value)
	if value == "" {
		return fmt.Errorf("资源路径为空或包含非法上级目录")
	}
	if strings.HasPrefix(value, "config/assets/") && cleanRelativePath(strings.TrimPrefix(value, "config/assets/")) != "" {
		return nil
	}
	parts := strings.SplitN(value, "/", 3)
	if len(parts) == 3 && parts[0] != "" && parts[1] == "assets" && cleanRelativePath(parts[2]) != "" {
		return nil
	}
	return fmt.Errorf("必须使用 config/assets/...、<component>/assets/...、绝对路径或外部 URL")
}

func mergeComponentSite(site *Site, owners map[string]string, current component.Component, contribution component.ManifestSite) error {
	if site == nil {
		return nil
	}

	if hasSiteOwnerFields(contribution) {
		if owner := owners[site.Key]; owner != "" && owner != current.Name {
			return fmt.Errorf("front site %q 同时被 %s 和 %s 定义，请改用唯一站点名或只追加 auth/public", site.Key, owner, current.Name)
		}
		if err := validateSiteConfig(manifestSiteConfig(contribution.Config), fmt.Sprintf("组件 %s front.sites.%s.config", current.Name, site.Key)); err != nil {
			return err
		}
		owners[site.Key] = current.Name
		applySiteOwnerFields(site, contribution)
	}

	sitePublic, _ := splitComponentSitePublic(contribution.Public)
	site.Auth = mergeAuthSeeds(site.Auth, contribution.Auth)
	site.Public = mergeSitePublic(site.Public, sitePublic)
	return nil
}

func mergeAuthSeeds(base []AuthSeed, additions []AuthSeed) []AuthSeed {
	if len(additions) == 0 {
		return base
	}
	result := make([]AuthSeed, 0, len(base)+len(additions))
	indexes := map[string]int{}
	for _, item := range append(base, additions...) {
		key := strings.TrimSpace(util.FirstNonEmpty(item.Key, item.ID))
		if key == "" {
			continue
		}
		if index, ok := indexes[key]; ok {
			result[index] = item
			continue
		}
		indexes[key] = len(result)
		result = append(result, item)
	}
	return result
}

func mergeSitePublic(base []string, additions []string) []string {
	if len(additions) == 0 {
		return base
	}
	items := append(append([]string(nil), base...), additions...)
	return normalizeSitePublic(items)
}

func splitComponentSitePublic(items []string) ([]string, []string) {
	if len(items) == 0 {
		return nil, nil
	}
	sitePublic := make([]string, 0, len(items))
	globalPublic := make([]string, 0)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "/") {
			globalPublic = append(globalPublic, item)
			continue
		}
		sitePublic = append(sitePublic, item)
	}
	return sitePublic, globalPublic
}

func defaultSite(siteKey string) Site {
	return Site{
		Key:    normalizeSiteKey(siteKey),
		Path:   cleanSitePath(siteKey),
		Page:   cleanPage("", siteKey),
		API:    defaultAPIForSite(siteKey),
		Public: []string{},
		Access: Access{
			Mode:         DefaultAccessMode,
			AuthProvider: DefaultAuthProvider,
		},
	}
}

func defaultAPIForSite(siteKey string) string {
	siteKey = normalizeSiteKey(siteKey)
	if siteKey == "" || siteKey == DefaultSiteKey {
		return DefaultAPI
	}
	return siteKey
}

func hasSiteOwnerFields(site component.ManifestSite) bool {
	return strings.TrimSpace(site.Page) != "" ||
		strings.TrimSpace(site.API) != "" ||
		strings.TrimSpace(site.Entry) != "" ||
		hasManifestSiteConfig(site.Config) ||
		hasSiteSetting(site.Setting) ||
		hasSiteAccess(site.Access)
}

func hasManifestSiteConfig(config component.ManifestSiteConfig) bool {
	return strings.TrimSpace(config.Name) != "" ||
		strings.TrimSpace(config.Subtitle) != "" ||
		strings.TrimSpace(config.Description) != "" ||
		strings.TrimSpace(config.URL) != "" ||
		strings.TrimSpace(config.Logo) != "" ||
		strings.TrimSpace(config.Favicon) != ""
}

func hasSiteSetting(setting component.ManifestSiteSetting) bool {
	return strings.TrimSpace(setting.Appearance.Theme) != "" ||
		strings.TrimSpace(setting.Appearance.Sidebar) != "" ||
		strings.TrimSpace(setting.Appearance.Layout) != "" ||
		strings.TrimSpace(setting.Appearance.Direction) != "" ||
		strings.TrimSpace(setting.Runtime.Skin) != "" ||
		strings.TrimSpace(setting.Runtime.RouterMode) != "" ||
		strings.TrimSpace(setting.Runtime.Shell) != "" ||
		len(setting.Runtime.Plugins) > 0
}

func hasSiteAccess(access component.ManifestSiteAccess) bool {
	return strings.TrimSpace(access.Mode) != "" ||
		strings.TrimSpace(access.AuthProvider) != ""
}

func applySiteOwnerFields(site *Site, contribution component.ManifestSite) {
	site.Page = cleanPage(contribution.Page, site.Key)
	site.API = cleanAPI(contribution.API)
	if site.API == "" {
		site.API = defaultAPIForSite(site.Key)
	}
	site.Config = normalizeSiteConfig(SiteConfig{
		Name:        contribution.Config.Name,
		Subtitle:    contribution.Config.Subtitle,
		Description: contribution.Config.Description,
		URL:         contribution.Config.URL,
		Logo:        contribution.Config.Logo,
		Favicon:     contribution.Config.Favicon,
	})
	site.Access = normalizeAccess(Access{
		Mode:         contribution.Access.Mode,
		AuthProvider: contribution.Access.AuthProvider,
	})
	site.Setting = normalizeSetting(SiteSetting{
		Appearance: AppearanceSetting{
			Theme:     contribution.Setting.Appearance.Theme,
			Sidebar:   contribution.Setting.Appearance.Sidebar,
			Layout:    contribution.Setting.Appearance.Layout,
			Direction: contribution.Setting.Appearance.Direction,
		},
		Runtime: RuntimeSetting{
			Skin:       contribution.Setting.Runtime.Skin,
			RouterMode: contribution.Setting.Runtime.RouterMode,
			Shell:      contribution.Setting.Runtime.Shell,
			Plugins:    contribution.Setting.Runtime.Plugins,
		},
	}, defaultShellForSite(*site))
	site.Entry = strings.Trim(strings.TrimSpace(contribution.Entry), "/")
}

func normalizeSite(site Site) Site {
	site.Key = normalizeSiteKey(site.Key)
	site.Path = cleanSitePath(site.Key)
	site.Page = cleanPage(site.Page, site.Key)
	site.API = cleanAPI(site.API)
	if site.API == "" {
		site.API = defaultAPIForSite(site.Key)
	}
	site.Public = normalizeSitePublic(site.Public)
	site.Config = normalizeSiteConfig(site.Config)
	site.Access = normalizeAccess(site.Access)
	site.Setting = normalizeSetting(site.Setting, defaultShellForSite(site))
	site.Entry = strings.Trim(strings.TrimSpace(site.Entry), "/")
	return site
}

func normalizeSiteConfig(config SiteConfig) SiteConfig {
	return SiteConfig{
		Name:        strings.TrimSpace(config.Name),
		Subtitle:    strings.TrimSpace(config.Subtitle),
		Description: strings.TrimSpace(config.Description),
		URL:         strings.TrimSpace(config.URL),
		Logo:        cleanAssetValue(config.Logo),
		Favicon:     cleanAssetValue(config.Favicon),
	}
}

func normalizeSetting(setting SiteSetting, defaultShell string) SiteSetting {
	theme := strings.TrimSpace(setting.Appearance.Theme)
	if theme == "" {
		theme = DefaultTheme
	}
	sidebar := strings.TrimSpace(setting.Appearance.Sidebar)
	if sidebar == "" {
		sidebar = DefaultSidebar
	}
	layout := strings.TrimSpace(setting.Appearance.Layout)
	if layout == "" {
		layout = DefaultLayout
	}
	direction := strings.TrimSpace(setting.Appearance.Direction)
	if direction == "" {
		direction = DefaultDirection
	}
	skin := strings.TrimSpace(setting.Runtime.Skin)
	if skin == "" {
		skin = DefaultSkin
	}
	routerMode := strings.TrimSpace(setting.Runtime.RouterMode)
	if routerMode == "" {
		routerMode = DefaultRouterMode
	}
	shell := normalizeShell(setting.Runtime.Shell, defaultShell)
	return SiteSetting{
		Appearance: AppearanceSetting{
			Theme:     theme,
			Sidebar:   sidebar,
			Layout:    layout,
			Direction: direction,
		},
		Runtime: RuntimeSetting{
			Skin:       skin,
			RouterMode: routerMode,
			Shell:      shell,
			Plugins:    normalizeStringList(setting.Runtime.Plugins),
		},
	}
}

func defaultShellForSite(site Site) string {
	if strings.EqualFold(site.Key, DefaultSiteKey) || site.UsesRBAC() {
		return ShellModeApp
	}
	return ShellModeBlank
}

func normalizeShell(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ShellModeApp:
		return ShellModeApp
	case ShellModeBlank:
		return ShellModeBlank
	}
	switch strings.ToLower(strings.TrimSpace(fallback)) {
	case ShellModeApp:
		return ShellModeApp
	case ShellModeBlank:
		return ShellModeBlank
	default:
		return DefaultShell
	}
}

func normalizeAccess(access Access) Access {
	mode := strings.ToLower(strings.TrimSpace(access.Mode))
	switch mode {
	case AccessModeRBAC, AccessModeLogin, AccessModePublic:
	default:
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
