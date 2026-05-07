package resource

import (
	"context"

	"github.com/shemic/dever/util"

	uploadrepo "my/package/front/service/upload/repository"

	frontrecord "my/package/front/service/record"
)

func loadCategoryItems(
	ctx context.Context,
	fileModel frontrecord.Model,
	bizKey string,
	kind string,
	withStats bool,
) ([]map[string]any, error) {
	items := make([]map[string]any, 0, 16)

	if withStats {
		totalCount, err := countResourcesByCategory(ctx, fileModel, bizKey, kind, "")
		if err != nil {
			return nil, err
		}
		uncategorizedCount, err := countResourcesByCategory(ctx, fileModel, bizKey, kind, categoryUncategorized)
		if err != nil {
			return nil, err
		}
		items = append(items,
			buildCategorySummaryItem("全部资源", categoryAll, totalCount),
			buildCategorySummaryItem("未分类", categoryUncategorized, uncategorizedCount),
		)
	}

	categoryModel, err := uploadrepo.ResolveCateModel()
	if err != nil {
		return items, nil
	}

	rows := categoryModel.SelectMap(ctx, nil, map[string]any{
		"page":     1,
		"pageSize": 500,
		"order":    "main.sort asc, main.id asc",
	})
	for _, row := range rows {
		categoryID := util.ToUint64(row["id"])
		item := map[string]any{
			"id":     categoryID,
			"value":  util.ToStringTrimmed(row["name"]),
			"raw_id": categoryID,
			"status": util.ToIntDefault(row["status"], 0),
			"sort":   util.ToIntDefault(row["sort"], 0),
		}
		if withStats {
			count, err := countResourcesByCategory(ctx, fileModel, bizKey, kind, util.ToString(categoryID))
			if err != nil {
				return nil, err
			}
			item["count"] = count
		}
		items = append(items, item)
	}

	return items, nil
}

func countResourcesByCategory(
	ctx context.Context,
	fileModel frontrecord.Model,
	bizKey string,
	kind string,
	categoryID string,
) (int64, error) {
	filters, err := buildFilters(ctx, bizKey, kind, "", "", categoryID)
	if err != nil {
		return 0, err
	}
	return fileModel.Count(ctx, filters), nil
}

func buildCategorySummaryItem(label string, categoryID string, count int64) map[string]any {
	return map[string]any{
		"id":    categoryID,
		"value": label,
		"count": count,
	}
}
