package render

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	richTextImageNodeName         = "editorMediaImage"
	richTextVideoNodeName         = "editorMediaVideo"
	richTextAudioNodeName         = "editorMediaAudio"
	richTextExternalMediaNodeName = "editorMediaEmbed"

	richTextContentClassName = "dever-rich-text min-w-0 text-sm leading-6 text-foreground [&_a]:font-medium [&_a]:text-primary [&_a]:underline [&_a]:decoration-primary/60 [&_a]:underline-offset-4 [&_a:hover]:decoration-primary [&_audio]:my-4 [&_audio]:block [&_audio]:w-full [&_audio]:max-w-sm [&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:text-[0.85em] [&_h1]:mb-3 [&_h1]:mt-4 [&_h1]:text-xl [&_h1]:font-semibold [&_h2]:mb-2.5 [&_h2]:mt-4 [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:mb-2 [&_h3]:mt-3 [&_h3]:text-base [&_h3]:font-semibold [&_h4]:mb-1.5 [&_h4]:mt-3 [&_h4]:text-sm [&_h4]:font-semibold [&_h5]:mb-1.5 [&_h5]:mt-3 [&_h5]:text-sm [&_h5]:font-medium [&_h6]:mb-1.5 [&_h6]:mt-3 [&_h6]:text-xs [&_h6]:font-medium [&_h6]:text-muted-foreground [&_hr]:my-4 [&_hr]:border-border [&_img]:my-4 [&_img]:block [&_img]:max-w-full [&_img]:rounded-lg [&_li]:my-1 [&_ol]:my-3 [&_ol]:list-decimal [&_ol]:pl-6 [&_p]:my-2 [&_pre]:my-3 [&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_strong]:font-semibold [&_table]:my-3 [&_table]:w-full [&_table]:border-collapse [&_td]:border [&_td]:border-border [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-border [&_th]:bg-muted/50 [&_th]:px-2 [&_th]:py-1 [&_th]:text-left [&_ul]:my-3 [&_ul]:list-disc [&_ul]:pl-6 [&_video]:my-4 [&_video]:block [&_video]:max-w-full [&_video]:rounded-lg [&_video]:bg-black/90"
	richTextCaptionClassName = "mt-2 whitespace-pre-line break-words px-1 text-center text-xs leading-5 text-muted-foreground"
	richTextCaptionStyle     = "margin-top: 0.5rem; white-space: pre-line; overflow-wrap: break-word; padding-left: 0.25rem; padding-right: 0.25rem; text-align: center; font-size: 0.75rem; line-height: 1.25rem; color: var(--muted-foreground, #64748b)"
	richTextExternalClass    = "my-4 flex w-full max-w-full items-center gap-3 rounded-lg border bg-muted/25 p-3 no-underline transition hover:bg-muted/40"
)

var (
	richTextHexColorPattern = regexp.MustCompile(`^#(?:[0-9a-f]{3}|[0-9a-f]{6})$`)
	richTextRGBColorPattern = regexp.MustCompile(`^rgba?\([\d\s,./%]+\)$`)
	richTextLinePattern     = regexp.MustCompile(`^(\d+(?:\.\d+)?)(px|em|rem|%)?$`)
	richTextSpacingPattern  = regexp.MustCompile(`^-?\d+(?:\.\d+)?(px|em|rem|%)$`)
	richTextSizePattern     = regexp.MustCompile(`^\d+(?:\.\d+)?(px|%)$`)
	richTextCSSDanger       = regexp.MustCompile(`(?i)(expression|javascript:|url\s*\(|[<>;"{}])`)

	richTextFontFamilies = map[string]struct{}{
		`system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`: {},
		`"Microsoft YaHei", "PingFang SC", sans-serif`:                         {},
		`SimSun, "Songti SC", serif`:                                           {},
		`SimHei, "Heiti SC", sans-serif`:                                       {},
		`"SFMono-Regular", Consolas, "Liberation Mono", monospace`:             {},
	}
)

type richTextNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Marks   []richTextMark `json:"marks"`
	Content []richTextNode `json:"content"`
}

type richTextMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"richText":      renderRichTextTemplateHTML,
		"richTextInner": renderRichTextTemplateInnerHTML,
	}
}

func renderRichTextTemplateHTML(value any) template.HTML {
	return template.HTML(renderRichText(value, true))
}

func renderRichTextTemplateInnerHTML(value any) template.HTML {
	return template.HTML(renderRichText(value, false))
}

func renderRichText(value any, wrapper bool) string {
	doc, ok := parseRichTextDoc(value)
	if !ok {
		return ""
	}

	content := renderRichTextChildren(*doc)
	if !wrapper {
		return content
	}
	return renderRichTextElement("div", map[string]string{"class": richTextContentClassName}, content)
}

func parseRichTextDoc(value any) (*richTextNode, bool) {
	content, ok := richTextJSONBytes(value)
	if !ok {
		return nil, false
	}

	var doc richTextNode
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return nil, false
	}
	doc.Type = normalizeRichTextNodeType(doc.Type)
	if doc.Type != "doc" || !richTextHasVisibleContent(doc) {
		return nil, false
	}
	return &doc, true
}

func richTextJSONBytes(value any) ([]byte, bool) {
	switch current := value.(type) {
	case nil:
		return nil, false
	case string:
		text := strings.TrimSpace(current)
		if text == "" {
			return nil, false
		}
		return []byte(text), true
	case []byte:
		text := strings.TrimSpace(string(current))
		if text == "" {
			return nil, false
		}
		return []byte(text), true
	case json.RawMessage:
		if len(current) == 0 {
			return nil, false
		}
		return current, true
	default:
		content, err := json.Marshal(current)
		return content, err == nil && len(content) > 0
	}
}

func richTextHasVisibleContent(node richTextNode) bool {
	switch normalizeRichTextNodeType(node.Type) {
	case richTextImageNodeName, richTextVideoNodeName, richTextAudioNodeName:
		return richTextAttr(node.Attrs, "src") != ""
	case richTextExternalMediaNodeName:
		return richTextAttr(node.Attrs, "src") != "" || richTextAttr(node.Attrs, "href") != ""
	case "agentAbilityPlaceholder", "agentTaskPlaceholder":
		return true
	case "text":
		return strings.TrimSpace(node.Text) != ""
	}

	for _, child := range node.Content {
		if richTextHasVisibleContent(child) {
			return true
		}
	}
	return false
}

func renderRichTextChildren(node richTextNode) string {
	var output strings.Builder
	for _, child := range node.Content {
		output.WriteString(renderRichTextNode(child))
	}
	return output.String()
}

func renderRichTextNode(node richTextNode) string {
	node.Type = normalizeRichTextNodeType(node.Type)
	children := renderRichTextChildren(node)

	switch node.Type {
	case "blockquote":
		return renderRichTextElement("blockquote", nil, children)
	case "bulletList":
		return renderRichTextElement("ul", nil, children)
	case "codeBlock":
		return renderRichTextElement("pre", nil, renderRichTextElement("code", nil, children))
	case "doc":
		return children
	case "hardBreak":
		return "<br>"
	case "heading":
		return renderRichTextElement(richTextHeadingTag(node), map[string]string{"style": richTextBlockStyle(node.Attrs)}, children)
	case "articleBlock":
		return renderRichTextElement("div", map[string]string{"style": richTextBlockStyle(node.Attrs)}, children)
	case "horizontalRule":
		return "<hr>"
	case "listItem":
		return renderRichTextElement("li", map[string]string{"style": richTextBlockStyle(node.Attrs)}, children)
	case "orderedList":
		return renderRichTextElement("ol", nil, children)
	case "paragraph":
		return renderRichTextElement("p", map[string]string{"style": richTextBlockStyle(node.Attrs)}, children)
	case "agentAbilityPlaceholder", "agentTaskPlaceholder":
		return renderRichTextPlaceholder(node)
	case "table":
		return renderRichTextElement("table", nil, children)
	case "tableCell":
		return renderRichTextElement("td", map[string]string{"style": richTextBlockStyle(node.Attrs)}, children)
	case "tableHeader":
		return renderRichTextElement("th", map[string]string{"style": richTextBlockStyle(node.Attrs)}, children)
	case "tableRow":
		return renderRichTextElement("tr", nil, children)
	case "text":
		return renderRichTextText(node)
	case richTextImageNodeName:
		return renderRichTextImage(node)
	case richTextVideoNodeName:
		return renderRichTextVideo(node)
	case richTextAudioNodeName:
		return renderRichTextAudio(node)
	case richTextExternalMediaNodeName:
		return renderRichTextExternalMedia(node)
	default:
		return children
	}
}

