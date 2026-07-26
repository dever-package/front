package upload

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	frontmodel "github.com/dever-package/front/model"
)

const (
	uploadReadableSourceMaxBytes = 240
	uploadReadableNameMaxBytes   = 240
)

type resolvedUploadProviderTarget struct {
	PathMode   string
	ObjectKeys []string
}

func (target resolvedUploadProviderTarget) PrimaryObjectKey() string {
	if len(target.ObjectKeys) == 0 {
		return ""
	}
	return strings.TrimSpace(target.ObjectKeys[0])
}

func resolveUploadStoragePathMode(storage resolvedUploadStorage) string {
	pathMode := frontmodel.NormalizeUploadStoragePathMode(strings.ToLower(strings.TrimSpace(storage.PathMode)))
	if pathMode != frontmodel.UploadStoragePathModeAuto {
		return pathMode
	}
	if strings.EqualFold(strings.TrimSpace(storage.Type), "feishu") {
		return frontmodel.UploadStoragePathModeReadable
	}
	return frontmodel.UploadStoragePathModeHash
}

func resolveUploadProviderTarget(
	storage resolvedUploadStorage,
	logicalObjectKey string,
	sourceName string,
	nameCandidates []string,
) resolvedUploadProviderTarget {
	pathMode := resolveUploadStoragePathMode(storage)
	if pathMode != frontmodel.UploadStoragePathModeReadable || strings.EqualFold(strings.TrimSpace(storage.Type), "feishu") {
		return resolvedUploadProviderTarget{
			PathMode:   pathMode,
			ObjectKeys: []string{strings.TrimSpace(logicalObjectKey)},
		}
	}

	providerType := strings.ToLower(strings.TrimSpace(storage.Type))
	keys := make([]string, 0, len(nameCandidates))
	for _, name := range nameCandidates {
		key := buildReadableUploadProviderKey(providerType, sourceName, name)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		keys = append(keys, strings.TrimSpace(logicalObjectKey))
	}
	return resolvedUploadProviderTarget{PathMode: pathMode, ObjectKeys: keys}
}

func buildReadableUploadProviderKey(providerType, sourceName, fileName string) string {
	sourceSegment := sanitizeUploadPathSegment(sourceName, "未分类", uploadReadableSourceMaxBytes)
	fileSegment := sanitizeUploadDisplayName(fileName, "")
	fileSegment = trimUploadProviderFileName(fileSegment, uploadReadableNameMaxBytes)
	if strings.EqualFold(strings.TrimSpace(providerType), "local") {
		return path.Join("upload", sourceSegment, fileSegment)
	}
	return path.Join(sourceSegment, fileSegment)
}

func trimUploadProviderFileName(name string, maxBytes int) string {
	if len(name) <= maxBytes {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	suffixIndex := strings.LastIndex(stem, "__")
	if suffixIndex > 0 {
		suffix := stem[suffixIndex:]
		prefixMaxBytes := maxBytes - len(suffix) - len(ext)
		if prefixMaxBytes > 0 {
			return trimUTF8Bytes(stem[:suffixIndex], prefixMaxBytes) + suffix + ext
		}
	}
	stemMaxBytes := maxBytes - len(ext)
	if stemMaxBytes <= 0 {
		return trimUTF8Bytes(name, maxBytes)
	}
	return trimUTF8Bytes(stem, stemMaxBytes) + ext
}

func sanitizeUploadPathSegment(value, fallback string, maxBytes int) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case unicode.IsControl(char):
			continue
		case strings.ContainsRune(`\\/:*?"<>|`, char):
			builder.WriteRune('_')
		default:
			builder.WriteRune(char)
		}
	}
	result := strings.Trim(strings.TrimSpace(builder.String()), ".")
	if result == "" {
		result = fallback
	}
	return trimUTF8Bytes(result, maxBytes)
}

func trimUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
