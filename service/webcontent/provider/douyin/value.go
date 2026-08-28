package douyin

import (
	"encoding/json"
	stdhtml "html"
	"net/url"
	"strconv"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"
)

func mapFromValue(value any) (map[string]any, bool) {
	if current, ok := value.(map[string]any); ok {
		return current, true
	}
	text, ok := value.(string)
	if !ok || !looksLikeJSON(text) {
		return nil, false
	}
	payload, ok := decodeJSON(text)
	if !ok {
		return nil, false
	}
	current, ok := payload.(map[string]any)
	return current, ok
}

func mapValue(data map[string]any, keys ...string) any {
	if len(data) == 0 {
		return nil
	}
	for _, key := range keys {
		target := normalizeKey(key)
		for currentKey, value := range data {
			if normalizeKey(currentKey) == target {
				return value
			}
		}
	}
	return nil
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func firstURL(value any) string {
	switch current := value.(type) {
	case string:
		return normalizeHTTPURL(current)
	case []any:
		for _, child := range current {
			if result := firstURL(child); result != "" {
				return result
			}
		}
	case map[string]any:
		for _, key := range []string{
			"url_list", "urlList", "main_url", "mainUrl", "backup_url", "backupUrl",
			"fallback_api", "fallbackApi", "url", "src",
		} {
			if result := firstURL(mapValue(current, key)); result != "" {
				return result
			}
		}
	}
	return ""
}

func normalizeHTTPURL(value string) string {
	value = strings.TrimSpace(stdhtml.UnescapeString(strings.ReplaceAll(value, `\/`, `/`)))
	if strings.HasPrefix(value, "//") {
		value = "https:" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return parsed.String()
}

func stringValue(value any) string {
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	case json.Number:
		return current.String()
	case float64:
		return strconv.FormatFloat(current, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(current), 'f', -1, 32)
	case int:
		return strconv.Itoa(current)
	case int64:
		return strconv.FormatInt(current, 10)
	case uint64:
		return strconv.FormatUint(current, 10)
	default:
		return ""
	}
}

func numberValue(value any) float64 {
	switch current := value.(type) {
	case json.Number:
		result, _ := current.Float64()
		return result
	case float64:
		return current
	case float32:
		return float64(current)
	case int:
		return float64(current)
	case int64:
		return float64(current)
	case uint64:
		return float64(current)
	case string:
		result, _ := strconv.ParseFloat(strings.TrimSpace(current), 64)
		return result
	default:
		return 0
	}
}

func int64Value(value any) int64 {
	return int64(numberValue(value))
}

func secondsToMilliseconds(value float64) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value * 1000)
}

func normalizeDurationMilliseconds(value float64) int64 {
	if value <= 0 {
		return 0
	}
	if value < 1000 {
		return secondsToMilliseconds(value)
	}
	return int64(value)
}

func unixTimePointer(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	result := time.Unix(value, 0).UTC()
	return &result
}

func firstTextLine(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80])
	}
	return value
}

func looksLikeJSON(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")
}

func findElements(root *nethtml.Node, match func(*nethtml.Node) bool, result *[]*nethtml.Node) {
	if root == nil {
		return
	}
	if match(root) {
		*result = append(*result, root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		findElements(child, match, result)
	}
}

func scriptText(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.TextNode {
			builder.WriteString(child.Data)
		}
	}
	return builder.String()
}

func attribute(node *nethtml.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}
