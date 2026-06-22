package article

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"net/url"
	"strings"

	nethtml "golang.org/x/net/html"
)

func parseArticlePage(page fetchedArticlePage, maxAssets int) (ImportedArticle, error) {
	document, err := parseHTMLDocument(page.SourceHTML)
	if err != nil {
		return ImportedArticle{}, fmt.Errorf("解析文章 HTML 失败: %w", err)
	}
	parsedURL, _ := url.Parse(page.URL)
	host := strings.ToLower(parsedURL.Hostname())

	switch {
	case strings.Contains(host, "mp.weixin.qq.com"):
		return parseWeChatArticle(page, document, maxAssets)
	case strings.Contains(host, "baijiahao.baidu.com"):
		if article, err := parseBaijiahaoArticle(page, document, maxAssets); err == nil {
			return article, nil
		}
	case isToutiaoHost(host):
		if article, err := parseToutiaoArticle(page, document, maxAssets); err == nil {
			return article, nil
		}
	}
	return parseGenericArticle(page, document, maxAssets)
}

func parseWeChatArticle(page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error) {
	title := cleanTitle(nodeText(findElementByID(document, "activity-name")))
	if title == "" {
		title = resolveDocumentTitle(document)
	}
	content := findElementByID(document, "js_content")
	if content == nil {
		if isWeChatVerifyPage(page.SourceHTML) {
			return ImportedArticle{}, fmt.Errorf("微信返回环境验证页，服务端无法直接读取正文；可先在浏览器打开文章后复制粘贴正文")
		}
		return parseGenericArticle(page, document, maxAssets)
	}

	bodyHTML, assets := sanitizeArticleNode(content, page.URL, maxAssets)
	return ImportedArticle{
		Title:     title,
		HTML:      mergeArticleTitle(title, bodyHTML),
		Assets:    assets,
		SourceURL: page.URL,
		Site:      "wechat",
	}, nil
}

func isWeChatVerifyPage(source string) bool {
	return strings.Contains(source, "环境异常") &&
		strings.Contains(source, "完成验证后即可继续访问")
}

func parseBaijiahaoArticle(page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error) {
	payload, err := extractBaijiahaoPayload(page.SourceHTML)
	if err != nil {
		return ImportedArticle{}, err
	}

	var data baijiahaoPayload
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return ImportedArticle{}, fmt.Errorf("解析百家号页面数据失败: %w", err)
	}

	title := resolveDocumentTitle(document)
	var fragments []string
	for _, item := range data.BsData.Superlanding {
		if item.ItemType != "" && !strings.EqualFold(item.ItemType, "article") {
			continue
		}
		if item.ItemData.Header != "" {
			title = cleanTitle(item.ItemData.Header)
		}
		for _, section := range item.ItemData.Sections {
			fragments = append(fragments, baijiahaoSectionHTML(section))
		}
	}
	if len(fragments) == 0 {
		return ImportedArticle{}, fmt.Errorf("百家号页面未找到正文结构")
	}

	bodyHTML, assets := sanitizeArticleFragment(strings.Join(fragments, "\n"), page.URL, maxAssets)
	return ImportedArticle{
		Title:     title,
		HTML:      mergeArticleTitle(title, bodyHTML),
		Assets:    assets,
		SourceURL: page.URL,
		Site:      "baijiahao",
	}, nil
}

func parseGenericArticle(page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error) {
	title := resolveDocumentTitle(document)
	content := findGenericArticleNode(document)
	if content == nil {
		return ImportedArticle{}, fmt.Errorf("未找到文章正文")
	}

	bodyHTML, assets := sanitizeArticleNode(content, page.URL, maxAssets)
	return ImportedArticle{
		Title:     title,
		HTML:      mergeArticleTitle(title, bodyHTML),
		Assets:    assets,
		SourceURL: page.URL,
		Site:      "generic",
	}, nil
}

func mergeArticleTitle(title string, bodyHTML string) string {
	title = cleanTitle(title)
	bodyHTML = strings.TrimSpace(bodyHTML)
	if title == "" {
		return bodyHTML
	}
	if strings.Contains(strings.ToLower(bodyHTML), "<h1") && strings.Contains(bodyHTML, title) {
		return bodyHTML
	}
	return `<h1>` + stdhtml.EscapeString(title) + `</h1>` + "\n" + bodyHTML
}

