package richtext

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

type HTMLOptions struct {
	Title     string
	MediaURLs map[string]string
}

func FromHTML(source string, options HTMLOptions) (map[string]any, error) {
	document, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("解析富文本失败: %w", err)
	}

	body := findElement(document, "body")
	if body == nil {
		body = document
	}
	content := compactDocumentContent(blockNodes(body.FirstChild, options))
	if title := strings.TrimSpace(options.Title); title != "" && !startsWithTitle(content, title) {
		content = append([]any{headingNode(title, 1)}, content...)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("富文本内容为空")
	}
	return map[string]any{"type": "doc", "content": content}, nil
}

func blockNodes(first *html.Node, options HTMLOptions) []any {
	result := make([]any, 0)
	for node := first; node != nil; node = node.NextSibling {
		result = append(result, blockNode(node, options)...)
	}
	return result
}

func blockNode(node *html.Node, options HTMLOptions) []any {
	if node == nil {
		return nil
	}
	if node.Type == html.TextNode {
		if text := normalizeHTMLText(node.Data); strings.TrimSpace(text) != "" {
			return []any{paragraphNode([]any{textNode(strings.TrimSpace(text), nil)})}
		}
		return nil
	}
	if node.Type != html.ElementNode {
		return blockNodes(node.FirstChild, options)
	}

	switch strings.ToLower(node.Data) {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(node.Data[1] - '0')
		return withBlockTextAlignment([]any{map[string]any{"type": "heading", "attrs": map[string]any{"level": level}, "content": inlineNodes(node.FirstChild, options, nil)}}, node)
	case "p":
		return withBlockTextAlignment(paragraphAndMediaNodes(node.FirstChild, options), node)
	case "blockquote":
		return withBlockTextAlignment([]any{map[string]any{"type": "blockquote", "content": ensureBlockChildren(blockNodes(node.FirstChild, options))}}, node)
	case "ul", "ol":
		return []any{listNode(node, options)}
	case "pre":
		text := strings.TrimSpace(textContent(node))
		if text == "" {
			return nil
		}
		return []any{map[string]any{"type": "codeBlock", "content": []any{textNode(text, nil)}}}
	case "hr":
		return []any{map[string]any{"type": "horizontalRule"}}
	case "table":
		if table := tableNode(node, options); table != nil {
			return []any{table}
		}
		return nil
	case "img", "video", "audio", "iframe":
		if media := mediaNode(node, options, ""); media != nil {
			return []any{media}
		}
		return nil
	case "figure":
		caption := strings.TrimSpace(elementText(node, "figcaption"))
		result := make([]any, 0)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && child.Data == "figcaption" {
				continue
			}
			if media := mediaNode(child, options, caption); media != nil {
				result = append(result, media)
				continue
			}
			result = append(result, blockNode(child, options)...)
		}
		return result
	case "article", "section", "main", "header", "footer", "div", "body":
		if hasBlockChild(node) {
			return blockNodes(node.FirstChild, options)
		}
		return paragraphAndMediaNodes(node.FirstChild, options)
	case "script", "style", "noscript", "template":
		return nil
	default:
		if hasBlockChild(node) {
			return blockNodes(node.FirstChild, options)
		}
		return paragraphAndMediaNodes(node.FirstChild, options)
	}
}

func paragraphAndMediaNodes(first *html.Node, options HTMLOptions) []any {
	result := make([]any, 0)
	inline := make([]any, 0)
	flush := func() {
		inline = trimParagraphContent(inline)
		if len(inline) > 0 {
			result = append(result, paragraphNode(inline))
		}
		inline = nil
	}
	var appendNode func(*html.Node, []any)
	appendNode = func(node *html.Node, marks []any) {
		if node == nil {
			return
		}
		if media := mediaNode(node, options, ""); media != nil {
			flush()
			result = append(result, media)
			return
		}
		if node.Type == html.TextNode {
			if text := normalizeHTMLText(node.Data); text != "" {
				inline = append(inline, textNode(text, marks))
			}
			return
		}
		if node.Type != html.ElementNode {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				appendNode(child, marks)
			}
			return
		}
		switch strings.ToLower(node.Data) {
		case "br":
			inline = append(inline, map[string]any{"type": "hardBreak"})
			return
		case "script", "style", "noscript", "template":
			return
		}
		nextMarks := inlineElementMarks(node, marks)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			appendNode(child, nextMarks)
		}
	}
	for node := first; node != nil; node = node.NextSibling {
		appendNode(node, nil)
	}
	flush()
	return result
}

func inlineNodes(first *html.Node, options HTMLOptions, marks []any) []any {
	result := make([]any, 0)
	for node := first; node != nil; node = node.NextSibling {
		result = append(result, inlineNode(node, options, marks)...)
	}
	return trimInlineWhitespace(result)
}

