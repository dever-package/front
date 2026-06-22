package article

import (
	"fmt"
	stdhtml "html"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
)

type articleHTMLSanitizer struct {
	baseURL   *url.URL
	assets    []ArticleAsset
	seenAsset map[string]struct{}
	maxAssets int
}

var articleDroppedTags = map[string]struct{}{
	"script":   {},
	"style":    {},
	"meta":     {},
	"link":     {},
	"object":   {},
	"embed":    {},
	"form":     {},
	"input":    {},
	"button":   {},
	"textarea": {},
	"select":   {},
	"noscript": {},
}

var articleWrappedTags = map[string]struct{}{
	"p":          {},
	"h1":         {},
	"h2":         {},
	"h3":         {},
	"blockquote": {},
	"ul":         {},
	"ol":         {},
	"li":         {},
	"table":      {},
	"thead":      {},
	"tbody":      {},
	"tr":         {},
	"th":         {},
	"td":         {},
	"strong":     {},
	"b":          {},
	"em":         {},
	"i":          {},
	"u":          {},
	"s":          {},
	"del":        {},
	"code":       {},
	"pre":        {},
	"span":       {},
	"mark":       {},
	"small":      {},
}

var articleContainerTags = map[string]struct{}{
	"body":       {},
	"div":        {},
	"section":    {},
	"article":    {},
	"main":       {},
	"figure":     {},
	"figcaption": {},
}

var articleMediaSourceAttrs = []string{
	"data-src",
	"data-original",
	"data-original-src",
	"data-lazy-src",
	"data-lazyload",
	"data-actualsrc",
	"data-origin-src",
	"data-url",
	"data-original-url",
	"data-raw-src",
	"data-imgurl",
	"data-image",
	"data-thumb",
	"data-thumburl",
	"data-backup",
	"data-echo",
	"origin-src",
	"lazy-src",
	"src",
}

var articleSourceSetAttrs = []string{"data-srcset", "srcset"}

var articleAllowedStyleProperties = map[string]struct{}{
	"color":            {},
	"background-color": {},
	"font-size":        {},
	"font-family":      {},
	"font-weight":      {},
	"font-style":       {},
	"text-decoration":  {},
	"text-align":       {},
	"line-height":      {},
	"letter-spacing":   {},
	"margin":           {},
	"margin-top":       {},
	"margin-right":     {},
	"margin-bottom":    {},
	"margin-left":      {},
	"padding":          {},
	"padding-top":      {},
	"padding-right":    {},
	"padding-bottom":   {},
	"padding-left":     {},
	"max-width":        {},
	"width":            {},
	"height":           {},
}

var articleTextStyleProperties = map[string]struct{}{
	"color":            {},
	"background-color": {},
	"font-size":        {},
	"font-family":      {},
}

var articleBlockStyleProperties = map[string]struct{}{
	"text-align":  {},
	"line-height": {},
}

func sanitizeArticleNode(node *nethtml.Node, baseURL string, maxAssets int) (string, []ArticleAsset) {
	sanitizer := newArticleHTMLSanitizer(baseURL, maxAssets)
	return strings.TrimSpace(sanitizer.renderChildren(node)), sanitizer.assets
}

func sanitizeArticleFragment(fragment string, baseURL string, maxAssets int) (string, []ArticleAsset) {
	sanitizer := newArticleHTMLSanitizer(baseURL, maxAssets)
	context := &nethtml.Node{Type: nethtml.ElementNode, Data: "div"}
	nodes, err := nethtml.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		return "", nil
	}
	var builder strings.Builder
	for _, node := range nodes {
		builder.WriteString(sanitizer.renderNode(node))
	}
	return strings.TrimSpace(builder.String()), sanitizer.assets
}

func newArticleHTMLSanitizer(baseURL string, maxAssets int) *articleHTMLSanitizer {
	parsed, _ := url.Parse(strings.TrimSpace(baseURL))
	if maxAssets <= 0 {
		maxAssets = defaultMaxAssets
	}
	return &articleHTMLSanitizer{
		baseURL:   parsed,
		seenAsset: map[string]struct{}{},
		maxAssets: maxAssets,
	}
}

