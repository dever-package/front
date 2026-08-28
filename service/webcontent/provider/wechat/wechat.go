package wechat

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	nethtml "golang.org/x/net/html"

	"github.com/dever-package/front/service/article"
	"github.com/dever-package/front/service/webcontent/provider"
)

type Resolver struct{}

func New() Resolver {
	return Resolver{}
}

func (Resolver) Platform() string {
	return provider.PlatformWeChat
}

func (Resolver) Match(parsedURL *url.URL) bool {
	if parsedURL == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsedURL.Hostname()))
	return host == "mp.weixin.qq.com"
}

func (resolver Resolver) Resolve(ctx context.Context, request provider.ResolveRequest) (provider.ResolvedContent, error) {
	if request.URL == nil || !resolver.Match(request.URL) {
		return provider.ResolvedContent{}, fmt.Errorf("公众号文章链接无效")
	}
	imported, err := article.ImportURLContent(ctx, article.ImportURLInput{
		URL:       request.URL.String(),
		MaxImages: request.MaxAssets,
		Cookie:    request.Credential,
	})
	if err != nil {
		return provider.ResolvedContent{}, err
	}

	media := make([]provider.Media, 0, len(imported.Assets))
	for _, asset := range imported.Assets {
		kind := normalizeMediaKind(asset.Kind)
		if kind == "" || strings.TrimSpace(asset.SourceURL) == "" {
			continue
		}
		media = append(media, provider.Media{
			Kind:      kind,
			SourceURL: strings.TrimSpace(asset.SourceURL),
			Name:      strings.TrimSpace(asset.Name),
		})
	}

	return provider.ResolvedContent{
		Platform:    provider.PlatformWeChat,
		ContentType: provider.ContentTypeRichText,
		Title:       strings.TrimSpace(imported.Title),
		Text:        plainText(imported.HTML),
		HTML:        imported.HTML,
		SourceURL:   imported.SourceURL,
		Media:       media,
	}, nil
}

func normalizeMediaKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case provider.MediaKindImage:
		return provider.MediaKindImage
	case provider.MediaKindAudio:
		return provider.MediaKindAudio
	case provider.MediaKindVideo:
		return provider.MediaKindVideo
	default:
		return ""
	}
}

func plainText(sourceHTML string) string {
	tokenizer := nethtml.NewTokenizer(strings.NewReader(sourceHTML))
	var parts []string
	for {
		switch tokenizer.Next() {
		case nethtml.ErrorToken:
			return strings.Join(parts, " ")
		case nethtml.TextToken:
			text := strings.Join(strings.Fields(string(tokenizer.Text())), " ")
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
}
