package upload

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

const uploadDisplayNameMaxRunes = 240

type resolvedUploadName struct {
	Name       string
	Candidates []string
}

func lockAvailableUploadName(
	ctx context.Context,
	ruleID uint64,
	bizID uint64,
	sessionID uint64,
	originalName string,
	ext string,
	hash string,
) (resolvedUploadName, func(), error) {
	return lockAvailableUploadNameWithReservations(ctx, ruleID, bizID, sessionID, originalName, ext, hash)
}

func lockAvailableDirectUploadName(
	ctx context.Context,
	ruleID uint64,
	bizID uint64,
	originalName string,
	ext string,
	hash string,
) (resolvedUploadName, func(), error) {
	return lockAvailableUploadNameWithReservations(ctx, ruleID, bizID, 0, originalName, ext, hash)
}

func lockAvailableUploadNameWithReservations(
	ctx context.Context,
	ruleID uint64,
	bizID uint64,
	sessionID uint64,
	originalName string,
	ext string,
	hash string,
) (resolvedUploadName, func(), error) {
	sanitizedName := sanitizeUploadDisplayName(originalName, ext)
	unlock := lockUploadStoreKey(uploadNameLockKey(ruleID, bizID, sanitizedName))
	resolved, err := resolveAvailableUploadNameWithReservations(
		ctx,
		ruleID,
		bizID,
		sessionID,
		sanitizedName,
		hash,
	)
	if err != nil {
		unlock()
		return resolvedUploadName{}, nil, err
	}
	return resolved, unlock, nil
}

func resolveAvailableUploadName(
	ctx context.Context,
	ruleID uint64,
	bizID uint64,
	originalName string,
	hash string,
) (resolvedUploadName, error) {
	return resolveAvailableUploadNameWithReservations(ctx, ruleID, bizID, 0, originalName, hash)
}

func resolveAvailableUploadNameWithReservations(
	ctx context.Context,
	ruleID uint64,
	bizID uint64,
	sessionID uint64,
	originalName string,
	hash string,
) (resolvedUploadName, error) {
	candidates := uploadNameCandidates(originalName, hash)
	available := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		existing, err := uploadrepo.FindUploadFileByScopedName(ctx, ruleID, bizID, candidate)
		if err != nil {
			return resolvedUploadName{}, err
		}
		if existing == nil {
			reserved, err := isUploadNameReservedByActiveSession(ctx, ruleID, bizID, sessionID, candidate)
			if err != nil {
				return resolvedUploadName{}, err
			}
			if reserved {
				continue
			}
			available = append(available, candidate)
		}
	}
	if len(available) == 0 {
		return resolvedUploadName{}, fmt.Errorf("文件名称冲突，请修改文件名后重试")
	}
	return resolvedUploadName{
		Name:       available[0],
		Candidates: available,
	}, nil
}

func isUploadNameReservedByActiveSession(ctx context.Context, ruleID, bizID, sessionID uint64, name string) (bool, error) {
	return uploadrepo.HasActiveUploadSessionByScopedName(ctx, ruleID, bizID, sessionID, name)
}

func uploadNameCandidates(name, hash string) []string {
	name = sanitizeUploadDisplayName(name, filepath.Ext(name))
	result := []string{name}
	normalizedHash := normalizeUploadHash(hash)
	if normalizedHash == "" {
		return result
	}

	seen := map[string]struct{}{name: {}}
	for _, length := range []int{8, 16, len(normalizedHash)} {
		if length > len(normalizedHash) {
			length = len(normalizedHash)
		}
		candidate := uploadNameWithHashSuffix(name, normalizedHash[:length])
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func sanitizeUploadDisplayName(name, fallbackExt string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	normalized = path.Base(normalized)

	var builder strings.Builder
	for _, char := range normalized {
		switch {
		case unicode.IsControl(char):
			continue
		case strings.ContainsRune(`/:*?"<>|`, char):
			builder.WriteRune('_')
		default:
			builder.WriteRune(char)
		}
	}

	normalized = strings.TrimSpace(builder.String())
	if normalized == "" || normalized == "." || normalized == ".." {
		normalized = "file"
	}
	if filepath.Ext(normalized) == "" {
		normalized += sanitizeUploadFallbackExt(fallbackExt)
	}
	return trimUploadDisplayName(normalized, uploadDisplayNameMaxRunes)
}

func sanitizeUploadFallbackExt(value string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext("file" + strings.TrimSpace(value))))
	if ext == "" || len(ext) > 24 {
		return ""
	}
	for _, char := range strings.TrimPrefix(ext, ".") {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return ""
		}
	}
	return ext
}

func uploadNameWithHashSuffix(name, hash string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	suffix := "__" + hash
	maxStemRunes := uploadDisplayNameMaxRunes - len([]rune(ext)) - len([]rune(suffix))
	if maxStemRunes < 1 {
		maxStemRunes = 1
	}
	stem = trimRunes(stem, maxStemRunes)
	return stem + suffix + ext
}

func trimUploadDisplayName(name string, maxRunes int) string {
	if len([]rune(name)) <= maxRunes {
		return name
	}
	ext := filepath.Ext(name)
	maxStemRunes := maxRunes - len([]rune(ext))
	if maxStemRunes < 1 {
		return trimRunes(name, maxRunes)
	}
	return trimRunes(strings.TrimSuffix(name, ext), maxStemRunes) + ext
}

func trimRunes(value string, max int) string {
	runes := []rune(value)
	if max <= 0 {
		return ""
	}
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
