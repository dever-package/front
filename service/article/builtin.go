package article

import (
	"context"
	"strings"

	"github.com/shemic/dever/server"

	"github.com/dever-package/front/service/internal/providerpayload"
)

type ArticleBuiltinService struct{}

func (ArticleBuiltinService) ProviderImportArticleURL(c *server.Context, params []any) any {
	payload := providerpayload.Map(params)
	input := ImportURLInput{
		URL:       providerpayload.Text(payload, "url", "source_url", "sourceUrl"),
		MaxImages: providerpayload.Int(payload, 0, "max_images", "maxImages"),
	}
	article, err := ImportURLContent(providerContext(c), input)
	if err != nil {
		panic(err)
	}
	return map[string]any{
		"title":      strings.TrimSpace(article.Title),
		"html":       article.HTML,
		"rich":       article.HTML,
		"content":    article.HTML,
		"assets":     article.Assets,
		"source_url": article.SourceURL,
		"sourceUrl":  article.SourceURL,
		"site":       article.Site,
		"summary":    articleImportSummary(article),
	}
}

func providerContext(c *server.Context) context.Context {
	if c == nil {
		return context.Background()
	}
	return c.Context()
}

func articleImportSummary(article ImportedArticle) string {
	title := strings.TrimSpace(article.Title)
	if title == "" {
		title = "文章"
	}
	return title + " 解析完成"
}