func renderRichTextText(node richTextNode) string {
	text := html.EscapeString(node.Text)
	for i := len(node.Marks) - 1; i >= 0; i-- {
		text = renderRichTextMark(node.Marks[i], text)
	}
	return text
}

func renderRichTextMark(mark richTextMark, children string) string {
	switch strings.TrimSpace(mark.Type) {
	case "bold":
		return renderRichTextElement("strong", nil, children)
	case "code":
		return renderRichTextElement("code", nil, children)
	case "italic":
		return renderRichTextElement("em", nil, children)
	case "link":
		return renderRichTextElement("a", map[string]string{
			"href":   safeRichTextHref(richTextAttr(mark.Attrs, "href")),
			"target": "_blank",
			"rel":    "noreferrer",
		}, children)
	case "strike":
		return renderRichTextElement("s", nil, children)
	case "textStyle":
		style := richTextTextStyle(mark.Attrs)
		if style == "" {
			return children
		}
		return renderRichTextElement("span", map[string]string{"style": style}, children)
	case "underline":
		return renderRichTextElement("u", nil, children)
	default:
		return children
	}
}

func renderRichTextImage(node richTextNode) string {
	src, style := richTextMediaAttrs(node)
	if src == "" {
		return ""
	}

	caption := normalizeRichTextCaption(richTextAttr(node.Attrs, "caption"))
	attrs := map[string]string{
		"src":   src,
		"alt":   richTextAttr(node.Attrs, "alt"),
		"title": richTextAttr(node.Attrs, "title"),
	}
	if caption == "" {
		attrs["style"] = style
		return renderRichTextVoidElement("img", attrs)
	}

	attrs["style"] = "margin: 0; max-width: 100%"
	return renderRichTextElement(
		"figure",
		map[string]string{"class": "my-4 max-w-full", "style": style},
		renderRichTextVoidElement("img", attrs)+renderRichTextCaption(caption),
	)
}

func renderRichTextVideo(node richTextNode) string {
	src, style := richTextMediaAttrs(node)
	if src == "" {
		return ""
	}

	caption := normalizeRichTextCaption(richTextAttr(node.Attrs, "caption"))
	mediaStyle := style
	if caption != "" {
		mediaStyle = "margin: 0; max-width: 100%"
	}
	media := renderRichTextElement("video", map[string]string{
		"src":         src,
		"controls":    "controls",
		"preload":     "metadata",
		"playsinline": "playsinline",
		"style":       mediaStyle,
	}, "")
	if caption == "" {
		return media
	}
	return renderRichTextElement(
		"figure",
		map[string]string{"class": "my-4 max-w-full", "style": style},
		media+renderRichTextCaption(caption),
	)
}

func renderRichTextAudio(node richTextNode) string {
	src, style := richTextMediaAttrs(node)
	if src == "" {
		return ""
	}

	caption := normalizeRichTextCaption(richTextAttr(node.Attrs, "caption"))
	mediaAttrs := map[string]string{
		"src":      src,
		"controls": "controls",
		"preload":  "none",
		"style":    style,
	}
	if caption != "" {
		mediaAttrs["class"] = "block w-full"
		mediaAttrs["style"] = ""
	}
	media := renderRichTextElement("audio", mediaAttrs, "")
	if caption == "" {
		return media
	}
	return renderRichTextElement(
		"figure",
		map[string]string{"class": "my-4 max-w-full", "style": style},
		media+renderRichTextCaption(caption),
	)
}

