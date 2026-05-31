package site

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"

	"my/package/front/service/siteconfig"
)

type runtimePayload struct {
	SiteKey    string                       `json:"siteKey"`
	BasePath   string                       `json:"basePath"`
	APIHost    string                       `json:"apiHost"`
	Site       runtimeSitePayload           `json:"site"`
	Appearance siteconfig.AppearanceSetting `json:"appearance,omitempty"`
	Runtime    siteconfig.RuntimeSetting    `json:"runtime,omitempty"`
	Access     siteconfig.Access            `json:"access"`
}

type runtimeSitePayload struct {
	Name        string `json:"name,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Logo        string `json:"logo,omitempty"`
	Favicon     string `json:"favicon,omitempty"`
}

func writeRuntime(c *server.Context, site siteconfig.Site, dev bool) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持 runtime 输出")
	}

	payload := runtimePayload{
		SiteKey:  site.Key,
		BasePath: site.Path,
		APIHost:  runtimeAPIHost(site, dev, raw.Hostname()),
		Site: runtimeSitePayload{
			Name:        site.Name,
			Subtitle:    site.Subtitle,
			Description: site.Description,
			URL:         site.URL,
			Logo:        site.LogoURL(),
			Favicon:     site.FaviconURL(),
		},
		Appearance: site.Setting.Appearance,
		Runtime:    site.Setting.Runtime,
		Access:     site.Access,
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return c.Error(err)
	}

	raw.Set("Cache-Control", "no-cache")
	raw.Set("Content-Type", "application/javascript; charset=utf-8")
	return raw.SendString("window.appRuntime = " + string(content) + ";\n")
}

func runtimeAPIHost(site siteconfig.Site, dev bool, hostname string) string {
	prefix := strings.Trim(site.APIPrefix(), "/")
	if dev {
		host := strings.TrimSpace(hostname)
		if host == "" {
			host = "localhost"
		}
		return "http://" + host + ":8085/" + prefix + "/"
	}
	return "/" + prefix + "/"
}
