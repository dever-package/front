package article

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"net/url"
	"sort"
	"strings"

	nethtml "golang.org/x/net/html"
)

type toutiaoArticleCandidate struct {
	Title string
	HTML  string
	Score int
}

var toutiaoJSONMarkers = []string{
	"window.__INITIAL_STATE__",
	"window.__SSR_HYDRATED_DATA__",
	"window.__NEXT_DATA__",
	"window.__DATA__",
	"window._ROUTER_DATA",
	"__INITIAL_STATE__",
	"__SSR_HYDRATED_DATA__",
	"__NEXT_DATA__",
	"_ROUTER_DATA",
}

var toutiaoTitleKeys = []string{
	"title",
	"article_title",
	"articleTitle",
	"content_title",
	"contentTitle",
	"share_title",
	"shareTitle",
	"seo_title",
	"seoTitle",
}

var toutiaoContentKeys = []string{
	"content",
	"article_content",
	"articleContent",
	"content_html",
	"contentHtml",
	"html",
	"rich_content",
	"richContent",
	"body",
}

func isToutiaoHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "toutiao.com" ||
		strings.HasSuffix(host, ".toutiao.com") ||
		host == "toutiaoapi.com" ||
		strings.HasSuffix(host, ".toutiaoapi.com")
}

func parseToutiaoArticle(page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error) {
	title := resolveDocumentTitle(document)
	candidates := extractToutiaoScriptCandidates(page.SourceHTML, document, title)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	for _, candidate := range candidates {
		bodyHTML, assets := sanitizeArticleFragment(candidate.HTML, page.URL, maxAssets)
		if strings.TrimSpace(bodyHTML) == "" {
			continue
		}
		articleTitle := cleanTitle(firstNonEmpty(candidate.Title, title))
		return ImportedArticle{
			Title:     articleTitle,
			HTML:      mergeArticleTitle(articleTitle, bodyHTML),
			Assets:    assets,
			SourceURL: page.URL,
			Site:      "toutiao",
		}, nil
	}

	if content := findToutiaoArticleNode(document); content != nil {
		bodyHTML, assets := sanitizeArticleNode(content, page.URL, maxAssets)
		if strings.TrimSpace(bodyHTML) != "" {
			return ImportedArticle{
				Title:     title,
				HTML:      mergeArticleTitle(title, bodyHTML),
				Assets:    assets,
				SourceURL: page.URL,
				Site:      "toutiao",
			}, nil
		}
	}

	return ImportedArticle{}, fmt.Errorf("今日头条页面未找到正文结构")
}

func extractToutiaoScriptCandidates(sourceHTML string, document *nethtml.Node, fallbackTitle string) []toutiaoArticleCandidate {
	var candidates []toutiaoArticleCandidate
	for _, source := range collectToutiaoScriptSources(sourceHTML, document) {
		for _, jsonText := range extractToutiaoJSONBlocks(source) {
			var payload any
			decoder := json.NewDecoder(strings.NewReader(jsonText))
			decoder.UseNumber()
			if err := decoder.Decode(&payload); err != nil {
				continue
			}
			walkToutiaoPayload(payload, fallbackTitle, &candidates)
		}
	}
	return dedupeToutiaoCandidates(candidates)
}

func collectToutiaoScriptSources(sourceHTML string, document *nethtml.Node) []string {
	var sources []string
	var scripts []*nethtml.Node
	findAllElements(document, func(node *nethtml.Node) bool {
		return strings.EqualFold(node.Data, "script")
	}, &scripts)
	for _, script := range scripts {
		text := strings.TrimSpace(scriptText(script))
		if text == "" {
			continue
		}
		sources = append(sources, text)
		if decoded, ok := decodeToutiaoEncodedJSON(text); ok {
			sources = append(sources, decoded)
		}
	}
	if sourceHTML != "" {
		sources = append(sources, sourceHTML)
	}
	return sources
}

func extractToutiaoJSONBlocks(source string) []string {
	source = strings.TrimSpace(stdhtml.UnescapeString(source))
	if source == "" {
		return nil
	}

	var blocks []string
	if looksLikeJSONObject(source) {
		blocks = append(blocks, source)
	}
	if decoded, ok := decodeToutiaoEncodedJSON(source); ok && looksLikeJSONObject(decoded) {
		blocks = append(blocks, decoded)
	}
	for _, marker := range toutiaoJSONMarkers {
		blocks = append(blocks, extractJSONAfterMarker(source, marker)...)
	}
	return blocks
}

func walkToutiaoPayload(value any, inheritedTitle string, candidates *[]toutiaoArticleCandidate) {
	switch current := value.(type) {
	case map[string]any:
		title := firstNonEmpty(firstStringByKeys(current, toutiaoTitleKeys...), inheritedTitle)
		for _, key := range toutiaoContentKeys {
			content := stringFromAny(current[key])
			if candidateHTML := normalizeToutiaoArticleHTML(content); candidateHTML != "" {
				*candidates = append(*candidates, toutiaoArticleCandidate{
					Title: title,
					HTML:  candidateHTML,
					Score: scoreToutiaoArticleHTML(candidateHTML, title),
				})
			}
		}
		for _, child := range current {
			walkToutiaoPayload(child, title, candidates)
		}
	case []any:
		for _, child := range current {
			walkToutiaoPayload(child, inheritedTitle, candidates)
		}
	case string:
		text := strings.TrimSpace(current)
		if !strings.Contains(text, "{") || !strings.Contains(text, "content") {
			return
		}
		for _, jsonText := range extractToutiaoJSONBlocks(text) {
			var payload any
			decoder := json.NewDecoder(strings.NewReader(jsonText))
			decoder.UseNumber()
			if err := decoder.Decode(&payload); err == nil {
				walkToutiaoPayload(payload, inheritedTitle, candidates)
			}
		}
	}
}

