package render

import (
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontpage "github.com/dever-package/front/service/page"
	"github.com/dever-package/front/service/siteconfig"
)

func resolveSEO(
	raw any,
	c *server.Context,
	site siteconfig.Site,
	routeValues map[string]any,
	query map[string]any,
	siteData map[string]any,
	data map[string]any,
) (SEO, error) {
	resolved, _ := frontpage.ResolveTemplateValue(raw, frontpage.TemplateContext{
		Context: c.Context(),
		Data:    data,
		Route:   routeValues,
		Query:   query,
		Site:    siteData,
	}).(map[string]any)
	if resolved == nil {
		resolved = map[string]any{}
	}

	title := util.ToStringTrimmed(resolved["title"])
	if title == "" {
		title = site.Config.Name
	}
	description := util.ToStringTrimmed(resolved["description"])
	if description == "" {
		description = site.Config.Description
	}
	canonical := util.ToStringTrimmed(resolved["canonical"])
	if canonical != "" && !strings.HasPrefix(canonical, "http://") && !strings.HasPrefix(canonical, "https://") {
		canonical = cleanAbsPath(canonical)
	}

	return SEO{
		Title:       title,
		Description: description,
		Image:       util.ToStringTrimmed(resolved["image"]),
		Canonical:   canonical,
	}, nil
}
