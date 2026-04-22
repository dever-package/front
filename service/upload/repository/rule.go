package repository

import (
	"context"
	"fmt"

	"github.com/shemic/dever/util"
)

func FindUploadRule(ctx context.Context, ruleID uint64) (UploadRule, error) {
	ruleModel, err := ResolveRuleModel()
	if err != nil {
		return UploadRule{}, err
	}

	row := ruleModel.FindMap(ctx, map[string]any{"id": ruleID})
	if len(row) == 0 {
		return UploadRule{}, fmt.Errorf("上传规则不存在")
	}

	rule := UploadRule{
		ID:           util.ToUint64(row["id"]),
		Name:         util.ToStringTrimmed(row["name"]),
		StorageID:    util.ToUint64(row["storage_id"]),
		AcceptTypeID: util.ToUint64(row["accept_type_id"]),
		Transport:    util.ToStringTrimmed(row["transport"]),
		ChunkSizeMB:  util.ToInt64(row["chunk_size"]),
		MaxSizeMB:    util.ToInt64(row["max_size"]),
		Status:       int(util.ToInt64(row["status"])),
	}
	if rule.Transport == "" {
		rule.Transport = "relay"
	}

	rule.Storage, err = FindUploadStorage(ctx, rule.StorageID)
	if err != nil {
		return UploadRule{}, err
	}
	rule.AcceptTypes, err = FindUploadAcceptTypesByRule(ctx, rule.ID, rule.AcceptTypeID)
	if err != nil {
		return UploadRule{}, err
	}
	if len(rule.AcceptTypes) > 0 {
		rule.AcceptType = rule.AcceptTypes[0]
		rule.AcceptTypeIDs = CollectAcceptTypeIDs(rule.AcceptTypes)
	}
	rule.Accept = MergeAcceptTypes(rule.AcceptTypes)
	return rule, nil
}

func FindUploadStorage(ctx context.Context, storageID uint64) (UploadStorage, error) {
	if storageID == 0 {
		return UploadStorage{}, fmt.Errorf("上传存储方式不能为空")
	}

	storageModel, err := ResolveStorageModel()
	if err != nil {
		return UploadStorage{}, err
	}
	row := storageModel.FindMap(ctx, map[string]any{"id": storageID})
	if len(row) == 0 {
		return UploadStorage{}, fmt.Errorf("上传存储方式不存在")
	}
	return mapUploadStorage(row), nil
}

func FindUploadAcceptType(ctx context.Context, acceptTypeID uint64) (UploadAcceptType, error) {
	if acceptTypeID == 0 {
		return UploadAcceptType{}, fmt.Errorf("上传允许类型不能为空")
	}

	acceptTypeModel, err := ResolveAcceptTypeModel()
	if err != nil {
		return UploadAcceptType{}, err
	}
	row := acceptTypeModel.FindMap(ctx, map[string]any{"id": acceptTypeID})
	if len(row) == 0 {
		return UploadAcceptType{}, fmt.Errorf("上传允许类型不存在")
	}
	return mapUploadAcceptType(row), nil
}

func FindUploadAcceptTypesByRule(ctx context.Context, ruleID, fallbackAcceptTypeID uint64) ([]UploadAcceptType, error) {
	ids := make([]uint64, 0, 4)
	relationModel, err := ResolveRuleAcceptTypeModel()
	if err == nil && ruleID != 0 {
		rows := relationModel.SelectMap(ctx, map[string]any{"upload_rule_id": ruleID}, map[string]any{
			"field": "main.accept_type_id",
			"order": "main.id asc",
		})
		for _, row := range rows {
			id := util.ToUint64(row["accept_type_id"])
			if id != 0 {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 && fallbackAcceptTypeID != 0 {
		ids = append(ids, fallbackAcceptTypeID)
	}
	ids = util.UniqueUint64s(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("上传允许类型不能为空")
	}

	result := make([]UploadAcceptType, 0, len(ids))
	for _, acceptTypeID := range ids {
		record, err := FindUploadAcceptType(ctx, acceptTypeID)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}
