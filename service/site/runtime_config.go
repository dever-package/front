package site

import (
	"context"
	"strings"
	"sync"

	"github.com/dever-package/front/service/siteconfig"
)

// RuntimeSiteConfig contains display-only values that may override a site's
// static configuration before the initial HTML is sent to the browser.
type RuntimeSiteConfig struct {
	Name        string
	Subtitle    string
	Description string
	Logo        string
	Favicon     string
}

type RuntimeSiteConfigProvider func(context.Context, siteconfig.Site) RuntimeSiteConfig

var runtimeSiteConfigProviders struct {
	sync.RWMutex
	items map[string]RuntimeSiteConfigProvider
}

// RegisterRuntimeSiteConfigProvider registers a display configuration source
// for one site. Routing and access settings remain owned by the static site
// configuration.
func RegisterRuntimeSiteConfigProvider(siteKey string, provider RuntimeSiteConfigProvider) {
	siteKey = strings.TrimSpace(siteKey)
	if siteKey == "" || provider == nil {
		return
	}

	runtimeSiteConfigProviders.Lock()
	defer runtimeSiteConfigProviders.Unlock()
	if runtimeSiteConfigProviders.items == nil {
		runtimeSiteConfigProviders.items = make(map[string]RuntimeSiteConfigProvider)
	}
	runtimeSiteConfigProviders.items[siteKey] = provider
}

func resolveRuntimeSitePayload(ctx context.Context, site siteconfig.Site) (runtimeSitePayload, bool) {
	payload := runtimeSitePayload{
		Name:        site.Config.Name,
		Subtitle:    site.Config.Subtitle,
		Description: site.Config.Description,
		URL:         site.Config.PrimaryURL(),
		Logo:        site.LogoURL(),
		Favicon:     site.FaviconURL(),
	}
	provider, ok := lookupRuntimeSiteConfigProvider(site.Key)
	if !ok {
		return payload, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	config := provider(ctx, site)
	setRuntimeSiteValue(&payload.Name, config.Name)
	setRuntimeSiteValue(&payload.Subtitle, config.Subtitle)
	setRuntimeSiteValue(&payload.Description, config.Description)
	setRuntimeSiteValue(&payload.Logo, config.Logo)
	setRuntimeSiteValue(&payload.Favicon, config.Favicon)
	return payload, true
}

func hasRuntimeSiteConfigProvider(siteKey string) bool {
	_, ok := lookupRuntimeSiteConfigProvider(siteKey)
	return ok
}

func lookupRuntimeSiteConfigProvider(siteKey string) (RuntimeSiteConfigProvider, bool) {
	runtimeSiteConfigProviders.RLock()
	defer runtimeSiteConfigProviders.RUnlock()
	provider, ok := runtimeSiteConfigProviders.items[strings.TrimSpace(siteKey)]
	return provider, ok && provider != nil
}

func setRuntimeSiteValue(target *string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		*target = value
	}
}