func (s *articleHTMLSanitizer) renderNode(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	switch node.Type {
	case nethtml.TextNode:
		return stdhtml.EscapeString(node.Data)
	case nethtml.ElementNode:
		return s.renderElement(node)
	default:
		return s.renderChildren(node)
	}
}

func (s *articleHTMLSanitizer) renderElement(node *nethtml.Node) string {
	tag := strings.ToLower(node.Data)
	if _, dropped := articleDroppedTags[tag]; dropped || shouldDropArticleElement(node) {
		return ""
	}

	switch tag {
	case "br":
		return "<br>"
	case "img":
		return s.renderMediaElement(node, "image")
	case "video":
		return s.renderMediaElement(node, "video")
	case "audio":
		return s.renderMediaElement(node, "audio")
	case "iframe", "mp-common-videosnap", "mpvideosnap", "mpvideo", "qqmusic", "mpvoice":
		return s.renderWeChatMediaElement(node, tag)
	case "a":
		return s.renderLinkElement(node)
	}

	if _, ok := articleContainerTags[tag]; ok {
		children := s.renderChildren(node)
		if tag == "figcaption" && strings.TrimSpace(children) != "" {
			return `<p>` + children + `</p>`
		}
		if shouldWrapContainerAsParagraph(node, children) {
			return s.renderStyledParagraph(node, children)
		}
		return children
	}
	if _, ok := articleWrappedTags[tag]; !ok {
		return s.renderChildren(node)
	}

	children := s.renderChildren(node)
	if strings.TrimSpace(children) == "" && tag != "td" && tag != "th" {
		return ""
	}
	if isArticleTextBlockTag(tag) {
		children = wrapChildrenWithTextStyle(sanitizeArticleTextStyle(attrValue(node, "style")), children)
		return "<" + tag + renderStyleAttr(sanitizeArticleBlockStyle(attrValue(node, "style"))) + s.renderTableCellAttrs(node, tag) + ">" + children + "</" + tag + ">"
	}
	return "<" + tag + s.renderCommonAttrs(node) + s.renderTableCellAttrs(node, tag) + ">" + children + "</" + tag + ">"
}

func (s *articleHTMLSanitizer) renderChildren(node *nethtml.Node) string {
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(s.renderNode(child))
	}
	return builder.String()
}

func (s *articleHTMLSanitizer) renderLinkElement(node *nethtml.Node) string {
	children := s.renderChildren(node)
	if strings.TrimSpace(children) == "" {
		return ""
	}
	href := s.resolveURL(attrValue(node, "href"))
	if !isSafeArticleHref(href) {
		return children
	}
	return `<a href="` + escapeAttr(href) + `" target="_blank" rel="noreferrer">` + children + `</a>`
}

func (s *articleHTMLSanitizer) renderMediaElement(node *nethtml.Node, kind string) string {
	sourceURL := s.resolveMediaSource(node, kind)
	if sourceURL == "" {
		return ""
	}

	switch kind {
	case "image":
		s.collectAsset("image", sourceURL, node)
		return `<img src="` + escapeAttr(sourceURL) + `"` + s.renderTextAttr(node, "alt") + s.renderTextAttr(node, "title") + s.renderCommonAttrs(node) + `>`
	case "video", "audio":
		s.collectAsset(kind, sourceURL, node)
		return `<` + kind + ` src="` + escapeAttr(sourceURL) + `" controls` + s.renderCommonAttrs(node) + `></` + kind + `>`
	default:
		return ""
	}
}

