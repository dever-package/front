package webcontent

import "github.com/dever-package/front/service/webcontent/provider"

const (
	PlatformWeChat = provider.PlatformWeChat
	PlatformDouyin = provider.PlatformDouyin

	ContentTypeRichText = provider.ContentTypeRichText
	ContentTypeVideo    = provider.ContentTypeVideo

	MediaKindImage = provider.MediaKindImage
	MediaKindAudio = provider.MediaKindAudio
	MediaKindVideo = provider.MediaKindVideo
)

type ResolveInput = provider.ResolveInput
type ResolvedContent = provider.ResolvedContent
type Author = provider.Author
type Media = provider.Media

type DetectedSource struct {
	Platform string
	URL      string
}
