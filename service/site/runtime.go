package site

import (
	"bytes"
	"encoding/json"
	"hash/crc32"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	"github.com/dever-package/front/service/runtimecache"
	"github.com/dever-package/front/service/siteconfig"
)

const runtimeHTMLPlaceholder = "<!-- dever:runtime -->"

type runtimePayload struct {
	SiteKey     string                       `json:"siteKey"`
	BasePath    string                       `json:"basePath"`
	PagePrefix  string                       `json:"pagePrefix"`
	APIPrefix   string                       `json:"apiPrefix"`
	APIHost     string                       `json:"apiHost"`
	SiteAPIHost string                       `json:"siteApiHost,omitempty"`
	Site        runtimeSitePayload           `json:"site"`
	Appearance  siteconfig.AppearanceSetting `json:"appearance,omitempty"`
	Runtime     runtimeSettingPayload        `json:"runtime,omitempty"`
	Access      siteconfig.Access            `json:"access"`
}

type runtimeSitePayload struct {
	Name        string `json:"name,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Logo        string `json:"logo,omitempty"`
	Favicon     string `json:"favicon,omitempty"`
}

type runtimeSettingPayload struct {
	Skin       string                    `json:"skin,omitempty"`
	RouterMode string                    `json:"routerMode,omitempty"`
	Shell      string                    `json:"shell,omitempty"`
	Plugins    []runtimePluginDescriptor `json:"plugins,omitempty"`
}

var (
	runtimeContentCache util.ConcurrentMap[string, []byte]
	runtimeHTMLCache    util.ConcurrentMap[string, []byte]
)

func init() {
	runtimecache.Register("front.site-runtime", clearRuntimeCaches, clearRuntimeCaches)
}

func clearRuntimeCaches() {
	runtimeContentCache.Clear()
	runtimeHTMLCache.Clear()
}

func writeRuntime(c *server.Context, site siteconfig.Site, pluginDev bool) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持 runtime 输出")
	}

	content, err := runtimeContent(site, pluginDev)
	if err != nil {
		return c.Error(err)
	}

	raw.Set("Cache-Control", "no-cache")
	raw.Set("Content-Type", "application/javascript; charset=utf-8")
	return raw.Send(content)
}

func runtimeContent(site siteconfig.Site, pluginDev bool) ([]byte, error) {
	if pluginDev {
		return buildRuntimeContent(site, pluginDev)
	}

	cacheKey := runtimeContentCacheKey(site, pluginDev)
	if cached, ok := runtimeContentCache.Load(cacheKey); ok {
		return cloneBytes(cached), nil
	}

	content, err := buildRuntimeContent(site, pluginDev)
	if err != nil {
		return nil, err
	}
	runtimeContentCache.Store(cacheKey, cloneBytes(content))
	return content, nil
}

func buildRuntimeContent(site siteconfig.Site, pluginDev bool) ([]byte, error) {
	runtimeSetting := runtimeSettingPayload{
		Skin:       site.Setting.Runtime.Skin,
		RouterMode: site.Setting.Runtime.RouterMode,
		Shell:      site.Setting.Runtime.Shell,
		Plugins:    runtimePluginDescriptors(site, pluginDev),
	}

	payload := runtimePayload{
		SiteKey:     site.Key,
		BasePath:    site.Path,
		PagePrefix:  site.PageRoutePrefix(),
		APIPrefix:   strings.Trim(site.APIPrefix(), "/"),
		APIHost:     runtimeAPIHost(siteconfig.DefaultAPI),
		SiteAPIHost: runtimeAPIHost(strings.Trim(site.APIPrefix(), "/")),
		Site: runtimeSitePayload{
			Name:        site.Config.Name,
			Subtitle:    site.Config.Subtitle,
			Description: site.Config.Description,
			URL:         site.Config.URL,
			Logo:        site.LogoURL(),
			Favicon:     site.FaviconURL(),
		},
		Appearance: site.Setting.Appearance,
		Runtime:    runtimeSetting,
		Access:     site.Access,
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return []byte("window.appRuntime = " + string(content) + ";\n"), nil
}

func injectRuntime(content []byte, site siteconfig.Site, pluginDev bool) ([]byte, error) {
	if pluginDev {
		return injectRuntimeUncached(content, site, pluginDev)
	}

	cacheKey := runtimeHTMLCacheKey(content, site, pluginDev)
	if cached, ok := runtimeHTMLCache.Load(cacheKey); ok {
		return cloneBytes(cached), nil
	}

	result, err := injectRuntimeUncached(content, site, pluginDev)
	if err != nil {
		return nil, err
	}
	runtimeHTMLCache.Store(cacheKey, cloneBytes(result))
	return result, nil
}

func injectRuntimeUncached(content []byte, site siteconfig.Site, pluginDev bool) ([]byte, error) {
	runtime, err := runtimeContent(site, pluginDev)
	if err != nil {
		return nil, err
	}

	script := []byte("<script>\n" + string(runtime) + "</script>")
	placeholder := []byte(runtimeHTMLPlaceholder)
	if bytes.Contains(content, placeholder) {
		return bytes.Replace(content, placeholder, script, 1), nil
	}

	headEnd := []byte("</head>")
	if bytes.Contains(content, headEnd) {
		return bytes.Replace(content, headEnd, append(script, []byte("\n  </head>")...), 1), nil
	}

	return append(script, content...), nil
}

func runtimeContentCacheKey(site siteconfig.Site, pluginDev bool) string {
	return site.Key + ":" + strconv.FormatBool(pluginDev)
}

func runtimeHTMLCacheKey(content []byte, site siteconfig.Site, pluginDev bool) string {
	return site.Key + ":" +
		strconv.FormatBool(pluginDev) + ":" +
		strconv.Itoa(len(content)) + ":" +
		strconv.FormatUint(uint64(crc32.ChecksumIEEE(content)), 16)
}

func cloneBytes(content []byte) []byte {
	if len(content) == 0 {
		return nil
	}
	return append([]byte(nil), content...)
}

func runtimeAPIHost(prefix string) string {
	prefix = strings.Trim(prefix, "/")
	return "/" + prefix + "/"
}