func findGenericArticleNode(document *nethtml.Node) *nethtml.Node {
	if node := findElementByID(document, "js_content"); node != nil {
		return node
	}
	if node := findFirstElement(document, func(current *nethtml.Node) bool {
		value := strings.ToLower(attrValue(current, "data-testid"))
		return value == "article" || value == "article-content"
	}); node != nil {
		return node
	}
	if node := findFirstElementByTag(document, "article", "main"); node != nil {
		return node
	}
	return highestScoredArticleNode(document)
}

func highestScoredArticleNode(document *nethtml.Node) *nethtml.Node {
	var candidates []*nethtml.Node
	findAllElements(document, func(node *nethtml.Node) bool {
		switch strings.ToLower(node.Data) {
		case "div", "section", "article", "main", "body":
			return !hasClassOrIDToken(node, "comment", "advert", "recommend", "related", "share", "toolbar", "login", "qrcode")
		default:
			return false
		}
	}, &candidates)

	var best *nethtml.Node
	bestScore := 0
	for _, candidate := range candidates {
		score := articleCandidateScore(candidate)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func articleCandidateScore(node *nethtml.Node) int {
	textLength := len([]rune(nodeText(node)))
	if textLength < 80 {
		return 0
	}
	score := textLength
	score += countDescendantTags(node, "p") * 120
	score += countDescendantTags(node, "img") * 160
	score -= linkTextLength(node) * 2
	switch strings.ToLower(node.Data) {
	case "body":
		score -= 300
	case "article", "main":
		score += 400
	}
	return score
}

func countDescendantTags(node *nethtml.Node, tag string) int {
	count := 0
	var visit func(*nethtml.Node)
	visit = func(current *nethtml.Node) {
		if current.Type == nethtml.ElementNode && strings.EqualFold(current.Data, tag) {
			count++
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return count
}

func linkTextLength(node *nethtml.Node) int {
	total := 0
	var visit func(*nethtml.Node)
	visit = func(current *nethtml.Node) {
		if current.Type == nethtml.ElementNode && strings.EqualFold(current.Data, "a") {
			total += len([]rune(nodeText(current)))
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return total
}

type baijiahaoPayload struct {
	BsData struct {
		Superlanding []struct {
			ItemType string `json:"itemType"`
			ItemData struct {
				Header   string                 `json:"header"`
				Sections []baijiahaoSectionData `json:"sections"`
			} `json:"itemData"`
		} `json:"superlanding"`
	} `json:"bsData"`
}

type baijiahaoSectionData struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	DataHTML string `json:"data_html"`
	Text     string `json:"text"`
	Link     string `json:"link"`
	Src      string `json:"src"`
	URL      string `json:"url"`
}

func baijiahaoSectionHTML(section baijiahaoSectionData) string {
	switch strings.ToLower(section.Type) {
	case "img", "image":
		source := firstNonEmpty(section.Link, section.Src, section.URL)
		if source == "" {
			return ""
		}
		return `<p><img src="` + stdhtml.EscapeString(source) + `"></p>`
	default:
		if section.DataHTML != "" {
			return section.DataHTML
		}
		content := firstNonEmpty(section.Content, section.Text)
		if content == "" {
			return ""
		}
		return `<p>` + stdhtml.EscapeString(content) + `</p>`
	}
}

func extractBaijiahaoPayload(source string) (string, error) {
	for _, marker := range []string{"window.jsonData", "jsonData"} {
		index := strings.Index(source, marker)
		if index < 0 {
			continue
		}
		if payload := extractBalancedJSONObject(source[index:]); payload != "" {
			return payload, nil
		}
	}
	return "", fmt.Errorf("百家号页面未找到结构化正文数据")
}

func extractBalancedJSONObject(source string) string {
	start := strings.Index(source, "{")
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for index, current := range source[start:] {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch current {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch current {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : start+index+1]
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