func renderRichTextExternalMedia(node richTextNode) string {
	src := safeRichTextHref(firstNonEmpty(richTextAttr(node.Attrs, "src"), richTextAttr(node.Attrs, "href")))
	if src == "#" {
		return ""
	}

	kind := strings.ToLower(richTextAttr(node.Attrs, "kind"))
	if kind != "audio" {
		kind = "video"
	}
	title := richTextAttr(node.Attrs, "title")
	if title == "" {
		if kind == "audio" {
			title = "查看音频"
		} else {
			title = "查看视频"
		}
	}

	cover := safeRichTextMediaSource(richTextAttr(node.Attrs, "cover"))
	coverHTML := ""
	if cover != "" {
		coverHTML = renderRichTextVoidElement("img", map[string]string{
			"src":   cover,
			"alt":   "",
			"class": "m-0 h-14 w-20 shrink-0 rounded-md object-cover",
		})
	} else {
		label := "视频"
		if kind == "audio" {
			label = "音频"
		}
		coverHTML = renderRichTextElement("span", map[string]string{
			"class": "flex h-12 w-12 shrink-0 items-center justify-center rounded-md bg-background text-xs font-medium text-muted-foreground",
		}, html.EscapeString(label))
	}

	textHTML := renderRichTextElement(
		"span",
		map[string]string{"class": "min-w-0 flex-1"},
		renderRichTextElement("span", map[string]string{"class": "block truncate text-sm font-medium text-foreground"}, html.EscapeString(title))+
			renderRichTextElement("span", map[string]string{"class": "mt-1 block truncate text-xs text-muted-foreground"}, html.EscapeString(src)),
	)
	_, style := richTextMediaAttrs(node)
	return renderRichTextElement(
		"div",
		map[string]string{"style": richTextMediaWrapperStyle(node)},
		renderRichTextElement("a", map[string]string{
			"href":   src,
			"target": "_blank",
			"rel":    "noreferrer",
			"class":  richTextExternalClass,
			"style":  style,
		}, coverHTML+textHTML),
	)
}

func renderRichTextPlaceholder(node richTextNode) string {
	status := richTextAttr(node.Attrs, "status")
	title := firstNonEmpty(richTextAttr(node.Attrs, "title"), richTextAttr(node.Attrs, "kind"), "素材生成")
	text := firstNonEmpty(richTextAttr(node.Attrs, "error"), richTextAttr(node.Attrs, "text"))
	progress := normalizeRichTextProgress(richTextAttr(node.Attrs, "progress"))

	stateClass := ""
	progressClass := "bg-primary"
	if status == "failed" {
		stateClass = " border-destructive/30 bg-destructive/10"
		progressClass = "bg-destructive"
	} else if status == "succeeded" {
		stateClass = " border-emerald-500/25 bg-emerald-500/10"
		progressClass = "bg-emerald-500"
	}

	progressLabel := ""
	progressBar := renderRichTextElement("div", map[string]string{"class": "h-full w-1/3 animate-pulse rounded-full bg-primary/75"}, "")
	if progress >= 0 {
		progressLabel = renderRichTextElement("span", map[string]string{"class": "shrink-0 tabular-nums"}, strconv.Itoa(progress)+"%")
		progressBar = renderRichTextElement("div", map[string]string{
			"class": "h-full rounded-full transition-[width] " + progressClass,
			"style": "width: " + strconv.Itoa(progress) + "%",
		}, "")
	}

	summary := html.EscapeString(title)
	if text != "" {
		summary += "：" + html.EscapeString(text)
	}
	return renderRichTextElement(
		"div",
		map[string]string{"class": "my-4 rounded-lg border bg-muted/25 px-3 py-2" + stateClass},
		renderRichTextElement(
			"div",
			map[string]string{"class": "mb-2 flex items-center justify-between gap-3 text-xs text-muted-foreground"},
			renderRichTextElement("span", map[string]string{"class": "truncate"}, summary)+progressLabel,
		)+renderRichTextElement("div", map[string]string{"class": "h-1.5 overflow-hidden rounded-full bg-muted"}, progressBar),
	)
}