func (s *articleHTMLSanitizer) renderWeChatMediaElement(node *nethtml.Node, tag string) string {
	switch tag {
	case "mpvoice":
		if sourceURL := s.resolveWeChatAudioSource(node); sourceURL != "" {
			if isDirectMediaURL(sourceURL, "audio") {
				s.collectAsset("audio", sourceURL, node)
				return `<audio src="` + escapeAttr(sourceURL) + `" controls` + s.renderCommonAttrs(node) + `></audio>`
			}
			return s.renderMediaLinkBlock("audio", "音频", sourceURL, "", node)
		}
		return s.renderTextOnlyMediaBlock("音频", node)
	case "qqmusic":
		if sourceURL := s.resolveURL(firstNonEmpty(attrValue(node, "url"), attrValue(node, "data-url"), attrValue(node, "music_url"))); sourceURL != "" {
			return s.renderMediaLinkBlock("audio", "音乐", sourceURL, "", node)
		}
		return s.renderTextOnlyMediaBlock("音乐", node)
	default:
		sourceURL := s.resolveWeChatVideoSource(node)
		coverURL := s.resolveWeChatVideoCover(node)
		var parts []string
		if sourceURL == "" {
			if coverURL != "" {
				s.collectAsset("image", coverURL, node)
				return s.renderCoverImageBlock(coverURL)
			}
			return s.renderTextOnlyMediaBlock("视频", node)
		}
		if isDirectMediaURL(sourceURL, "video") {
			if coverURL != "" {
				s.collectAsset("image", coverURL, node)
				parts = append(parts, s.renderCoverImageBlock(coverURL))
			}
			s.collectAsset("video", sourceURL, node)
			parts = append(parts, `<video src="`+escapeAttr(sourceURL)+`" controls`+s.renderCommonAttrs(node)+`></video>`)
			return strings.Join(parts, "")
		}
		if coverURL != "" {
			s.collectAsset("image", coverURL, node)
			return s.renderCoverImageBlock(coverURL)
		}
		return s.renderTextOnlyMediaBlock("视频", node)
	}
}

func (s *articleHTMLSanitizer) renderStyledParagraph(node *nethtml.Node, children string) string {
	blockStyle := sanitizeArticleBlockStyle(attrValue(node, "style"))
	textStyle := sanitizeArticleTextStyle(attrValue(node, "style"))
	return `<p` + renderStyleAttr(blockStyle) + `>` + wrapChildrenWithTextStyle(textStyle, children) + `</p>`
}

func (s *articleHTMLSanitizer) renderCoverImageBlock(sourceURL string) string {
	return `<p><img src="` + escapeAttr(sourceURL) + `"></p>`
}

func (s *articleHTMLSanitizer) renderMediaLinkBlock(kind string, label string, sourceURL string, coverURL string, node *nethtml.Node) string {
	text := firstNonEmpty(attrValue(node, "data-title"), attrValue(node, "title"), attrValue(node, "name"), "查看"+label)
	coverAttr := ""
	if coverURL != "" {
		s.collectAsset("image", coverURL, node)
		coverAttr = ` data-rich-media-cover="` + escapeAttr(coverURL) + `"`
	}
	return `<div data-rich-media-embed="true" data-rich-media-kind="` + escapeAttr(kind) + `" data-rich-media-src="` + escapeAttr(sourceURL) + `" data-rich-media-title="` + escapeAttr(text) + `"` + coverAttr + `><a href="` + escapeAttr(sourceURL) + `" target="_blank" rel="noreferrer">` + stdhtml.EscapeString(text) + `</a></div>`
}

func (s *articleHTMLSanitizer) renderTextOnlyMediaBlock(label string, node *nethtml.Node) string {
	text := firstNonEmpty(attrValue(node, "data-title"), attrValue(node, "title"), attrValue(node, "name"), nodeText(node), label)
	return `<p>` + stdhtml.EscapeString(text) + `</p>`
}

