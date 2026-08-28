package richtext

import "strings"

var richTextMediaURLAttrs = map[string][]string{
	"editorMediaImage": {"src"},
	"editorMediaVideo": {"src", "poster"},
	"editorMediaAudio": {"src"},
	"externalMedia":    {"src", "href"},
}

func RewriteMediaURLs(document map[string]any, replacements map[string]string) map[string]any {
	rewritten, _ := rewriteRichTextValue(document, replacements).(map[string]any)
	if rewritten == nil {
		return map[string]any{}
	}
	return rewritten
}

func rewriteRichTextValue(value any, replacements map[string]string) any {
	switch current := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(current))
		for key, item := range current {
			next[key] = rewriteRichTextValue(item, replacements)
		}
		rewriteRichTextNodeAttrs(next, replacements)
		return next
	case []any:
		next := make([]any, len(current))
		for index, item := range current {
			next[index] = rewriteRichTextValue(item, replacements)
		}
		return next
	case []map[string]any:
		next := make([]map[string]any, len(current))
		for index, item := range current {
			next[index], _ = rewriteRichTextValue(item, replacements).(map[string]any)
		}
		return next
	default:
		return value
	}
}

func rewriteRichTextNodeAttrs(node map[string]any, replacements map[string]string) {
	nodeType, _ := node["type"].(string)
	attrsToRewrite := richTextMediaURLAttrs[nodeType]
	if len(attrsToRewrite) == 0 {
		return
	}
	attrs, _ := node["attrs"].(map[string]any)
	for _, attr := range attrsToRewrite {
		source, _ := attrs[attr].(string)
		replacement := strings.TrimSpace(replacements[strings.TrimSpace(source)])
		if replacement != "" {
			attrs[attr] = replacement
		}
	}
}
