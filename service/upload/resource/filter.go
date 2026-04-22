package resource

import (
	"context"
	"strings"

	"github.com/shemic/dever/util"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

const (
	categoryAll           = "__all__"
	categoryUncategorized = "__uncategorized__"
)

func buildFilters(
	ctx context.Context,
	bizKey string,
	kind string,
	keyword string,
	fileType string,
	categoryID string,
) (any, error) {
	conditions := make([]any, 0, 6)

	if bizKey != "" {
		bizRecord, err := uploadrepo.FindUploadBizByKey(ctx, bizKey)
		if err != nil {
			return nil, err
		}
		if bizRecord.ID == 0 {
			return map[string]any{"main.id": 0}, nil
		}
		conditions = append(conditions, map[string]any{"main.biz_id": bizRecord.ID})
	}
	if kind != "" {
		conditions = append(conditions, map[string]any{"main.kind": kind})
	}
	if categoryID == categoryUncategorized {
		conditions = append(conditions, map[string]any{"main.category_id": 0})
	}
	if categoryID != "" && categoryID != categoryUncategorized && categoryID != categoryAll {
		normalizedCategoryID := uploadrepo.NormalizeRelationID(categoryID)
		if normalizedCategoryID == 0 {
			return map[string]any{"main.id": 0}, nil
		}
		conditions = append(conditions, map[string]any{"main.category_id": normalizedCategoryID})
	}
	if keyword != "" {
		conditions = append(conditions, map[string]any{
			"or": []any{
				map[string]any{"main.name": map[string]any{"like": "%" + keyword + "%"}},
				map[string]any{"main.hash": map[string]any{"like": "%" + keyword + "%"}},
			},
		})
	}
	if fileType != "" {
		token := fileType
		if !strings.HasPrefix(token, ".") && !strings.Contains(token, "/") {
			token = "%" + token + "%"
		}
		conditions = append(conditions, map[string]any{
			"or": []any{
				map[string]any{"main.ext": map[string]any{"like": token}},
				map[string]any{"main.mime": map[string]any{"like": "%" + fileType + "%"}},
				map[string]any{"main.kind": map[string]any{"like": "%" + fileType + "%"}},
			},
		})
	}

	switch len(conditions) {
	case 0:
		return nil, nil
	case 1:
		return conditions[0], nil
	default:
		return map[string]any{"and": conditions}, nil
	}
}

func normalizeKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return ""
	case "image", "video", "audio", "file", "other":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizePageSize(size int) int {
	switch {
	case size <= 0:
		return 24
	case size > 100:
		return 100
	default:
		return size
	}
}

func maxPage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizeFileIDs(fileID uint64, fileIDs []uint64) []uint64 {
	items := make([]uint64, 0, len(fileIDs)+1)
	if fileID != 0 {
		items = append(items, fileID)
	}
	for _, current := range fileIDs {
		if current == 0 {
			continue
		}
		items = append(items, current)
	}
	return util.UniqueUint64s(items)
}