func (s *articleHTMLSanitizer) resolveMediaSource(node *nethtml.Node, kind string) string {
	for _, attr := range articleMediaSourceAttrs {
		if resolved := s.resolveMediaURL(attrValue(node, attr)); resolved != "" {
			return resolved
		}
	}
	if kind == "image" {
		for _, attr := range articleSourceSetAttrs {
			if resolved := s.resolveMediaURL(bestSourceSetCandidate(attrValue(node, attr))); resolved != "" {
				return resolved
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != nethtml.ElementNode || !strings.EqualFold(child.Data, "source") {
			continue
		}
		if resolved := s.resolveMediaURL(attrValue(child, "src")); resolved != "" {
			return resolved
		}
	}
	return ""
}

func (s *articleHTMLSanitizer) resolveWeChatVideoSource(node *nethtml.Node) string {
	for _, attr := range []string{"data-src", "data-url", "src", "url", "video_url", "data-video-url"} {
		if resolved := s.resolveMediaURL(attrValue(node, attr)); resolved != "" {
			return resolved
		}
		if resolved := s.resolveURL(attrValue(node, attr)); resolved != "" && isLikelyVideoEmbedURL(resolved) {
			return resolved
		}
	}
	if vid := firstNonEmpty(attrValue(node, "data-mpvid"), attrValue(node, "data-vid"), attrValue(node, "vid")); vid != "" {
		return "https://mp.weixin.qq.com/mp/readtemplate?t=pages/video_player_tmpl&auto=0&vid=" + url.QueryEscape(vid)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != nethtml.ElementNode {
			continue
		}
		if resolved := s.resolveWeChatVideoSource(child); resolved != "" {
			return resolved
		}
	}
	return ""
}

func (s *articleHTMLSanitizer) resolveWeChatVideoCover(node *nethtml.Node) string {
	for _, attr := range []string{"data-cover", "cover", "data-imgurl", "data-thumb", "data-src", "src"} {
		if resolved := s.resolveMediaURL(attrValue(node, attr)); resolved != "" && !isLikelyVideoEmbedURL(resolved) {
			return resolved
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != nethtml.ElementNode {
			continue
		}
		if strings.EqualFold(child.Data, "img") {
			if resolved := s.resolveMediaSource(child, "image"); resolved != "" {
				return resolved
			}
		}
		if resolved := s.resolveWeChatVideoCover(child); resolved != "" {
			return resolved
		}
	}
	return ""
}

func (s *articleHTMLSanitizer) resolveWeChatAudioSource(node *nethtml.Node) string {
	for _, attr := range []string{"src", "data-src", "voice_url", "data-voice-url", "audio_url", "music_url", "url", "data-url"} {
		if resolved := s.resolveMediaURL(attrValue(node, attr)); resolved != "" {
			return resolved
		}
		if resolved := s.resolveURL(attrValue(node, attr)); resolved != "" {
			return resolved
		}
	}
	fileID := firstNonEmpty(attrValue(node, "voice_encode_fileid"), attrValue(node, "data-fileid"), attrValue(node, "fileid"))
	if fileID != "" {
		return "https://mp.weixin.qq.com/cgi-bin/readtemplate?t=tmpl/audio_tmpl&voice_id=" + url.QueryEscape(fileID)
	}
	return ""
}

func (s *articleHTMLSanitizer) resolveMediaURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "data:image/") {
		return value
	}
	resolved := s.resolveURL(value)
	if strings.HasPrefix(strings.ToLower(resolved), "http://") || strings.HasPrefix(strings.ToLower(resolved), "https://") {
		return resolved
	}
	return ""
}

func (s *articleHTMLSanitizer) resolveURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "//") {
		scheme := "https"
		if s.baseURL != nil && s.baseURL.Scheme != "" {
			scheme = s.baseURL.Scheme
		}
		return scheme + ":" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	if s.baseURL == nil {
		return ""
	}
	return s.baseURL.ResolveReference(parsed).String()
}

