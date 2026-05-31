package siteconfig

import (
	"os"
	"strings"

	"github.com/shemic/dever/config"
)

var pluginDevProxyRoutes = []string{
	"/@fs/*",
	"/@id/*",
	"/@vite/*",
	"/@vite/client",
	"/@react-refresh",
	"/.vite/*",
	"/vite/*",
	"/node_modules/.vite/*",
	"/package/*",
	"/module/*",
	"/backend/package/*",
	"/backend/module/*",
	"/src/*",
}

var pluginDevProxyPrefixes = []string{
	"/@fs/",
	"/@id/",
	"/@vite/",
	"/.vite/",
	"/vite/",
	"/node_modules/.vite/",
	"/package/",
	"/module/",
	"/backend/package/",
	"/backend/module/",
	"/src/",
}

var pluginDevProxyExact = map[string]struct{}{
	"/@react-refresh": {},
}

var pluginDevViteDepPrefixes = []string{
	"/.vite/deps/",
	"/vite/deps/",
	"/node_modules/.vite/deps/",
}

func PluginDevProxyRoutes() []string {
	return append([]string(nil), pluginDevProxyRoutes...)
}

func PluginDevEnabled(cfg config.FrontSite) bool {
	if value, ok := pluginDevEnvBool("DEVER_FRONT_PLUGIN_DEV"); ok {
		return value
	}
	if cfg.PluginDev.Enabled != nil {
		return *cfg.PluginDev.Enabled
	}
	return false
}

func IsPluginDevProxyPath(requestPath string) bool {
	requestPath = cleanAbsPath(requestPath)
	if requestPath == "" {
		return false
	}
	if _, ok := pluginDevProxyExact[requestPath]; ok {
		return true
	}
	for _, prefix := range pluginDevProxyPrefixes {
		if strings.HasPrefix(requestPath, prefix) {
			return true
		}
	}
	return false
}

func pluginDevEnvBool(name string) (bool, bool) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func IsPluginDevViteDepPath(requestPath string) bool {
	requestPath = cleanAbsPath(requestPath)
	if strings.Contains(requestPath, "/.vite/deps/") {
		return true
	}
	for _, prefix := range pluginDevViteDepPrefixes {
		if strings.HasPrefix(requestPath, prefix) {
			return true
		}
	}
	return false
}
