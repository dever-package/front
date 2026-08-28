package douyin

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"net/url"
	"sort"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"

	"github.com/dever-package/front/service/webcontent/provider"
)

var douyinPayloadMarkers = []string{
	"window._SSR_DATA",
	"window._ROUTER_DATA",
	"window.__SSR_DATA__",
	"window.__INITIAL_STATE__",
}

type contentCandidate struct {
	id          string
	title       string
	text        string
	coverURL    string
	author      *provider.Author
	publishedAt *time.Time
	video       videoSource
	score       int64
}

type videoSource struct {
	url        string
	mimeType   string
	width      int
	height     int
	durationMS int64
	expiresAt  *time.Time
	score      int64
}

func parsePage(sourceHTML string, itemID string, sourceURL string) (provider.ResolvedContent, error) {
	var candidates []contentCandidate
	for _, payload := range extractPayloads(sourceHTML) {
		collectCandidates(payload, itemID, 0, &candidates)
	}
	if len(candidates) == 0 {
		return provider.ResolvedContent{}, fmt.Errorf("页面中未找到作品数据")
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].score > candidates[right].score
	})

	var selected contentCandidate
	for _, candidate := range candidates {
		if candidate.video.url != "" {
			selected = candidate
			break
		}
	}
	if selected.video.url == "" {
		return provider.ResolvedContent{}, fmt.Errorf("页面中未找到公开视频地址")
	}
	selected.title = strings.TrimSpace(selected.title)
	selected.text = strings.TrimSpace(selected.text)
	if selected.title == "" {
		selected.title = firstTextLine(selected.text)
	}
	if selected.title == "" {
		selected.title = "抖音作品"
	}
	if selected.text == "" {
		selected.text = selected.title
	}

	media := provider.Media{
		Kind:       provider.MediaKindVideo,
		SourceURL:  selected.video.url,
		Name:       selected.title,
		MIMEType:   selected.video.mimeType,
		Width:      selected.video.width,
		Height:     selected.video.height,
		DurationMS: selected.video.durationMS,
		ExpiresAt:  selected.video.expiresAt,
	}
	var cover *provider.Media
	if selected.coverURL != "" {
		cover = &provider.Media{
			Kind:      provider.MediaKindImage,
			SourceURL: selected.coverURL,
			Name:      selected.title + "封面",
		}
	}
	return provider.ResolvedContent{
		Platform:    provider.PlatformDouyin,
		ContentType: provider.ContentTypeVideo,
		ExternalID:  itemID,
		Title:       selected.title,
		Text:        selected.text,
		SourceURL:   sourceURL,
		Author:      selected.author,
		PublishedAt: selected.publishedAt,
		Cover:       cover,
		Media:       []provider.Media{media},
	}, nil
}

func extractPayloads(sourceHTML string) []any {
	var payloads []any
	for _, marker := range douyinPayloadMarkers {
		payloads = append(payloads, decodePayloadsAfterMarker(sourceHTML, marker)...)
	}

	document, err := nethtml.Parse(strings.NewReader(sourceHTML))
	if err != nil {
		return payloads
	}
	var scripts []*nethtml.Node
	findElements(document, func(node *nethtml.Node) bool {
		return node.Type == nethtml.ElementNode && strings.EqualFold(node.Data, "script")
	}, &scripts)
	for _, script := range scripts {
		text := strings.TrimSpace(scriptText(script))
		if text == "" {
			continue
		}
		if strings.EqualFold(attribute(script, "id"), "RENDER_DATA") {
			if decoded, err := url.QueryUnescape(text); err == nil {
				text = decoded
			}
		}
		scriptType := strings.ToLower(attribute(script, "type"))
		if scriptType == "application/ld+json" || looksLikeJSON(text) {
			if payload, ok := decodeJSON(text); ok {
				payloads = append(payloads, payload)
			}
		}
	}
	return payloads
}