func (s *articleHTMLSanitizer) collectAsset(kind string, sourceURL string, node *nethtml.Node) {
	if strings.HasPrefix(strings.ToLower(sourceURL), "data:") || len(s.assets) >= s.maxAssets {
		return
	}
	key := kind + ":" + sourceURL
	if _, exists := s.seenAsset[key]; exists {
		return
	}
	s.seenAsset[key] = struct{}{}
	s.assets = append(s.assets, ArticleAsset{
		Kind:      kind,
		SourceURL: sourceURL,
		Name:      articleAssetName(sourceURL, kind, node),
	})
}

func (s *articleHTMLSanitizer) renderCommonAttrs(node *nethtml.Node) string {
	if style := sanitizeArticleStyle(attrValue(node, "style")); style != "" {
		return ` style="` + escapeAttr(style) + `"`
	}
	return ""
}

func renderStyleAttr(style string) string {
	if style == "" {
		return ""
	}
	return ` style="` + escapeAttr(style) + `"`
}

func wrapChildrenWithTextStyle(style string, children string) string {
	if strings.TrimSpace(style) == "" || strings.TrimSpace(children) == "" {
		return children
	}
	return `<span style="` + escapeAttr(style) + `">` + children + `</span>`
}

func shouldWrapContainerAsParagraph(node *nethtml.Node, renderedChildren string) bool {
	if strings.TrimSpace(renderedChildren) == "" {
		return false
	}
	if sanitizeArticleBlockStyle(attrValue(node, "style")) == "" && sanitizeArticleTextStyle(attrValue(node, "style")) == "" {
		return false
	}
	return !hasDescendantArticleBlock(node)
}

func hasDescendantArticleBlock(node *nethtml.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != nethtml.ElementNode {
			continue
		}
		tag := strings.ToLower(child.Data)
		if isArticleTextBlockTag(tag) || tag == "img" || tag == "video" || tag == "audio" || tag == "iframe" || strings.HasPrefix(tag, "mp") {
			return true
		}
		if hasDescendantArticleBlock(child) {
			return true
		}
	}
	return false
}

func isArticleTextBlockTag(tag string) bool {
	switch tag {
	case "p", "h1", "h2", "h3", "blockquote", "li", "td", "th":
		return true
	default:
		return false
	}
}

func (s *articleHTMLSanitizer) renderTableCellAttrs(node *nethtml.Node, tag string) string {
	if tag != "td" && tag != "th" {
		return ""
	}
	attrs := ""
	if colspan := normalizePositiveIntAttr(attrValue(node, "colspan"), 20); colspan != "" {
		attrs += ` colspan="` + colspan + `"`
	}
	if rowspan := normalizePositiveIntAttr(attrValue(node, "rowspan"), 50); rowspan != "" {
		attrs += ` rowspan="` + rowspan + `"`
	}
	return attrs
}

func (s *articleHTMLSanitizer) renderTextAttr(node *nethtml.Node, key string) string {
	value := strings.TrimSpace(attrValue(node, key))
	if value == "" {
		return ""
	}
	return ` ` + key + `="` + escapeAttr(value) + `"`
}

func sanitizeArticleStyle(style string) string {
	return sanitizeArticleStyleByProperties(style, articleAllowedStyleProperties)
}

func sanitizeArticleTextStyle(style string) string {
	return sanitizeArticleStyleByProperties(style, articleTextStyleProperties)
}

func sanitizeArticleBlockStyle(style string) string {
	return sanitizeArticleStyleByProperties(style, articleBlockStyleProperties)
}

func sanitizeArticleStyleByProperties(style string, allowedProperties map[string]struct{}) string {
	var parts []string
	for _, declaration := range strings.Split(style, ";") {
		key, value, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if _, allowed := allowedProperties[key]; !allowed || !isSafeCSSValue(value) {
			continue
		}
		parts = append(parts, key+": "+value)
	}
	return strings.Join(parts, "; ")
}

