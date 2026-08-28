package webcontent

import (
	"context"
	"fmt"
	stdhtml "html"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/dever-package/front/service/remoteurl"
	"github.com/dever-package/front/service/webcontent/provider"
	"github.com/dever-package/front/service/webcontent/provider/douyin"
	"github.com/dever-package/front/service/webcontent/provider/wechat"
)

var (
	sharedResolvers = []provider.Resolver{
		wechat.New(),
		douyin.New(),
	}
	sharedURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
)

func Resolve(ctx context.Context, input ResolveInput) (ResolvedContent, error) {
	parsedURL, err := sourceURL(input.Source)
	if err != nil {
		return ResolvedContent{}, err
	}
	if err := remoteurl.Validate(parsedURL); err != nil {
		return ResolvedContent{}, err
	}

	credential := strings.TrimSpace(input.Credential)
	if strings.ContainsAny(credential, "\r\n") {
		return ResolvedContent{}, fmt.Errorf("平台登录态格式无效")
	}
	resolver, err := selectResolver(strings.ToLower(strings.TrimSpace(input.Platform)), parsedURL)
	if err != nil {
		return ResolvedContent{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := resolver.Resolve(ctx, provider.ResolveRequest{
		URL:        parsedURL,
		Credential: credential,
		MaxAssets:  input.MaxAssets,
	})
	if err != nil {
		return ResolvedContent{}, err
	}
	result.Platform = resolver.Platform()
	if strings.TrimSpace(result.SourceURL) == "" {
		result.SourceURL = parsedURL.String()
	}
	if result.Media == nil {
		result.Media = []provider.Media{}
	}
	return result, nil
}

// DetectSources identifies and deduplicates every supported URL in pasted share text.
func DetectSources(source string) ([]DetectedSource, error) {
	parsedURLs, err := sourceURLs(source)
	if err != nil {
		return nil, err
	}
	result := make([]DetectedSource, 0, len(parsedURLs))
	for index, parsedURL := range parsedURLs {
		resolver, resolveErr := selectResolver("", parsedURL)
		if resolveErr != nil {
			return nil, fmt.Errorf("第%d个链接: %w", index+1, resolveErr)
		}
		result = append(result, DetectedSource{
			Platform: resolver.Platform(),
			URL:      parsedURL.String(),
		})
	}
	return result, nil
}

func selectResolver(platform string, parsedURL *url.URL) (provider.Resolver, error) {
	if platform != "" {
		for _, resolver := range sharedResolvers {
			if resolver.Platform() != platform {
				continue
			}
			if !resolver.Match(parsedURL) {
				return nil, fmt.Errorf("链接与指定平台不匹配")
			}
			return resolver, nil
		}
		return nil, fmt.Errorf("暂不支持该内容平台: %s", platform)
	}

	for _, resolver := range sharedResolvers {
		if resolver.Match(parsedURL) {
			return resolver, nil
		}
	}
	return nil, fmt.Errorf("暂不支持该内容链接")
}

func sourceURL(source string) (*url.URL, error) {
	parsedURLs, err := sourceURLs(source)
	if err != nil {
		return nil, err
	}
	return parsedURLs[0], nil
}

func sourceURLs(source string) ([]*url.URL, error) {
	source = stdhtml.UnescapeString(strings.TrimSpace(source))
	matches := sharedURLPattern.FindAllString(source, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("未找到可解析的内容链接")
	}
	result := make([]*url.URL, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		parsed, err := parseSourceURL(match)
		if err != nil {
			return nil, err
		}
		normalized := parsed.String()
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, parsed)
	}
	return result, nil
}

func parseSourceURL(source string) (*url.URL, error) {
	source = strings.TrimRightFunc(source, trailingSharePunctuation)
	parsed, err := url.Parse(source)
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("内容链接无效")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("内容链接不能包含账号信息")
	}
	return parsed, nil
}

func trailingSharePunctuation(current rune) bool {
	if unicode.IsSpace(current) {
		return true
	}
	return strings.ContainsRune(".,;:!?)]}，。；：！？）》】", current)
}
