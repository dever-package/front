package render

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	devercache "github.com/shemic/dever/cache"
	"github.com/shemic/dever/component"
	dlog "github.com/shemic/dever/log"
	"github.com/shemic/dever/server"

	"github.com/dever-package/front/service/runtimecache"
	"github.com/dever-package/front/service/siteconfig"
)

var (
	errNotFound = errors.New("render: not found")

	templateCache = devercache.New[string, templateExecutor](
		devercache.WithTTL(5*time.Minute),
		devercache.WithMaxEntries(256),
	)
)

func init() {
	runtimecache.Register("front.render.template", clearTemplateCache, clearTemplateCache)
}

func clearTemplateCache() {
	templateCache.Clear()
}

func Render(c *server.Context, site siteconfig.Site, route TemplateRoute) error {
	routeValues := routeParams(c, route)
	data, seo, err := resolveRenderData(c, site, route, routeValues)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return c.Error("页面不存在", http.StatusNotFound)
		}
		logRenderError("front.render.data_failed", site, route, err)
		return c.Error(err, http.StatusInternalServerError)
	}

	executor, err := loadTemplateExecutor(route.PageName, route.Config)
	if err != nil {
		logRenderError("front.render.template_load_failed", site, route, err)
		return c.Error(err, http.StatusInternalServerError)
	}

	ctx := RenderContext{
		Page:  route.Page,
		Data:  data,
		SEO:   seo,
		Route: routeValues,
		Query: requestQuery(c),
		Site: SiteContext{
			Key:       site.Key,
			Path:      site.Path,
			Page:      site.Page,
			API:       site.APIPrefix(),
			Name:      site.Config.Name,
			AssetBase: assetBase(site.Path),
		},
	}

	var output bytes.Buffer
	if err := executor.tpl.ExecuteTemplate(&output, executor.view, ctx); err != nil {
		logRenderError("front.render.template_execute_failed", site, route, err)
		return c.Error(err, http.StatusInternalServerError)
	}

	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持模板输出", http.StatusInternalServerError)
	}
	raw.Set("Content-Type", "text/html; charset=utf-8")
	return raw.Send(output.Bytes())
}

func logRenderError(event string, site siteconfig.Site, route TemplateRoute, err error) {
	if err == nil {
		return
	}
	dlog.ErrorFields(event, "模板页面渲染失败", dlog.Fields{
		"site":  site.Key,
		"api":   site.APIPrefix(),
		"page":  site.Page,
		"path":  route.Path,
		"route": route.Route,
		"error": dlog.ErrorValue(err),
	})
}

func loadTemplateExecutor(pageName string, config templateConfig) (templateExecutor, error) {
	cacheKey := pageName + ":" + strings.TrimSpace(config.Layout) + ":" + strings.TrimSpace(config.View)
	return templateCache.GetOrSet(cacheKey, func() (templateExecutor, error) {
		return buildTemplateExecutor(pageName, config)
	})
}

func buildTemplateExecutor(pageName string, config templateConfig) (templateExecutor, error) {
	viewName := cleanRelativePath(config.View)
	if viewName == "" {
		return templateExecutor{}, fmt.Errorf("template.view 不能为空")
	}
	layoutName := cleanRelativePath(config.Layout)

	tpl := template.New(viewName).Funcs(templateFuncs())

	files := []string{viewName}
	if layoutName != "" && layoutName != viewName {
		files = append(files, layoutName)
	}
	files = append(files, templatePartials(pageName)...)

	for _, fileName := range uniqueTemplateFiles(files) {
		content, err := readComponentTemplate(pageName, fileName)
		if err != nil {
			return templateExecutor{}, err
		}
		if err := parseTemplateFile(tpl, fileName, content); err != nil {
			return templateExecutor{}, fmt.Errorf("模板解析失败 %s: %w", fileName, err)
		}
	}

	executeName := viewName
	if layoutName != "" {
		executeName = layoutName
	}
	return templateExecutor{
		view:   executeName,
		layout: layoutName,
		tpl:    tpl,
	}, nil
}

func parseTemplateFile(tpl *template.Template, fileName string, content []byte) error {
	name := cleanRelativePath(fileName)
	if name == "" {
		return fmt.Errorf("模板路径不合法")
	}

	source := string(content)
	if hasTemplateDefinition(source) {
		_, err := tpl.Parse(source)
		return err
	}

	wrapped := "{{ define " + strconv.Quote(name) + " }}\n" + source + "\n{{ end }}"
	_, err := tpl.Parse(wrapped)
	return err
}

func hasTemplateDefinition(source string) bool {
	rest := source
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			return false
		}
		rest = rest[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return false
		}

		action := strings.TrimSpace(rest[:end])
		action = strings.TrimPrefix(action, "-")
		action = strings.TrimSpace(action)
		action = strings.TrimSuffix(action, "-")
		action = strings.TrimSpace(action)
		if action == "define" || strings.HasPrefix(action, "define ") {
			return true
		}

		rest = rest[end+2:]
	}
}

func readComponentTemplate(pageName, fileName string) ([]byte, error) {
	if cleanRelativePath(fileName) == "" || cleanRelativePath(pageName) == "" {
		return nil, fmt.Errorf("模板路径不合法")
	}
	for _, current := range component.Active() {
		if current.PageFS == nil {
			continue
		}
		fullPath := componentTemplatePath(current, pageName, fileName)
		content, err := fs.ReadFile(current.PageFS, fullPath)
		if err == nil {
			return content, nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("模板不存在: %s", fileName)
}

func templatePartials(pageName string) []string {
	if cleanRelativePath(pageName) == "" {
		return nil
	}
	filesByPath := map[string]struct{}{}
	for _, current := range component.Active() {
		if current.PageFS == nil {
			continue
		}
		root := componentTemplatePath(current, pageName, "partials")
		trimPrefix := strings.TrimSuffix(componentTemplatePath(current, pageName, ""), "/") + "/"
		_ = fs.WalkDir(current.PageFS, root, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry == nil || entry.IsDir() {
				return nil
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".html" {
				return nil
			}
			rel := strings.TrimPrefix(filepath.ToSlash(filePath), trimPrefix)
			if rel != "" {
				filesByPath[rel] = struct{}{}
			}
			return nil
		})
	}
	files := make([]string, 0, len(filesByPath))
	for fileName := range filesByPath {
		files = append(files, fileName)
	}
	return files
}

func uniqueTemplateFiles(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = cleanRelativePath(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