func richTextHeadingTag(node richTextNode) string {
	level, _ := strconv.Atoi(richTextAttr(node.Attrs, "level"))
	if level < 1 || level > 6 {
		level = 2
	}
	return "h" + strconv.Itoa(level)
}

func renderRichTextCaption(caption string) string {
	return renderRichTextElement(
		"figcaption",
		map[string]string{"class": richTextCaptionClassName, "style": richTextCaptionStyle},
		html.EscapeString(caption),
	)
}

func richTextMediaAttrs(node richTextNode) (string, string) {
	src := safeRichTextMediaSource(richTextAttr(node.Attrs, "src"))
	if src == "" {
		return "", ""
	}

	style := map[string]string{}
	if maxWidth := normalizeRichTextMaxWidth(richTextAttr(node.Attrs, "maxWidth")); maxWidth != "" {
		style["max-width"] = maxWidth
	}
	for key, value := range richTextMediaAlignStyle(normalizeRichTextMediaAlign(richTextAttr(node.Attrs, "align"))) {
		style[key] = value
	}
	return src, richTextStyleText(style)
}

func richTextMediaWrapperStyle(node richTextNode) string {
	switch normalizeRichTextMediaAlign(richTextAttr(node.Attrs, "align")) {
	case "center":
		return "display: flex; justify-content: center"
	case "right":
		return "display: flex; justify-content: flex-end"
	default:
		return ""
	}
}

func richTextMediaAlignStyle(align string) map[string]string {
	switch align {
	case "center":
		return map[string]string{"margin-left": "auto", "margin-right": "auto"}
	case "right":
		return map[string]string{"margin-left": "auto"}
	default:
		return nil
	}
}

func richTextTextStyle(attrs map[string]any) string {
	return richTextStyleFromAttrs(attrs, []string{"color", "backgroundColor", "fontFamily", "fontSize"})
}

func richTextBlockStyle(attrs map[string]any) string {
	return richTextStyleFromAttrs(attrs, []string{
		"textAlign",
		"lineHeight",
		"color",
		"backgroundColor",
		"fontFamily",
		"fontSize",
		"textIndent",
		"marginTop",
		"marginRight",
		"marginBottom",
		"marginLeft",
		"paddingTop",
		"paddingRight",
		"paddingBottom",
		"paddingLeft",
		"border",
		"borderTop",
		"borderRight",
		"borderBottom",
		"borderLeft",
		"borderRadius",
		"width",
		"maxWidth",
	})
}

func richTextStyleFromAttrs(attrs map[string]any, keys []string) string {
	style := map[string]string{}
	for _, key := range keys {
		value := normalizeRichTextStyleValue(key, richTextAttr(attrs, key))
		if value != "" {
			style[richTextCSSName(key)] = value
		}
	}
	return richTextStyleText(style)
}

func normalizeRichTextStyleValue(key string, value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}

	switch key {
	case "textAlign":
		if text == "left" || text == "center" || text == "right" || text == "justify" {
			return text
		}
	case "lineHeight":
		return normalizeRichTextLineHeight(text)
	case "color", "backgroundColor":
		return normalizeRichTextColor(text)
	case "fontSize":
		if text == "12px" || text == "14px" || text == "16px" || text == "18px" || text == "24px" || text == "32px" {
			return text
		}
	case "fontFamily":
		if _, ok := richTextFontFamilies[text]; ok {
			return text
		}
	case "width", "maxWidth":
		if richTextSizePattern.MatchString(text) {
			return text
		}
	case "marginTop", "marginRight", "marginBottom", "marginLeft":
		if text == "auto" || richTextSpacingPattern.MatchString(text) {
			return text
		}
	case "paddingTop", "paddingRight", "paddingBottom", "paddingLeft", "textIndent":
		if richTextSpacingPattern.MatchString(text) {
			return text
		}
	case "border", "borderTop", "borderRight", "borderBottom", "borderLeft", "borderRadius":
		if richTextSafeCSSValue(text) {
			return text
		}
	}
	return ""
}

