package repository

import (
	"path/filepath"
	"strings"

	"github.com/shemic/dever/util"
)

func NormalizeHash(value any) string {
	hash := strings.ToLower(util.ToStringTrimmed(value))
	hash = strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'f':
			return r
		default:
			return -1
		}
	}, hash)
	if len(hash) > 32 {
		hash = hash[:32]
	}
	if len(hash) < 6 {
		return ""
	}
	return hash
}

func NormalizeBizKey(value any) string {
	return strings.ToLower(strings.TrimSpace(util.ToStringTrimmed(value)))
}

func NormalizeBizName(value any) string {
	return strings.TrimSpace(util.ToStringTrimmed(value))
}

func NormalizeRelationID(value any) uint64 {
	return util.ToUint64(value)
}

func ResolveKind(kind, fileName, mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image", "video", "audio", "file", "other":
		return strings.ToLower(strings.TrimSpace(kind))
	}

	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	}

	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".mp4", ".mov", ".avi", ".mkv", ".webm":
		return "video"
	case ".mp3", ".wav", ".m4a", ".aac", ".ogg":
		return "audio"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".zip", ".rar":
		return "file"
	default:
		return "file"
	}
}

func MergeAcceptTypes(items []UploadAcceptType) string {
	if len(items) == 0 {
		return ""
	}

	tokens := make([]string, 0, len(items)*2)
	seen := map[string]struct{}{}
	for _, item := range items {
		accept := strings.TrimSpace(item.Accept)
		if accept == "" || accept == "*" || accept == "*/*" {
			return ""
		}
		for _, token := range SplitAccept(accept) {
			if _, exists := seen[token]; exists {
				continue
			}
			seen[token] = struct{}{}
			tokens = append(tokens, token)
		}
	}
	return strings.Join(tokens, ",")
}

func CollectAcceptTypeIDs(items []UploadAcceptType) []uint64 {
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		if item.ID != 0 {
			result = append(result, item.ID)
		}
	}
	return util.UniqueUint64s(result)
}

func SplitAccept(accept string) []string {
	parts := strings.FieldsFunc(accept, func(r rune) bool {
		return r == ',' || r == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}
