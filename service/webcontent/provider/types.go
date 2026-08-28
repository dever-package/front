package provider

import (
	"context"
	"net/url"
	"time"
)

const (
	PlatformWeChat = "wechat"
	PlatformDouyin = "douyin"

	ContentTypeRichText = "richtext"
	ContentTypeVideo    = "video"

	MediaKindImage = "image"
	MediaKindAudio = "audio"
	MediaKindVideo = "video"
)

type ResolveInput struct {
	Source     string `json:"source"`
	Platform   string `json:"platform,omitempty"`
	Credential string `json:"-"`
	MaxAssets  int    `json:"max_assets,omitempty"`
}

type ResolveRequest struct {
	URL        *url.URL
	Credential string
	MaxAssets  int
}

type ResolvedContent struct {
	Platform    string     `json:"platform"`
	ContentType string     `json:"content_type"`
	ExternalID  string     `json:"external_id,omitempty"`
	Title       string     `json:"title,omitempty"`
	Text        string     `json:"text,omitempty"`
	HTML        string     `json:"html,omitempty"`
	SourceURL   string     `json:"source_url"`
	Author      *Author    `json:"author,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Cover       *Media     `json:"cover,omitempty"`
	Media       []Media    `json:"media"`
}

type Author struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type Media struct {
	Kind       string     `json:"kind"`
	SourceURL  string     `json:"source_url"`
	Name       string     `json:"name,omitempty"`
	MIMEType   string     `json:"mime_type,omitempty"`
	Width      int        `json:"width,omitempty"`
	Height     int        `json:"height,omitempty"`
	DurationMS int64      `json:"duration_ms,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type Resolver interface {
	Platform() string
	Match(*url.URL) bool
	Resolve(context.Context, ResolveRequest) (ResolvedContent, error)
}