func normalizeRichTextLineHeight(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "normal" {
		return text
	}
	match := richTextLinePattern.FindStringSubmatch(text)
	if len(match) != 3 {
		return ""
	}

	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil || number <= 0 {
		return ""
	}
	unit := match[2]
	if unit == "" && number >= 1 && number <= 3 {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	if unit == "%" && number >= 80 && number <= 300 {
		return text
	}
	if (unit == "em" || unit == "rem") && number >= 0.8 && number <= 3 {
		return text
	}
	if unit == "px" && number >= 12 && number <= 96 {
		return text
	}
	return ""
}

func normalizeRichTextColor(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if richTextHexColorPattern.MatchString(text) || richTextRGBColorPattern.MatchString(text) {
		return text
	}
	if text == "transparent" {
		return text
	}
	return ""
}

func normalizeRichTextMaxWidth(value string) string {
	text := strings.TrimSpace(value)
	if richTextSizePattern.MatchString(text) {
		return text
	}
	return ""
}

func normalizeRichTextMediaAlign(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "center" || text == "right" {
		return text
	}
	return "left"
}

func normalizeRichTextCaption(value string) string {
	text := strings.ReplaceAll(value, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	text = strings.TrimSpace(strings.Join(lines, "\n"))
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	runes := []rune(text)
	if len(runes) > 1000 {
		return string(runes[:1000])
	}
	return text
}

func normalizeRichTextProgress(value string) int {
	progress, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return -1
	}
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return int(progress + 0.5)
}

func richTextCSSName(key string) string {
	var output strings.Builder
	for index, char := range key {
		if char >= 'A' && char <= 'Z' {
			if index > 0 {
				output.WriteByte('-')
			}
			output.WriteRune(char + ('a' - 'A'))
			continue
		}
		output.WriteRune(char)
	}
	return output.String()
}

func richTextStyleText(style map[string]string) string {
	if len(style) == 0 {
		return ""
	}
	keys := make([]string, 0, len(style))
	for key := range style {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(style[key])
		if value != "" {
			parts = append(parts, key+": "+value)
		}
	}
	return strings.Join(parts, "; ")
}

func richTextSafeCSSValue(value string) bool {
	text := strings.TrimSpace(value)
	return text != "" && !richTextCSSDanger.MatchString(text)
}

func normalizeRichTextNodeType(value string) string {
	switch strings.TrimSpace(value) {
	case "image", "mediaImage":
		return richTextImageNodeName
	case "video", "mediaVideo":
		return richTextVideoNodeName
	case "audio", "mediaAudio":
		return richTextAudioNodeName
	case "editorMediaExternal", "externalMedia", "mediaEmbed", "mediaExternal":
		return richTextExternalMediaNodeName
	default:
		return strings.TrimSpace(value)
	}
}

func renderRichTextElement(tag string, attrs map[string]string, children string) string {
	return "<" + tag + renderRichTextAttrs(attrs) + ">" + children + "</" + tag + ">"
}

func renderRichTextVoidElement(tag string, attrs map[string]string) string {
	return "<" + tag + renderRichTextAttrs(attrs) + ">"
}

func renderRichTextAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var output strings.Builder
	for _, key := range keys {
		value := strings.TrimSpace(attrs[key])
		if value == "" {
			continue
		}
		output.WriteByte(' ')
		output.WriteString(key)
		output.WriteString(`="`)
		output.WriteString(html.EscapeString(value))
		output.WriteByte('"')
	}
	return output.String()
}

func richTextAttr(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
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
	case bool:
		if current {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func safeRichTextHref(value string) string {
	text := strings.TrimSpace(value)
	if isSafeRichTextURL(text, true) {
		return text
	}
	return "#"
}

func safeRichTextMediaSource(value string) string {
	text := strings.TrimSpace(value)
	if isSafeRichTextURL(text, false) {
		return text
	}
	return ""
}

func isSafeRichTextURL(value string, allowMailto bool) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	case "mailto":
		return allowMailto && parsed.Opaque != ""
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