func decodePayloadsAfterMarker(source string, marker string) []any {
	var result []any
	offset := 0
	for {
		index := strings.Index(source[offset:], marker)
		if index < 0 {
			return result
		}
		start := offset + index + len(marker)
		jsonStart := strings.IndexAny(source[start:], "{[")
		if jsonStart < 0 {
			return result
		}
		jsonStart += start
		if payload, ok := decodeJSON(source[jsonStart:]); ok {
			result = append(result, payload)
		}
		offset = jsonStart + 1
	}
}

func decodeJSON(source string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(stdhtml.UnescapeString(source))))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, false
	}
	return payload, true
}

func collectCandidates(value any, itemID string, depth int, candidates *[]contentCandidate) {
	if depth > 64 {
		return
	}
	switch current := value.(type) {
	case map[string]any:
		if candidate, ok := candidateFromMap(current, itemID); ok {
			*candidates = append(*candidates, candidate)
		}
		for _, child := range current {
			collectCandidates(child, itemID, depth+1, candidates)
		}
	case []any:
		for _, child := range current {
			collectCandidates(child, itemID, depth+1, candidates)
		}
	case string:
		text := strings.TrimSpace(current)
		if !looksLikeJSON(text) {
			return
		}
		if payload, ok := decodeJSON(text); ok {
			collectCandidates(payload, itemID, depth+1, candidates)
		}
	}
}

func candidateFromMap(data map[string]any, itemID string) (contentCandidate, bool) {
	candidate := contentCandidate{
		id:       stringValue(mapValue(data, "gid", "aweme_id", "awemeId", "item_id", "itemId", "group_id", "groupId")),
		title:    stringValue(mapValue(data, "title", "share_title", "shareTitle", "name")),
		text:     stringValue(mapValue(data, "abstract", "desc", "description", "share_desc", "shareDesc")),
		coverURL: firstURL(mapValue(data, "cover_image_url", "coverImage", "cover_image", "cover", "origin_cover", "originCover", "thumbnailUrl")),
	}
	if candidate.title == "" {
		candidate.title = stringValue(mapValue(data, "desc"))
	}
	if candidate.text == "" {
		candidate.text = candidate.title
	}
	candidate.author = authorFromMap(data)
	candidate.publishedAt = publishedAtFromMap(data)
	candidate.video = videoFromMap(data)

	if candidate.id == "" && candidate.title == "" && candidate.text == "" {
		return contentCandidate{}, false
	}
	candidate.score = candidate.video.score
	if candidate.video.url != "" {
		candidate.score += 10_000_000_000_000
	}
	if candidate.id == itemID {
		candidate.score += 1_000_000_000_000_000
	} else if candidate.id != "" {
		candidate.score += 100_000_000_000
	}
	if candidate.text != "" {
		candidate.score += 10_000
	}
	if candidate.author != nil {
		candidate.score += 1_000
	}
	if candidate.coverURL != "" {
		candidate.score += 100
	}
	return candidate, true
}

func authorFromMap(data map[string]any) *provider.Author {
	value := mapValue(data, "media_user", "mediaUser", "author", "user")
	authorData, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	author := &provider.Author{
		ID:        stringValue(mapValue(authorData, "id", "uid", "sec_uid", "secUid")),
		Name:      stringValue(mapValue(authorData, "screen_name", "screenName", "nickname", "name")),
		AvatarURL: firstURL(mapValue(authorData, "avatar_url", "avatarUrl", "avatar_thumb", "avatarThumb", "avatar")),
	}
	if author.ID == "" && author.Name == "" && author.AvatarURL == "" {
		return nil
	}
	return author
}

func publishedAtFromMap(data map[string]any) *time.Time {
	value := mapValue(data, "publish_time", "publishTime", "create_time", "createTime", "uploadDate", "datePublished")
	if unixTime := int64Value(value); unixTime > 0 {
		publishedAt := time.Unix(unixTime, 0).UTC()
		return &publishedAt
	}
	text := stringValue(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			publishedAt := parsed.UTC()
			return &publishedAt
		}
	}
	return nil
}