func inlineNode(node *html.Node, options HTMLOptions, marks []any) []any {
	if node == nil {
		return nil
	}
	if node.Type == html.TextNode {
		if text := normalizeHTMLText(node.Data); text != "" {
			return []any{textNode(text, marks)}
		}
		return nil
	}
	if node.Type != html.ElementNode {
		return inlineNodes(node.FirstChild, options, marks)
	}
	if media := mediaNode(node, options, ""); media != nil {
		return []any{media}
	}

	switch strings.ToLower(node.Data) {
	case "br":
		return []any{map[string]any{"type": "hardBreak"}}
	case "script", "style", "noscript", "template":
		return nil
	}
	return inlineNodes(node.FirstChild, options, inlineElementMarks(node, marks))
}

func inlineElementMarks(node *html.Node, marks []any) []any {
	nextMarks := append([]any(nil), marks...)
	switch strings.ToLower(node.Data) {
	case "strong", "b":
		nextMarks = append(nextMarks, map[string]any{"type": "bold"})
	case "em", "i":
		nextMarks = append(nextMarks, map[string]any{"type": "italic"})
	case "u":
		nextMarks = append(nextMarks, map[string]any{"type": "underline"})
	case "s", "strike", "del":
		nextMarks = append(nextMarks, map[string]any{"type": "strike"})
	case "code":
		nextMarks = append(nextMarks, map[string]any{"type": "code"})
	case "a":
		if href := safeLink(attribute(node, "href")); href != "" {
			nextMarks = append(nextMarks, map[string]any{"type": "link", "attrs": map[string]any{"href": href, "target": "_blank"}})
		}
	}
	return nextMarks
}

func listNode(node *html.Node, options HTMLOptions) map[string]any {
	listType := "bulletList"
	if strings.EqualFold(node.Data, "ol") {
		listType = "orderedList"
	}
	items := make([]any, 0)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || !strings.EqualFold(child.Data, "li") {
			continue
		}
		children := blockNodes(child.FirstChild, options)
		items = append(items, map[string]any{"type": "listItem", "content": ensureBlockChildren(children)})
	}
	return map[string]any{"type": listType, "content": items}
}

func tableNode(node *html.Node, options HTMLOptions) map[string]any {
	rows := make([]any, 0)
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && strings.EqualFold(current.Data, "tr") {
			cells := make([]any, 0)
			for cell := current.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type != html.ElementNode || (cell.Data != "td" && cell.Data != "th") {
					continue
				}
				cellType := "tableCell"
				if cell.Data == "th" {
					cellType = "tableHeader"
				}
				cells = append(cells, map[string]any{"type": cellType, "content": ensureBlockChildren(blockNodes(cell.FirstChild, options))})
			}
			if len(cells) > 0 {
				rows = append(rows, map[string]any{"type": "tableRow", "content": cells})
			}
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	if len(rows) == 0 {
		return nil
	}
	return map[string]any{"type": "table", "content": rows}
}

func mediaNode(node *html.Node, options HTMLOptions, caption string) map[string]any {
	if node == nil || node.Type != html.ElementNode {
		return nil
	}
	tag := strings.ToLower(node.Data)
	if tag != "img" && tag != "video" && tag != "audio" && tag != "iframe" {
		return nil
	}
	source := firstAttribute(node, "src", "data-src", "data-original", "data-url")
	if source == "" && (tag == "video" || tag == "audio") {
		if child := findElement(node, "source"); child != nil {
			source = attribute(child, "src")
		}
	}
	if replacement := strings.TrimSpace(options.MediaURLs[source]); replacement != "" {
		source = replacement
	}
	if strings.TrimSpace(source) == "" {
		return nil
	}

	attrs := map[string]any{"src": strings.TrimSpace(source)}
	if alt := strings.TrimSpace(attribute(node, "alt")); alt != "" {
		attrs["alt"] = alt
	}
	if title := strings.TrimSpace(attribute(node, "title")); title != "" {
		attrs["title"] = title
	}
	if caption = strings.TrimSpace(caption); caption != "" {
		attrs["caption"] = caption
	}
	switch tag {
	case "img":
		return map[string]any{"type": "editorMediaImage", "attrs": attrs}
	case "video":
		if poster := firstAttribute(node, "poster", "data-poster"); poster != "" {
			if replacement := strings.TrimSpace(options.MediaURLs[poster]); replacement != "" {
				poster = replacement
			}
			attrs["poster"] = poster
		}
		return map[string]any{"type": "editorMediaVideo", "attrs": attrs}
	case "audio":
		return map[string]any{"type": "editorMediaAudio", "attrs": attrs}
	default:
		attrs["href"] = attrs["src"]
		return map[string]any{"type": "externalMedia", "attrs": attrs}
	}
}

func headingNode(text string, level int) map[string]any {
	return map[string]any{"type": "heading", "attrs": map[string]any{"level": level}, "content": []any{textNode(text, nil)}}
}

func paragraphNode(content []any) map[string]any {
	return map[string]any{"type": "paragraph", "content": content}
}

func textNode(text string, marks []any) map[string]any {
	node := map[string]any{"type": "text", "text": text}
	if len(marks) > 0 {
		node["marks"] = append([]any(nil), marks...)
	}
	return node
}

