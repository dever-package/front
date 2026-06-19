package upload

import (
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

func buildUploadObjectKey(ruleID uint64, hash, ext, bizKey string) string {
	hash = normalizeUploadHash(hash)
	if hash == "" {
		hash = normalizeUploadHash(strconv.FormatInt(time.Now().UnixNano(), 16))
	}
	bizSegment := normalizeUploadBizKeySegment(bizKey)
	return filepath.ToSlash(filepath.Join(
		"upload",
		strconv.FormatUint(ruleID, 10),
		bizSegment,
		hash[0:2],
		hash[2:4],
		hash[4:6],
		hash+ext,
	))
}

func normalizeUploadHash(value any) string {
	return uploadrepo.NormalizeHash(value)
}

func resolveUploadStorageProvider(storage resolvedUploadStorage) string {
	return strings.ToLower(strings.TrimSpace(storage.Type))
}

func normalizeUploadBizKeySegment(bizKey string) string {
	if strings.TrimSpace(bizKey) == "" {
		return "common"
	}

	segment := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}, strings.ToLower(strings.TrimSpace(bizKey)))

	segment = strings.Trim(segment, "-.")
	if segment == "" {
		return "common"
	}
	return segment
}

func resolveUploadKind(kind, fileName, mimeType string) string {
	return uploadrepo.ResolveKind(kind, fileName, mimeType)
}

func normalizeUploadChunkSize(size int64) int64 {
	if size <= 0 {
		return 2 * 1024 * 1024
	}
	return size
}

func uploadRuleChunkSizeBytes(rule resolvedUploadRule) int64 {
	return normalizeUploadChunkSize(normalizeUploadSizeMB(rule.ChunkSizeMB, 2) * uploadSizeMBUnit)
}

func uploadRuleMaxSizeBytes(rule resolvedUploadRule) int64 {
	return normalizeUploadSizeMB(rule.MaxSizeMB, 10) * uploadSizeMBUnit
}

func normalizeUploadSizeMB(size, fallback int64) int64 {
	if size <= 0 {
		return fallback
	}
	return size
}

func resolveUploadExt(fileName, mimeType string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if ext != "" {
		return ext
	}
	matches, err := mime.ExtensionsByType(strings.TrimSpace(mimeType))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(matches[0]))
}
