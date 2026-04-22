package resource

import (
	"context"

	"github.com/shemic/dever/util"

	uploadrepo "github.com/dever-package/front/service/upload/repository"

	frontrecord "github.com/dever-package/front/service/record"
)

func loadSourceItems(
	ctx context.Context,
	fileModel frontrecord.Model,
	kind string,
) ([]map[string]any, error) {
	filters, err := buildFilters(ctx, "", kind, "", "", "")
	if err != nil {
		return nil, err
	}
	rows := fileModel.SelectMap(ctx, filters, map[string]any{
		"page":     1,
		"pageSize": 500,
		"order":    "main.biz_id asc, main.id desc",
		"field":    "main.biz_id",
	})

	seen := map[uint64]struct{}{}
	bizIDs := make([]uint64, 0, 32)
	for _, row := range rows {
		bizID := util.ToUint64(row["biz_id"])
		if bizID == 0 {
			continue
		}
		if _, exists := seen[bizID]; exists {
			continue
		}
		seen[bizID] = struct{}{}
		bizIDs = append(bizIDs, bizID)
	}

	bizRecords, err := uploadrepo.FindUploadBizMapByIDs(ctx, bizIDs)
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0, len(bizIDs))
	for _, bizID := range bizIDs {
		bizRecord := bizRecords[bizID]
		if bizRecord.ID == 0 || bizRecord.Key == "" {
			continue
		}
		items = append(items, map[string]any{
			"id":    bizRecord.Key,
			"value": resolveSourceValue(bizRecord.Key, bizRecord.Name),
		})
	}

	return items, nil
}

func resolveSourceValue(sourceKey string, sourceName string) string {
	if normalized := uploadrepo.NormalizeBizName(sourceName); normalized != "" {
		return normalized
	}
	return uploadrepo.NormalizeBizKey(sourceKey)
}