func normalizeToutiaoArticleHTML(value string) string {
	text := strings.TrimSpace(stdhtml.UnescapeString(strings.ReplaceAll(value, `\/`, `/`)))
	if text == "" {
		return ""
	}
	if !looksLikeArticleBody(text) {
		return ""
	}
	if strings.Contains(text, "<") && strings.Contains(text, ">") {
		return text
	}

	var paragraphs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paragraphs = append(paragraphs, "<p>"+stdhtml.EscapeString(line)+"</p>")
		}
	}
	return strings.Join(paragraphs, "\n")
}

func looksLikeArticleBody(value string) bool {
	text := strings.TrimSpace(value)
	if len([]rune(stripHTMLTags(text))) < 80 {
		return false
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "<p") ||
		strings.Contains(lower, "<img") ||
		strings.Contains(lower, "<section") ||
		strings.Contains(lower, "<article") ||
		strings.Count(text, "。")+strings.Count(text, "，")+strings.Count(text, "\n") >= 3
}

func scoreToutiaoArticleHTML(value string, title string) int {
	lower := strings.ToLower(value)
	score := len([]rune(stripHTMLTags(value)))
	score += strings.Count(lower, "<p") * 120
	score += strings.Count(lower, "<img") * 160
	score += strings.Count(lower, "<video") * 160
	score += strings.Count(lower, "<h1") * 100
	if strings.TrimSpace(title) != "" {
		score += 80
	}
	return score
}

func dedupeToutiaoCandidates(candidates []toutiaoArticleCandidate) []toutiaoArticleCandidate {
	seen := map[string]struct{}{}
	result := make([]toutiaoArticleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.TrimSpace(stripHTMLTags(candidate.HTML))
		if len([]rune(key)) > 160 {
			key = string([]rune(key)[:160])
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func findToutiaoArticleNode(document *nethtml.Node) *nethtml.Node {
	return findFirstElement(document, func(node *nethtml.Node) bool {
		if node == nil || node.Type != nethtml.ElementNode {
			return false
		}
		dataTestID := strings.ToLower(attrValue(node, "data-testid"))
		if dataTestID == "article" || dataTestID == "article-content" {
			return true
		}
		return hasClassOrIDToken(
			node,
			"article-content",
			"article_content",
			"articleContent",
			"syl-page-article",
			"tt-article-content",
			"detail-content",
			"content-wrapper",
		)
	})
}

func extractJSONAfterMarker(source string, marker string) []string {
	var result []string
	offset := 0
	for {
		index := strings.Index(source[offset:], marker)
		if index < 0 {
			break
		}
		index += offset + len(marker)
		jsonStart := findNextJSONObjectStart(source, index)
		if jsonStart < 0 {
			offset = index
			continue
		}
		if block, end := balancedJSONBlock(source, jsonStart); block != "" {
			result = append(result, block)
			offset = end
			continue
		}
		offset = jsonStart + 1
	}
	return result
}

func findNextJSONObjectStart(source string, start int) int {
	for index := start; index < len(source); index++ {
		switch source[index] {
		case '{', '[':
			return index
		case ';', '<':
			return -1
		}
	}
	return -1
}

func balancedJSONBlock(source string, start int) (string, int) {
	if start < 0 || start >= len(source) {
		return "", start
	}
	open := source[start]
	if open != '{' && open != '[' {
		return "", start
	}

	stack := []byte{open}
	inString := false
	escaped := false
	for index := start + 1; index < len(source); index++ {
		current := source[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == '"' {
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			continue
		}
		if current == '{' || current == '[' {
			stack = append(stack, current)
			continue
		}
		if current != '}' && current != ']' {
			continue
		}
		if len(stack) == 0 || !matchesJSONPair(stack[len(stack)-1], current) {
			return "", index
		}
		stack = stack[:len(stack)-1]
		if len(stack) == 0 {
			return source[start : index+1], index + 1
		}
	}
	return "", start
}

func matchesJSONPair(open byte, close byte) bool {
	return (open == '{' && close == '}') || (open == '[' && close == ']')
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

func decodeToutiaoEncodedJSON(value string) (string, bool) {
	text := strings.TrimSpace(value)
	lower := strings.ToLower(text)
	if text == "" || !strings.Contains(lower, "%7b") && !strings.Contains(lower, "%5b") {
		return "", false
	}
	decoded, err := url.QueryUnescape(text)
	if err != nil {
		return "", false
	}
	decoded = strings.TrimSpace(stdhtml.UnescapeString(decoded))
	return decoded, looksLikeJSONObject(decoded)
}

func looksLikeJSONObject(value string) bool {
	text := strings.TrimSpace(value)
	return strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")
}

func firstStringByKeys(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringFromAny(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch current := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(current)
	case json.Number:
		return current.String()
	case fmt.Stringer:
		return strings.TrimSpace(current.String())
	default:
		return ""
	}
}

func stripHTMLTags(value string) string {
	var builder strings.Builder
	inTag := false
	for _, char := range value {
		switch char {
		case '<':
			inTag = true
		case '>':
			inTag = false
			builder.WriteRune(' ')
		default:
			if !inTag {
				builder.WriteRune(char)
			}
		}
	}
	return strings.Join(strings.Fields(stdhtml.UnescapeString(builder.String())), " ")
}
