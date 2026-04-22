package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/shemic/dever/util"
)

func EnsureUploadBiz(ctx context.Context, bizKey, bizName string) (UploadBiz, error) {
	normalizedBizKey := NormalizeBizKey(bizKey)
	normalizedBizName := NormalizeBizName(bizName)
	if normalizedBizKey == "" {
		return UploadBiz{}, nil
	}
	if normalizedBizName == "" {
		normalizedBizName = normalizedBizKey
	}

	record, err := FindUploadBizByKey(ctx, normalizedBizKey)
	if err == nil && record.ID != 0 {
		if normalizedBizName != "" && record.Name != normalizedBizName {
			bizModel, err := ResolveBizModel()
			if err != nil {
				return UploadBiz{}, err
			}
			bizModel.Update(ctx, map[string]any{"id": record.ID}, map[string]any{"name": normalizedBizName})
			record.Name = normalizedBizName
		}
		return record, nil
	}

	bizModel, err := ResolveBizModel()
	if err != nil {
		return UploadBiz{}, err
	}
	bizID := util.ToUint64(bizModel.Insert(ctx, map[string]any{
		"key":        normalizedBizKey,
		"name":       normalizedBizName,
		"created_at": time.Now(),
	}))
	if bizID == 0 {
		return UploadBiz{}, fmt.Errorf("保存资源来源失败")
	}
	return UploadBiz{ID: bizID, Key: normalizedBizKey, Name: normalizedBizName}, nil
}

func FindUploadBizByID(ctx context.Context, bizID uint64) (UploadBiz, error) {
	if bizID == 0 {
		return UploadBiz{}, nil
	}

	bizModel, err := ResolveBizModel()
	if err != nil {
		return UploadBiz{}, err
	}
	row := bizModel.FindMap(ctx, map[string]any{"id": bizID})
	if len(row) == 0 {
		return UploadBiz{}, fmt.Errorf("资源来源不存在")
	}
	return mapUploadBiz(row), nil
}

func FindUploadBizMapByIDs(ctx context.Context, bizIDs []uint64) (map[uint64]UploadBiz, error) {
	normalizedIDs := util.UniqueUint64s(bizIDs)
	if len(normalizedIDs) == 0 {
		return map[uint64]UploadBiz{}, nil
	}

	bizModel, err := ResolveBizModel()
	if err != nil {
		return nil, err
	}

	rows := bizModel.SelectMap(ctx, map[string]any{"id": normalizedIDs})
	result := make(map[uint64]UploadBiz, len(rows))
	for _, row := range rows {
		record := mapUploadBiz(row)
		if record.ID == 0 {
			continue
		}
		result[record.ID] = record
	}
	return result, nil
}

func FindUploadBizByKey(ctx context.Context, bizKey string) (UploadBiz, error) {
	normalizedBizKey := NormalizeBizKey(bizKey)
	if normalizedBizKey == "" {
		return UploadBiz{}, nil
	}

	bizModel, err := ResolveBizModel()
	if err != nil {
		return UploadBiz{}, err
	}
	row := bizModel.FindMap(ctx, map[string]any{"key": normalizedBizKey})
	if len(row) == 0 {
		return UploadBiz{}, nil
	}
	return mapUploadBiz(row), nil
}

func EnsureUploadCateID(ctx context.Context, categoryID uint64) (uint64, error) {
	if categoryID == 0 {
		return 0, nil
	}

	cateModel, err := ResolveCateModel()
	if err != nil {
		return 0, err
	}
	row := cateModel.FindMap(ctx, map[string]any{"id": categoryID})
	if len(row) == 0 {
		return 0, fmt.Errorf("资源分类不存在")
	}
	return util.ToUint64(row["id"]), nil
}
