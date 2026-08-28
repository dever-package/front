package douyin

import (
	"sort"
	"strings"
)

func videoFromMap(data map[string]any) videoSource {
	var choices []videoSource
	if modelValue := mapValue(data, "video_model", "videoModel"); modelValue != nil {
		if model, ok := mapFromValue(modelValue); ok {
			if video := videoFromModel(model); video.url != "" {
				choices = append(choices, video)
			}
		}
	}
	if videoValue := mapValue(data, "video", "video_data", "videoData"); videoValue != nil {
		if videoData, ok := mapFromValue(videoValue); ok {
			if video := videoFromStandardMap(videoData); video.url != "" {
				choices = append(choices, video)
			}
		}
	}
	for _, key := range []string{"video_url", "videoUrl", "contentUrl", "content_url", "play_url", "playUrl"} {
		if sourceURL := firstURL(mapValue(data, key)); sourceURL != "" {
			choices = append(choices, videoSource{url: sourceURL, mimeType: "video/mp4", score: 1})
		}
	}
	if len(choices) == 0 {
		return videoSource{}
	}
	sort.SliceStable(choices, func(left, right int) bool {
		return choices[left].score > choices[right].score
	})
	return choices[0]
}

func videoFromModel(model map[string]any) videoSource {
	durationMS := secondsToMilliseconds(numberValue(mapValue(model, "video_duration", "videoDuration")))
	expiresAt := unixTimePointer(int64Value(mapValue(model, "url_expire", "urlExpire")))
	var choices []videoSource
	if values, ok := mapValue(model, "video_list", "videoList").([]any); ok {
		for _, value := range values {
			entry, ok := value.(map[string]any)
			if !ok {
				continue
			}
			video := videoListEntry(entry)
			if video.url == "" {
				continue
			}
			video.durationMS = durationMS
			video.expiresAt = expiresAt
			choices = append(choices, video)
		}
	}
	if len(choices) == 0 {
		if sourceURL := firstURL(mapValue(model, "fallback_api", "fallbackApi")); sourceURL != "" {
			return videoSource{
				url:        sourceURL,
				mimeType:   "video/mp4",
				durationMS: durationMS,
				expiresAt:  expiresAt,
				score:      1,
			}
		}
		return videoSource{}
	}
	sort.SliceStable(choices, func(left, right int) bool {
		return choices[left].score > choices[right].score
	})
	return choices[0]
}

func videoListEntry(entry map[string]any) videoSource {
	sourceURL := firstURL(mapValue(entry, "main_url", "mainUrl", "backup_url", "backupUrl"))
	if sourceURL == "" {
		return videoSource{}
	}
	meta, _ := mapValue(entry, "video_meta", "videoMeta").(map[string]any)
	codec := strings.ToLower(stringValue(mapValue(meta, "codec_type", "codecType", "codec")))
	container := strings.ToLower(stringValue(mapValue(meta, "vtype", "format")))
	width := int(numberValue(mapValue(meta, "vwidth", "width")))
	height := int(numberValue(mapValue(meta, "vheight", "height")))
	bitrate := int64(numberValue(mapValue(meta, "bitrate", "real_bitrate", "realBitrate")))

	score := int64(width*height)*1_000 + bitrate
	switch {
	case strings.Contains(codec, "264") || strings.Contains(codec, "avc"):
		score += 3_000_000_000_000
	case codec == "":
		score += 2_000_000_000_000
	default:
		score += 1_000_000_000_000
	}
	if container == "mp4" || strings.Contains(sourceURL, "mime_type=video_mp4") {
		score += 500_000_000_000
	}
	return videoSource{
		url:      sourceURL,
		mimeType: "video/mp4",
		width:    width,
		height:   height,
		score:    score,
	}
}

func videoFromStandardMap(data map[string]any) videoSource {
	for _, key := range []string{
		"play_addr", "playAddr", "play_addr_h264", "playAddrH264",
		"download_addr", "downloadAddr", "play_url", "playUrl",
	} {
		if sourceURL := firstURL(mapValue(data, key)); sourceURL != "" {
			return videoSource{
				url:        sourceURL,
				mimeType:   "video/mp4",
				width:      int(numberValue(mapValue(data, "width"))),
				height:     int(numberValue(mapValue(data, "height"))),
				durationMS: normalizeDurationMilliseconds(numberValue(mapValue(data, "duration"))),
				score:      2_000_000_000_000,
			}
		}
	}
	return videoSource{}
}