func isSafeCSSValue(value string) bool {
	lower := strings.ToLower(value)
	if value == "" || len(value) > 240 {
		return false
	}
	if strings.ContainsAny(value, "<>{}") {
		return false
	}
	return !strings.Contains(lower, "url(") &&
		!strings.Contains(lower, "expression") &&
		!strings.Contains(lower, "javascript:") &&
		!strings.Contains(lower, "@import") &&
		!strings.Contains(lower, "behavior:")
}

func shouldDropArticleElement(node *nethtml.Node) bool {
	return hasClassOrIDToken(node,
		"comment",
		"advert",
		"recommend",
		"related",
		"share",
		"toolbar",
		"qrcode",
		"login",
		"copyright",
	)
}

func isSafeArticleHref(href string) bool {
	lower := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

func isDirectMediaURL(sourceURL string, kind string) bool {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	switch kind {
	case "video":
		if isLikelyDownloadableArticleVideoURL(parsed) {
			return true
		}
		return ext == ".mp4" || ext == ".webm" || ext == ".ogg" || ext == ".mov" || ext == ".m4v"
	case "audio":
		return ext == ".mp3" || ext == ".wav" || ext == ".ogg" || ext == ".m4a" || ext == ".aac" || ext == ".flac"
	default:
		return false
	}
}

func isLikelyDownloadableArticleVideoURL(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Host)
	path := strings.ToLower(parsed.Path)
	query := strings.ToLower(parsed.RawQuery)
	if strings.Contains(host, "findermp.video.qq.com") && strings.Contains(path, "stodownload") {
		return true
	}
	return strings.Contains(host, "video.qq.com") &&
		strings.Contains(path, "stodownload") &&
		strings.Contains(query, "encfilekey=")
}

func isLikelyVideoEmbedURL(sourceURL string) bool {
	lower := strings.ToLower(sourceURL)
	return strings.Contains(lower, "video") ||
		strings.Contains(lower, "mp.weixin.qq.com/mp/readtemplate") ||
		strings.Contains(lower, "vid=")
}

func bestSourceSetCandidate(sourceSet string) string {
	var bestSource string
	bestScore := -1
	for _, raw := range strings.Split(sourceSet, ",") {
		parts := strings.Fields(strings.TrimSpace(raw))
		if len(parts) == 0 {
			continue
		}
		score := sourceSetScore("")
		if len(parts) > 1 {
			score = sourceSetScore(parts[1])
		}
		if bestSource == "" || score >= bestScore {
			bestSource = parts[0]
			bestScore = score
		}
	}
	return bestSource
}

func sourceSetScore(value string) int {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 1
	}
	if strings.HasSuffix(value, "w") || strings.HasSuffix(value, "x") {
		if score, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(value, "w"), "x")); err == nil {
			return score
		}
	}
	return 1
}

func normalizePositiveIntAttr(value string, maxValue int) string {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number <= 0 || number > maxValue {
		return ""
	}
	return strconv.Itoa(number)
}

func articleAssetName(sourceURL string, kind string, node *nethtml.Node) string {
	if title := strings.TrimSpace(attrValue(node, "alt")); title != "" {
		return safeArticleAssetName(title, kind)
	}
	parsed, err := url.Parse(sourceURL)
	if err == nil {
		if name := strings.TrimSpace(filepath.Base(parsed.Path)); name != "" && name != "." && name != "/" {
			return safeArticleAssetName(name, kind)
		}
	}
	return fmt.Sprintf("article-%s", kind)
}

func safeArticleAssetName(value string, kind string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if value == "" {
		return fmt.Sprintf("article-%s%s", kind, defaultArticleAssetExtension(kind))
	}
	if len([]rune(value)) > 80 {
		value = string([]rune(value)[:80])
	}
	if filepath.Ext(value) == "" {
		value += defaultArticleAssetExtension(kind)
	}
	return value
}

func defaultArticleAssetExtension(kind string) string {
	switch kind {
	case "video":
		return ".mp4"
	case "audio":
		return ".mp3"
	default:
		return ""
	}
}

func escapeAttr(value string) string {
	return stdhtml.EscapeString(strings.TrimSpace(value))
}
