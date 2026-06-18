package page

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	authctx "my/package/front/service/internal/authctx"
	"my/package/front/service/siteconfig"
)

type TemplateContext struct {
	Context context.Context
	Form    map[string]any
	Payload any
	Data    map[string]any
	Route   map[string]any
	Query   map[string]any
	Site    map[string]any
	User    map[string]any
}

func ResolveTemplateValue(value any, ctx TemplateContext) any {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, item := range current {
			result[key] = ResolveTemplateValue(item, ctx)
		}
		return result
	case []any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			result = append(result, ResolveTemplateValue(item, ctx))
		}
		return result
	case string:
		return resolveTemplateReference(current, ctx)
	default:
		return value
	}
}

func resolveTemplateReference(value string, ctx TemplateContext) any {
	if strings.Contains(value, "${") {
		return interpolateTemplateString(value, ctx)
	}

	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "$") {
		return value
	}

	return resolveTemplatePath(strings.TrimPrefix(trimmed, "$"), ctx)
}

func resolveTemplatePath(reference string, ctx TemplateContext) any {
	root, path := splitTemplateReference(reference)
	switch root {
	case "form":
		return readTemplateValue(ctx.Form, path)
	case "payload":
		return readTemplateValue(ctx.Payload, path)
	case "data":
		return readTemplateValue(ctx.Data, path)
	case "route":
		return readTemplateValue(ctx.Route, path)
	case "query":
		return readTemplateValue(ctx.Query, path)
	case "site":
		if len(ctx.Site) == 0 {
			ctx.Site = SiteTemplateData(ctx.Context)
		}
		return readTemplateValue(ctx.Site, path)
	case "user":
		if len(ctx.User) == 0 {
			ctx.User = UserTemplateData(ctx.Context)
		}
		return readTemplateValue(ctx.User, path)
	default:
		return nil
	}
}

func interpolateTemplateString(value string, ctx TemplateContext) string {
	var builder strings.Builder
	rest := value
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			builder.WriteString(rest)
			break
		}
		builder.WriteString(rest[:start])
		rest = rest[start+2:]

		end := strings.Index(rest, "}")
		if end < 0 {
			builder.WriteString("${")
			builder.WriteString(rest)
			break
		}

		reference := strings.TrimSpace(rest[:end])
		if resolved := resolveTemplatePath(reference, ctx); resolved != nil {
			builder.WriteString(fmt.Sprint(resolved))
		}
		rest = rest[end+1:]
	}
	return builder.String()
}

func splitTemplateReference(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	root, path, ok := strings.Cut(value, ".")
	if !ok {
		return value, ""
	}
	return strings.TrimSpace(root), strings.TrimSpace(path)
}

func readTemplateValue(source any, path string) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return source
	}
	return GetByDotPath(source, path)
}

func GetByDotPath(source any, path string) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return source
	}

	current := source
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" || current == nil {
			return nil
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[segment]
		case map[string]string:
			current = typed[segment]
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil {
				return nil
			}
			if index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}
	return current
}

func SiteTemplateData(ctx context.Context) map[string]any {
	site, _ := siteconfig.FromContext(ctx)
	if site.Key == "" {
		return map[string]any{
			"key":  siteconfig.DefaultSiteKey,
			"page": siteconfig.DefaultPage,
			"api":  siteconfig.DefaultAPI,
		}
	}
	return map[string]any{
		"key":  site.Key,
		"path": site.Path,
		"page": site.Page,
		"api":  site.API,
		"name": site.Name,
	}
}

func UserTemplateData(ctx context.Context) map[string]any {
	uid := authctx.OptionalUID(ctx)
	if uid == 0 {
		return map[string]any{}
	}
	return map[string]any{
		"id": uid,
	}
}
