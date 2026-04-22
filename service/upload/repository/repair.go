package repository

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/shemic/dever/util"
)

const uploadBizRepairBatchSize = 200

var uploadBizRepairScheduled atomic.Bool

func ScheduleUploadFilesBizRepair() {
	if !uploadBizRepairScheduled.CompareAndSwap(false, true) {
		return
	}

	go func() {
		_ = RepairUploadFilesBizFromPath(context.Background())
	}()
}

func RepairUploadFilesBizFromPath(ctx context.Context) error {
	fileModel, err := ResolveFileModel()
	if err != nil {
		return err
	}

	for {
		rows := fileModel.SelectMap(ctx, map[string]any{"biz_id": 0}, map[string]any{
			"field":    "main.id, main.path",
			"order":    "main.id asc",
			"page":     1,
			"pageSize": uploadBizRepairBatchSize,
		})
		if len(rows) == 0 {
			return nil
		}

		repairedCount := 0
		for _, row := range rows {
			record := UploadFile{
				ID:   util.ToUint64(row["id"]),
				Path: util.ToStringTrimmed(row["path"]),
			}
			if err := RepairUploadFileBizFromPath(ctx, &record); err != nil {
				return err
			}
			if record.BizID != 0 {
				repairedCount += 1
			}
		}

		if repairedCount == 0 || len(rows) < uploadBizRepairBatchSize {
			return nil
		}
	}
}

func RepairUploadFileBizFromPath(ctx context.Context, record *UploadFile) error {
	if record == nil || record.ID == 0 || record.BizID != 0 {
		return nil
	}

	bizKey := inferUploadBizKeyFromPath(record.Path)
	if bizKey == "" {
		return nil
	}

	bizRecord, err := EnsureUploadBiz(ctx, bizKey, "")
	if err != nil {
		return err
	}
	if bizRecord.ID == 0 {
		return nil
	}

	fileModel, err := ResolveFileModel()
	if err != nil {
		return err
	}
	fileModel.Update(ctx, map[string]any{"id": record.ID}, map[string]any{"biz_id": bizRecord.ID})
	record.BizID = bizRecord.ID
	record.BizKey = bizRecord.Key
	record.BizName = bizRecord.Name
	return nil
}

func inferUploadBizKeyFromPath(path string) string {
	normalizedPath := strings.Trim(strings.TrimSpace(path), "/")
	if normalizedPath == "" {
		return ""
	}

	parts := strings.Split(normalizedPath, "/")
	if len(parts) < 3 || parts[0] != "upload" {
		return ""
	}

	bizKey := NormalizeBizKey(parts[2])
	if bizKey == "" || bizKey == "common" {
		return ""
	}
	return bizKey
}