func ensureBlockChildren(content []any) []any {
	if len(content) > 0 {
		return content
	}
	return []any{paragraphNode(nil)}
}

func compactDocumentContent(content []any) []any {
	result := make([]any, 0, len(content))
	for _, current := range content {
		node, ok := current.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(node["type"])) == "" {
			continue
		}
		if node["type"] == "paragraph" {
			children, _ := node["content"].([]any)
			children = trimParagraphContent(children)
			if len(children) == 0 {
				continue
			}
			node["content"] = children
		}
		result = append(result, node)
	}
	return result
}

func startsWithTitle(content []any, title string) bool {
	if len(content) == 0 {
		return false
	}
	node, ok := content[0].(map[string]any)
	if !ok || node["type"] != "heading" {
		return false
	}
	return strings.Join(strings.Fields(richNodeText(node)), " ") == strings.Join(strings.Fields(title), " ")
}

func richNodeText(value any) string {
	node, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if node["type"] == "text" {
		return fmt.Sprint(node["text"])
	}
	children, _ := node["content"].([]any)
	var result strings.Builder
	for _, child := range children {
		result.WriteString(richNodeText(child))
	}
	return result.String()
}

func trimInlineWhitespace(nodes []any) []any {
	for len(nodes) > 0 {
		text, ok := nodes[0].(map[string]any)
		if !ok || text["type"] != "text" {
			break
		}
		value := strings.TrimLeftFunc(fmt.Sprint(text["text"]), unicode.IsSpace)
		if value != "" {
			text["text"] = value
			break
		}
		nodes = nodes[1:]
	}
	for len(nodes) > 0 {
		last := len(nodes) - 1
		text, ok := nodes[last].(map[string]any)
		if !ok || text["type"] != "text" {
			break
		}
		value := strings.TrimRightFunc(fmt.Sprint(text["text"]), unicode.IsSpace)
		if value != "" {
			text["text"] = value
			break
		}
		nodes = nodes[:last]
	}
	return nodes
}

func trimParagraphContent(nodes []any) []any {
	nodes = trimInlineWhitespace(nodes)
	for len(nodes) > 0 {
		last, ok := nodes[len(nodes)-1].(map[string]any)
		if !ok || last["type"] != "hardBreak" {
			break
		}
		nodes = trimInlineWhitespace(nodes[:len(nodes)-1])
	}
	return nodes
}

func withBlockTextAlignment(nodes []any, source *html.Node) []any {
	alignment := blockTextAlignment(source)
	if alignment == "" {
		return nodes
	}
	for _, current := range nodes {
		node, ok := current.(map[string]any)
		if !ok {
			continue
		}
		attributeKey := ""
		switch node["type"] {
		case "paragraph", "heading", "blockquote":
			attributeKey = "textAlign"
		case "editorMediaImage", "editorMediaVideo", "editorMediaAudio":
			if alignment != "justify" {
				attributeKey = "align"
			}
		}
		if attributeKey == "" {
			continue
		}
		attrs, _ := node["attrs"].(map[string]any)
		if attrs == nil {
			attrs = make(map[string]any)
			node["attrs"] = attrs
		}
		attrs[attributeKey] = alignment
	}
	return nodes
}

func blockTextAlignment(node *html.Node) string {
	alignment := attribute(node, "align")
	for _, declaration := range strings.Split(attribute(node, "style"), ";") {
		key, value, ok := strings.Cut(declaration, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "text-align") {
			alignment = value
		}
	}
	switch strings.ToLower(strings.TrimSpace(alignment)) {
	case "left", "center", "right", "justify":
		return strings.ToLower(strings.TrimSpace(alignment))
	default:
		return ""
	}
}

func normalizeHTMLText(value string) string {
	if value == "" {
		return ""
	}
	leadingRune, _ := utf8.DecodeRuneInString(value)
	trailingRune, _ := utf8.DecodeLastRuneInString(value)
	leading := unicode.IsSpace(leadingRune)
	trailing := unicode.IsSpace(trailingRune)
	text := strings.Join(strings.Fields(value), " ")
	if text == "" {
		if leading || trailing {
			return " "
		}
		return ""
	}
	if leading {
		text = " " + text
	}
	if trailing {
		text += " "
	}
	return text
}

func hasBlockChild(node *html.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		switch strings.ToLower(child.Data) {
		case "article", "section", "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "ul", "ol", "pre", "hr", "table", "figure":
			return true
		}
	}
	return false
}

func findElement(node *html.Node, name string) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func elementText(node *html.Node, name string) string {
	if child := findElement(node, name); child != nil {
		return textContent(child)
	}
	return ""
}

func textContent(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var result strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		result.WriteString(textContent(child))
	}
	return result.String()
}

func attribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func firstAttribute(node *html.Node, keys ...string) string {
	for _, key := range keys {
		if value := attribute(node, key); value != "" {
			return value
		}
	}
	return ""
}

func safeLink(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "mailto", "tel":
			return value
		default:
			return ""
		}
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "#") {
		return value
	}
	return ""
}
