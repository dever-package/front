package page

import (
	"fmt"
	"strings"

	frontrecord "github.com/dever-package/front/service/record"
)

func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "\\", "/")
	return path
}

func DefaultModelName(pathValue string) string {
	pathValue = NormalizePath(pathValue)
	segments := splitPathSegments(pathValue)
	if len(segments) == 0 {
		return ""
	}

	moduleName := segments[0]
	resourceSegments := normalizeModelSegments(segments)
	candidates := defaultModelCandidates(moduleName, resourceSegments)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if frontrecord.LoadSafe(candidate) != nil {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func splitPathSegments(pathValue string) []string {
	parts := strings.Split(strings.Trim(NormalizePath(pathValue), "/"), "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func normalizeModelSegments(segments []string) []string {
	if len(segments) <= 1 {
		return nil
	}

	resourceSegments := append([]string(nil), segments[1:]...)
	if len(resourceSegments) == 0 {
		return nil
	}

	last := strings.ToLower(strings.TrimSpace(resourceSegments[len(resourceSegments)-1]))
	switch last {
	case "list", "update", "detail", "info", "create", "view":
		resourceSegments = resourceSegments[:len(resourceSegments)-1]
	}
	return resourceSegments
}

func defaultModelCandidates(moduleName string, resourceSegments []string) []string {
	moduleName = strings.TrimSpace(moduleName)
	if moduleName == "" {
		return nil
	}

	modulePascal := toPascal(moduleName)
	if len(resourceSegments) == 0 {
		return []string{
			fmt.Sprintf("%s.New%sModel", moduleName, modulePascal),
		}
	}

	resourceName := toPascal(strings.Join(resourceSegments, "/"))
	return uniqueStrings([]string{
		fmt.Sprintf("%s.New%sModel", moduleName, resourceName),
		fmt.Sprintf("%s.New%s%sModel", moduleName, modulePascal, resourceName),
	})
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
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

func toPascal(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == '/' || r == ' '
	})
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			builder.WriteString(strings.ToLower(part[1:]))
		}
	}
	return builder.String()
}
