package article

import (
	"fmt"
	"strings"

	nethtml "golang.org/x/net/html"
)

type articleSiteParser struct {
	Site          string
	AllowFallback bool
	Match         func(host string) bool
	Parse         func(page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error)
}

var articleSiteParsers = []articleSiteParser{
	{
		Site:          "wechat",
		AllowFallback: false,
		Match:         matchArticleHosts("mp.weixin.qq.com"),
		Parse:         parseWeChatArticle,
	},
	{
		Site:          "baijiahao",
		AllowFallback: true,
		Match:         matchArticleHosts("baijiahao.baidu.com"),
		Parse:         parseBaijiahaoArticle,
	},
	{
		Site:          "toutiao",
		AllowFallback: true,
		Match:         isToutiaoHost,
		Parse:         parseToutiaoArticle,
	},
	newKnownSiteArticleParser("zhihu", []string{"zhuanlan.zhihu.com"}, []string{
		"Post-RichText",
		"RichText",
		"article-content",
		"ContentItem-content",
	}),
	newKnownSiteArticleParser("juejin", []string{"juejin.cn"}, []string{
		"article-content",
		"markdown-body",
		"byted-editor-output",
		"entry-content",
	}),
	newKnownSiteArticleParser("jianshu", []string{"jianshu.com", "www.jianshu.com"}, []string{
		"article",
		"show-content",
		"note",
		"entry-content",
	}),
	newKnownSiteArticleParser("csdn", []string{"blog.csdn.net"}, []string{
		"article_content",
		"blog-content-box",
		"htmledit_views",
		"markdown_views",
	}),
	newKnownSiteArticleParser("cnblogs", []string{"cnblogs.com", "www.cnblogs.com"}, []string{
		"cnblogs_post_body",
		"postBody",
		"blogpost-body",
		"post",
	}),
	newKnownSiteArticleParser("sohu", []string{"www.sohu.com", "mp.sohu.com", "m.sohu.com", "news.sohu.com"}, []string{
		"mp-article-texts",
		"article-content",
		"content-article",
		"text",
	}),
	newKnownSiteArticleParser("netease", []string{"www.163.com", "news.163.com", "dy.163.com", "m.163.com"}, []string{
		"post_body",
		"post-content",
		"article-content",
		"content",
	}),
	newKnownSiteArticleParser("qq-news", []string{"new.qq.com", "view.inews.qq.com", "om.qq.com", "news.qq.com"}, []string{
		"article-content",
		"content-article",
		"qq_article",
		"rich_media_content",
	}),
}

func newKnownSiteArticleParser(site string, hosts []string, contentTokens []string) articleSiteParser {
	return articleSiteParser{
		Site:          site,
		AllowFallback: true,
		Match:         matchArticleHosts(hosts...),
		Parse: func(page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error) {
			return parseKnownSiteArticle(site, contentTokens, page, document, maxAssets)
		},
	}
}

func matchArticleHosts(hosts ...string) func(host string) bool {
	normalized := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			normalized = append(normalized, host)
		}
	}
	return func(host string) bool {
		host = strings.ToLower(strings.TrimSpace(host))
		for _, candidate := range normalized {
			if host == candidate || strings.HasSuffix(host, "."+candidate) {
				return true
			}
		}
		return false
	}
}

func parseKnownSiteArticle(site string, contentTokens []string, page fetchedArticlePage, document *nethtml.Node, maxAssets int) (ImportedArticle, error) {
	if article, err := parseStructuredArticle(page, document, maxAssets, site); err == nil {
		return article, nil
	}

	content := findKnownSiteArticleNode(document, contentTokens)
	if content == nil {
		return ImportedArticle{}, fmt.Errorf("%s 页面未找到正文结构", site)
	}
	title := resolveDocumentTitle(document)
	bodyHTML, assets := sanitizeArticleNode(content, page.URL, maxAssets)
	if strings.TrimSpace(bodyHTML) == "" {
		return ImportedArticle{}, fmt.Errorf("%s 页面正文为空", site)
	}
	return ImportedArticle{
		Title:     title,
		HTML:      mergeArticleTitle(title, bodyHTML),
		Assets:    assets,
		SourceURL: page.URL,
		Site:      site,
	}, nil
}

func findKnownSiteArticleNode(document *nethtml.Node, contentTokens []string) *nethtml.Node {
	var candidates []*nethtml.Node
	findAllElements(document, func(node *nethtml.Node) bool {
		if node == nil || node.Type != nethtml.ElementNode {
			return false
		}
		if strings.EqualFold(node.Data, "article") || strings.EqualFold(node.Data, "main") {
			return true
		}
		return hasClassOrIDToken(node, contentTokens...)
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
