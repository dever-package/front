package siteconfig

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shemic/dever/config"
)

const (
	pluginDevNamesEnv   = "DEVER_FRONT_PLUGIN_DEV_NAMES"
	pluginDevVersionEnv = "DEVER_FRONT_PLUGIN_DEV_VERSION"
)

var pluginDevProcessVersion = strconv.FormatInt(time.Now().UnixNano(), 36)

var pluginDevProxyRoutes = []string{
	"/@fs/*",
	"/@id/*",
	"/@vite/*",
	"/@vite/client",
	"/@react-refresh",
	"/.vite/*",
	"/vite/*",
	"/node_modules/.pnpm/*",
	"/node_modules/.vite/*",
	"/tmp/dever/compiler/front/*",
	"/package/*",
	"/module/*",
	"/backend/package/*",
	"/backend/module/*",
	"/src/*",
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

func PluginDevSourceNames() ([]string, bool) {
	value, explicit := os.LookupEnv(pluginDevNamesEnv)
	if !explicit {
		return nil, false
	}

	seen := make(map[string]struct{})
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			seen[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, true
}

func PluginDevSourceAllowed(name string) bool {
	names, explicit := PluginDevSourceNames()
	if !explicit {
		return true
	}
	name = strings.TrimSpace(name)
	for _, allowed := range names {
		if name == allowed {
			return true
		}
	}
	return false
}

func PluginDevVersion() string {
	if version := strings.TrimSpace(os.Getenv(pluginDevVersionEnv)); version != "" {
		return version
	}
	return pluginDevProcessVersion
}

func IsPluginDevProxyPath(requestPath string) bool {
	requestPath = cleanAbsPath(requestPath)
	if requestPath == "" {
		return false
	}
	for _, route := range pluginDevProxyRoutes {
		if matchPluginDevRoute(route, requestPath) {
			return true
		}
	}
	return false
}

func matchPluginDevRoute(route string, requestPath string) bool {
	route = cleanAbsPath(route)
	if route == "" || requestPath == "" {
		return false
	}
	if strings.HasSuffix(route, "/*") {
		prefix := strings.TrimSuffix(route, "/*")
		return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
	}
	return requestPath == route
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
