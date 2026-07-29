package repository

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// FindUploadFileByReference resolves a public upload URL, an upload open URL,
// or a local public upload path back to its persisted upload record.
func FindUploadFileByReference(ctx context.Context, value string) (*UploadFile, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return nil, false
	}
	if fileID := uploadFileIDFromReference(parsed); fileID > 0 {
		file, findErr := FindUploadFile(ctx, fileID)
		if findErr != nil {
			return nil, false
		}
		return &file, true
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	isLocalPublicPath := scheme == "" && strings.HasPrefix(parsed.Path, "/upload/")
	if !isLocalPublicPath && scheme != "http" && scheme != "https" {
		return nil, false
	}
	objectPath := strings.TrimPrefix(strings.TrimSpace(parsed.Path), "/")
	if objectPath == "" {
		return nil, false
	}

	for _, candidate := range uploadReferencePathCandidates(objectPath) {
		file := FindUploadFileByPath(ctx, candidate)
		if file == nil {
			continue
		}
		if isLocalPublicPath || uploadReferenceURLMatches(*file, value) {
			return file, true
		}
	}
	return nil, false
}

func uploadFileIDFromReference(parsed *url.URL) uint64 {
	if parsed == nil || !strings.Contains(strings.TrimSpace(parsed.Path), "/front/upload/open") {
		return 0
	}
	fileID, _ := strconv.ParseUint(strings.TrimSpace(parsed.Query().Get("id")), 10, 64)
	return fileID
}

func uploadReferencePathCandidates(objectPath string) []string {
	result := []string{objectPath}
	if strings.HasPrefix(objectPath, "upload/") {
		result = append(result, strings.TrimPrefix(objectPath, "upload/"))
	}
	return result
}

func uploadReferenceURLMatches(file UploadFile, value string) bool {
	payload := BuildUploadFilePayload(file)
	expected, _ := payload["url"].(string)
	return normalizedUploadReferenceURL(expected) == normalizedUploadReferenceURL(value)
}

func normalizedUploadReferenceURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}
