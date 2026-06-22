package article

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"sort"
	"strings"

	nethtml "golang.org/x/net/html"
)

type structuredArticleCandidate struct {
	Title  string
	HTML   string
	Images []string
	Score  int
}

var structuredJSONMarkers = []string{
	"window.__INITIAL_STATE__",
	"window.__INITIAL_DATA__",
	"window.__PRELOADED_STATE__",
	"window.__APOLLO_STATE__",
	"window.__NUXT__",
	"window.__NEXT_DATA__",
	"window.__SSR_HYDRATED_DATA__",
	"__INITIAL_STATE__",
	"__INITIAL_DATA__",
	"__PRELOADED_STATE__",
	"__APOLLO_STATE__",
	"__NUXT__",
	"__NEXT_DATA__",
	"__SSR_HYDRATED_DATA__",
}

var structuredTitleKeys = []string{
	"headline",
	"name",
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

var structuredContentKeys = []string{
	"articleBody",
	"article_body",
	"content",
	"article_content",
	"articleContent",
	"content_html",
	"contentHtml",
	"html",
	"rich_content",
	"richContent",
	"body",
	"text",
	"markdown",
}

func parseStructuredArticle(page fetchedArticlePage, document *nethtml.Node, maxAssets int, site string) (ImportedArticle, error) {
	title := resolveDocumentTitle(document)
	candidates := extractStructuredArticleCandidates(page.SourceHTML, document, title)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	for _, candidate := range candidates {
		articleTitle := cleanTitle(firstNonEmpty(candidate.Title, title))
		bodyHTML := normalizeStructuredArticleHTML(candidate.HTML, candidate.Images)
		if bodyHTML == "" {
			continue
		}
		bodyHTML, assets := sanitizeArticleFragment(bodyHTML, page.URL, maxAssets)
		if strings.TrimSpace(bodyHTML) == "" {
			continue
		}
		return ImportedArticle{
			Title:     articleTitle,
			HTML:      mergeArticleTitle(articleTitle, bodyHTML),
			Assets:    assets,
			SourceURL: page.URL,
			Site:      site,
		}, nil
	}

	return ImportedArticle{}, fmt.Errorf("未找到结构化文章正文")
}

func extractStructuredArticleCandidates(sourceHTML string, document *nethtml.Node, fallbackTitle string) []structuredArticleCandidate {
	var candidates []structuredArticleCandidate
	for _, jsonText := range extractJSONLDScripts(document) {
		walkStructuredJSON(jsonText, fallbackTitle, 600, &candidates)
	}
	for _, source := range collectStructuredScriptSources(sourceHTML, document) {
		for _, jsonText := range extractStructuredJSONBlocks(source) {
			walkStructuredJSON(jsonText, fallbackTitle, 0, &candidates)
		}
	}
	return dedupeStructuredCandidates(candidates)
}

func extractJSONLDScripts(document *nethtml.Node) []string {
	var scripts []*nethtml.Node
	findAllElements(document, func(node *nethtml.Node) bool {
		if !strings.EqualFold(node.Data, "script") {
			return false
		}
		return strings.Contains(strings.ToLower(attrValue(node, "type")), "ld+json")
	}, &scripts)

	var sources []string
	for _, script := range scripts {
		if text := strings.TrimSpace(scriptText(script)); text != "" {
			sources = append(sources, text)
		}
	}
	return sources
}

func collectStructuredScriptSources(sourceHTML string, document *nethtml.Node) []string {
	var sources []string
	var scripts []*nethtml.Node
	findAllElements(document, func(node *nethtml.Node) bool {
		return strings.EqualFold(node.Data, "script")
	}, &scripts)
	for _, script := range scripts {
		text := strings.TrimSpace(scriptText(script))
		if !hasStructuredJSONHint(text) {
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

func hasStructuredJSONHint(source string) bool {
	text := strings.TrimSpace(source)
	if text == "" {
		return false
	}
	if looksLikeJSONObject(text) {
		return true
	}
	lower := strings.ToLower(text)
	for _, marker := range structuredJSONMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return strings.Contains(lower, "articlebody") ||
		strings.Contains(lower, "article_content") ||
		strings.Contains(lower, "contenthtml")
}

func extractStructuredJSONBlocks(source string) []string {
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
	for _, marker := range structuredJSONMarkers {
		blocks = append(blocks, extractJSONAfterMarker(source, marker)...)
	}
	return blocks
}

func walkStructuredJSON(jsonText string, inheritedTitle string, boost int, candidates *[]structuredArticleCandidate) {
	var payload any
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return
	}
	walkStructuredPayload(payload, inheritedTitle, boost, candidates)
}

func walkStructuredPayload(value any, inheritedTitle string, boost int, candidates *[]structuredArticleCandidate) {
	switch current := value.(type) {
	case map[string]any:
		title := firstNonEmpty(firstStringByKeys(current, structuredTitleKeys...), inheritedTitle)
		images := extractStructuredImages(current["image"])
		images = append(images, extractStructuredImages(current["images"])...)
		images = append(images, extractStructuredImages(current["thumbnail"])...)
		images = append(images, extractStructuredImages(current["thumbnailUrl"])...)
		for _, key := range structuredContentKeys {
			content := stringFromAny(current[key])
			bodyHTML := normalizeStructuredArticleHTML(content, images)
			if bodyHTML == "" {
				continue
			}
			*candidates = append(*candidates, structuredArticleCandidate{
				Title:  title,
				HTML:   bodyHTML,
				Images: images,
				Score:  boost + scoreStructuredArticleHTML(bodyHTML, title, images),
			})
		}
		for _, child := range current {
			walkStructuredPayload(child, title, boost, candidates)
		}
	case []any:
		for _, child := range current {
			walkStructuredPayload(child, inheritedTitle, boost, candidates)
		}
	case string:
		text := strings.TrimSpace(current)
		if !strings.Contains(text, "{") || !hasStructuredJSONHint(text) {
			return
		}
		for _, jsonText := range extractStructuredJSONBlocks(text) {
			walkStructuredJSON(jsonText, inheritedTitle, boost, candidates)
		}
	}
}

func normalizeStructuredArticleHTML(value string, images []string) string {
	text := strings.TrimSpace(stdhtml.UnescapeString(strings.ReplaceAll(value, `\/`, `/`)))
	if text == "" || !looksLikeArticleBody(text) {
		return ""
	}

	var bodyHTML string
	if strings.Contains(text, "<") && strings.Contains(text, ">") {
		bodyHTML = text
	} else {
		bodyHTML = plainTextArticleHTML(text)
	}

	if strings.Contains(strings.ToLower(bodyHTML), "<img") {
		return bodyHTML
	}
	imageHTML := structuredImageHTML(images)
	if imageHTML == "" {
		return bodyHTML
	}
	return imageHTML + "\n" + bodyHTML
}

func plainTextArticleHTML(value string) string {
	var paragraphs []string
	for _, block := range strings.Split(value, "\n") {
		block = strings.TrimSpace(block)
		if block != "" {
			paragraphs = append(paragraphs, "<p>"+stdhtml.EscapeString(block)+"</p>")
		}
	}
	if len(paragraphs) == 0 {
		paragraphs = append(paragraphs, "<p>"+stdhtml.EscapeString(value)+"</p>")
	}
	return strings.Join(paragraphs, "\n")
}

func structuredImageHTML(images []string) string {
	var builder strings.Builder
	for _, imageURL := range dedupeStructuredStrings(images) {
		if !strings.HasPrefix(strings.ToLower(imageURL), "http://") &&
			!strings.HasPrefix(strings.ToLower(imageURL), "https://") {
			continue
		}
		builder.WriteString(`<p><img src="`)
		builder.WriteString(stdhtml.EscapeString(imageURL))
		builder.WriteString(`"></p>`)
		builder.WriteByte('\n')
	}
	return strings.TrimSpace(builder.String())
}

func scoreStructuredArticleHTML(value string, title string, images []string) int {
	lower := strings.ToLower(value)
	score := len([]rune(stripHTMLTags(value)))
	score += strings.Count(lower, "<p") * 100
	score += strings.Count(lower, "<section") * 60
	score += strings.Count(lower, "<img") * 140
	score += len(images) * 40
	if strings.TrimSpace(title) != "" {
		score += 80
	}
	return score
}

func dedupeStructuredCandidates(candidates []structuredArticleCandidate) []structuredArticleCandidate {
	seen := map[string]struct{}{}
	result := make([]structuredArticleCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.TrimSpace(stripHTMLTags(candidate.HTML))
		if len([]rune(key)) > 180 {
			key = string([]rune(key)[:180])
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

func extractStructuredImages(value any) []string {
	var images []string
	switch current := value.(type) {
	case nil:
		return nil
	case string:
		if text := strings.TrimSpace(current); text != "" {
			images = append(images, text)
		}
	case []any:
		for _, item := range current {
			images = append(images, extractStructuredImages(item)...)
		}
	case map[string]any:
		for _, key := range []string{"url", "contentUrl", "thumbnailUrl", "src"} {
			if imageURL := stringFromAny(current[key]); imageURL != "" {
				images = append(images, imageURL)
			}
		}
	}
	return dedupeStructuredStrings(images)
}

func dedupeStructuredStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
