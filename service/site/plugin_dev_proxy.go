package site

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"

	"my/package/front/service/siteconfig"
)

const pluginDevProxyTimeout = 30 * time.Second

var pluginDevProxyClient = &http.Client{Timeout: pluginDevProxyTimeout}

func registerPluginDevProxy(s server.Server, siteSettings settings) {
	if !siteSettings.pluginDev || siteSettings.pluginDevURL == "" {
		return
	}

	proxy := func(c *server.Context) error {
		return proxyPluginDevRequest(c, siteSettings.pluginDevURL)
	}

	for _, route := range siteconfig.PluginDevProxyRoutes() {
		s.Get(route, proxy)
	}
}

func proxyPluginDevRequest(c *server.Context, baseURL string) error {
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持前端插件开发代理")
	}

	served, err := openPluginDevSharedModule(raw)
	if err != nil {
		return c.Error(err, http.StatusBadGateway)
	}
	if served {
		return nil
	}

	target := strings.TrimRight(baseURL, "/") + raw.OriginalURL()
	req, err := http.NewRequestWithContext(raw.UserContext(), http.MethodGet, target, nil)
	if err != nil {
		return c.Error(err, http.StatusBadGateway)
	}

	resp, err := pluginDevProxyClient.Do(req)
	if err != nil {
		return c.Error(fmt.Errorf("前端插件源码编译服务不可用，请重启 dever run: %w", err), http.StatusBadGateway)
	}
	defer resp.Body.Close()

	copyProxyHeaders(raw, resp.Header)
	raw.Status(resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Error(err, http.StatusBadGateway)
	}
	return raw.Send(body)
}

func openPluginDevSharedModule(raw *fiber.Ctx) (bool, error) {
	switch raw.Path() {
	case "/@vite/client":
		return true, sendPluginDevModule(raw, viteClientNoopModule())
	case "/@react-refresh":
		return true, sendPluginDevModule(raw, reactRefreshNoopModule())
	}

	if !siteconfig.IsPluginDevViteDepPath(raw.Path()) {
		return false, nil
	}

	switch filepath.Base(raw.Path()) {
	case "react.js":
		return true, sendPluginDevModule(raw, reactGlobalModule())
	case "react_jsx-runtime.js", "react_jsx-dev-runtime.js":
		return true, sendPluginDevModule(raw, reactJSXRuntimeGlobalModule())
	case "react-dom.js":
		return true, sendPluginDevModule(raw, reactDOMGlobalModule())
	case "react-dom_client.js":
		return true, sendPluginDevModule(raw, reactDOMClientGlobalModule())
	default:
		return false, nil
	}
}

func sendPluginDevModule(raw *fiber.Ctx, content string) error {
	raw.Set("Cache-Control", "no-cache")
	raw.Set("Content-Type", "application/javascript; charset=utf-8")
	return raw.SendString(content)
}

func viteClientNoopModule() string {
	return `
const styles = new Map();

export function createHotContext() {
  return {
    data: {},
    accept() {},
    dispose() {},
    prune() {},
    decline() {},
    invalidate() {},
    on() {},
    off() {},
    send() {},
  };
}

export function updateStyle(id, content) {
  if (typeof document === "undefined") {
    return;
  }
  let style = styles.get(id);
  if (!style) {
    style = document.createElement("style");
    style.setAttribute("type", "text/css");
    style.setAttribute("data-vite-dev-id", id);
    document.head.appendChild(style);
    styles.set(id, style);
  }
  style.textContent = content;
}

export function removeStyle(id) {
  const style = styles.get(id);
  if (style) {
    style.remove();
    styles.delete(id);
  }
}

export function injectQuery(url) {
  return url;
}

export class ErrorOverlay extends HTMLElement {}

export default {};
`
}

func reactRefreshNoopModule() string {
	return `
const runtime = {
  injectIntoGlobalHook() {},
  register() {},
  createSignatureFunctionForTransform() {
    return (type) => type;
  },
  isLikelyComponentType() {
    return false;
  },
  performReactRefresh() {},
};
export default runtime;
export const injectIntoGlobalHook = runtime.injectIntoGlobalHook;
export const register = runtime.register;
export const createSignatureFunctionForTransform = runtime.createSignatureFunctionForTransform;
export const isLikelyComponentType = runtime.isLikelyComponentType;
export const performReactRefresh = runtime.performReactRefresh;
`
}

func reactGlobalModule() string {
	names := []string{
		"Children",
		"Component",
		"Fragment",
		"Profiler",
		"PureComponent",
		"StrictMode",
		"Suspense",
		"cloneElement",
		"createContext",
		"createElement",
		"createRef",
		"forwardRef",
		"isValidElement",
		"lazy",
		"memo",
		"startTransition",
		"use",
		"useCallback",
		"useContext",
		"useDebugValue",
		"useDeferredValue",
		"useEffect",
		"useId",
		"useImperativeHandle",
		"useInsertionEffect",
		"useLayoutEffect",
		"useMemo",
		"useOptimistic",
		"useReducer",
		"useRef",
		"useState",
		"useSyncExternalStore",
		"useTransition",
	}

	lines := []string{"const React = window.React;"}
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("export const %s = React.%s;", name, name))
	}
	lines = append(lines, "export default React;")
	return strings.Join(lines, "\n") + "\n"
}

func reactJSXRuntimeGlobalModule() string {
	return `
const React = window.React;
export const Fragment = React.Fragment;
function withKey(props, key) {
  if (key === undefined || key === null) return props || {};
  return Object.assign({}, props || {}, { key });
}
export function jsx(type, props, key) {
  return React.createElement(type, withKey(props, key));
}
export const jsxs = jsx;
export function jsxDEV(type, props, key) {
  return React.createElement(type, withKey(props, key));
}
export default { Fragment, jsx, jsxs, jsxDEV };
`
}

func reactDOMGlobalModule() string {
	return `
const ReactDOM = window.ReactDOM || {};
export const createPortal = ReactDOM.createPortal;
export const flushSync = ReactDOM.flushSync;
export const preconnect = ReactDOM.preconnect;
export const prefetchDNS = ReactDOM.prefetchDNS;
export const preinit = ReactDOM.preinit;
export const preinitModule = ReactDOM.preinitModule;
export const preload = ReactDOM.preload;
export const preloadModule = ReactDOM.preloadModule;
export const requestFormReset = ReactDOM.requestFormReset;
export const unstable_batchedUpdates = ReactDOM.unstable_batchedUpdates;
export const useFormState = ReactDOM.useFormState;
export const useFormStatus = ReactDOM.useFormStatus;
export const version = ReactDOM.version;
export default ReactDOM;
`
}

func reactDOMClientGlobalModule() string {
	return `
const ReactDOMClient = window.ReactDOMClient || {};
export const createRoot = ReactDOMClient.createRoot;
export const hydrateRoot = ReactDOMClient.hydrateRoot;
export const version = ReactDOMClient.version;
export default ReactDOMClient;
`
}

func copyProxyHeaders(raw *fiber.Ctx, headers http.Header) {
	for key, values := range headers {
		if isHopByHopHeader(key) {
			continue
		}
		raw.Set(key, strings.Join(values, ", "))
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
