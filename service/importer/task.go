package importer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shemic/dever/util"

	frontmodel "github.com/dever-package/front/model"
)

func createTask(
	ctx context.Context,
	accountID uint64,
	pagePath string,
	importKey string,
	fileID uint64,
	sheetName string,
	input importTaskInput,
) (taskSnapshot, error) {
	model := frontmodel.NewImportTaskModel()
	now := time.Now()
	taskID := util.ToUint64(model.Insert(ctx, map[string]any{
		"account_id":    accountID,
		"page_path":     strings.TrimSpace(pagePath),
		"import_key":    strings.TrimSpace(importKey),
		"file_id":       fileID,
		"sheet_name":    strings.TrimSpace(sheetName),
		"mapping_json":  encodeTaskInput(input),
		"status":        importTaskStatusPending,
		"progress":      0,
		"stage":         "等待执行",
		"total_rows":    0,
		"success_rows":  0,
		"failed_rows":   0,
		"summary_json":  "{}",
		"error_message": "",
		"created_at":    now,
		"updated_at":    now,
	}))
	if taskID == 0 {
		return taskSnapshot{}, fmt.Errorf("创建导入任务失败")
	}
	return findTask(ctx, taskID)
}

func claimPendingTask(ctx context.Context) (taskSnapshot, bool, error) {
	model := frontmodel.NewImportTaskModel()
	rows := model.SelectMap(ctx, map[string]any{"status": importTaskStatusPending}, map[string]any{
		"field":    "main.id",
		"order":    "main.id asc",
		"page":     1,
		"pageSize": 1,
	})
	if len(rows) == 0 {
		return taskSnapshot{}, false, nil
	}

	taskID := util.ToUint64(rows[0]["id"])
	if taskID == 0 {
		return taskSnapshot{}, false, nil
	}

	now := time.Now()
	updated := model.Update(ctx, map[string]any{
		"id":     taskID,
		"status": importTaskStatusPending,
	}, map[string]any{
		"status":     importTaskStatusRunning,
		"progress":   1,
		"stage":      "准备导入任务",
		"started_at": now,
		"updated_at": now,
	})
	if updated == 0 {
		return taskSnapshot{}, false, nil
	}

	task, err := findTask(ctx, taskID)
	if err != nil {
		return taskSnapshot{}, false, err
	}
	return task, true, nil
}

func findTask(ctx context.Context, taskID uint64) (taskSnapshot, error) {
	if taskID == 0 {
		return taskSnapshot{}, fmt.Errorf("导入任务不存在")
	}

	row := frontmodel.NewImportTaskModel().FindMap(ctx, map[string]any{"id": taskID})
	if len(row) == 0 {
		return taskSnapshot{}, fmt.Errorf("导入任务不存在")
	}
	return taskFromRow(row), nil
}

func findTaskByOwner(ctx context.Context, taskID, accountID uint64) (taskSnapshot, error) {
	task, err := findTask(ctx, taskID)
	if err != nil {
		return taskSnapshot{}, err
	}
	if accountID == 0 || task.AccountID == 0 || task.AccountID != accountID {
		return taskSnapshot{}, fmt.Errorf("暂无权限")
	}
	return task, nil
}

func updateTaskProgress(ctx context.Context, taskID uint64, progress int, stage string) {
	if taskID == 0 {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 99 {
		progress = 99
	}
	frontmodel.NewImportTaskModel().Update(ctx, map[string]any{"id": taskID}, map[string]any{
		"progress":   progress,
		"stage":      strings.TrimSpace(stage),
		"updated_at": time.Now(),
	})
}

func finishTaskSuccess(ctx context.Context, taskID uint64, summary importSummary) {
	now := time.Now()
	frontmodel.NewImportTaskModel().Update(ctx, map[string]any{"id": taskID}, map[string]any{
		"status":        importTaskStatusSuccess,
		"progress":      100,
		"stage":         "导入完成",
		"total_rows":    summary.TotalRows,
		"success_rows":  summary.SuccessRows,
		"failed_rows":   summary.FailedRows,
		"summary_json":  encodeSummary(summary),
		"error_message": "",
		"finished_at":   now,
		"updated_at":    now,
	})
}

func finishTaskFailed(ctx context.Context, taskID uint64, err error) {
	now := time.Now()
	frontmodel.NewImportTaskModel().Update(ctx, map[string]any{"id": taskID}, map[string]any{
		"status":        importTaskStatusFailed,
		"stage":         "导入失败",
		"error_message": normalizeErrorMessage(err),
		"finished_at":   now,
		"updated_at":    now,
	})
}

func taskFromRow(row map[string]any) taskSnapshot {
	return taskSnapshot{
		ID:           util.ToUint64(row["id"]),
		AccountID:    util.ToUint64(row["account_id"]),
		PagePath:     strings.TrimSpace(util.ToString(row["page_path"])),
		ImportKey:    strings.TrimSpace(util.ToString(row["import_key"])),
		FileID:       util.ToUint64(row["file_id"]),
		SheetName:    strings.TrimSpace(util.ToString(row["sheet_name"])),
		MappingJSON:  strings.TrimSpace(util.ToString(row["mapping_json"])),
		Status:       strings.TrimSpace(util.ToString(row["status"])),
		Progress:     util.ToIntDefault(row["progress"], 0),
		Stage:        strings.TrimSpace(util.ToString(row["stage"])),
		TotalRows:    util.ToIntDefault(row["total_rows"], 0),
		SuccessRows:  util.ToIntDefault(row["success_rows"], 0),
		FailedRows:   util.ToIntDefault(row["failed_rows"], 0),
		SummaryJSON:  strings.TrimSpace(util.ToString(row["summary_json"])),
		ErrorMessage: strings.TrimSpace(util.ToString(row["error_message"])),
	}
}
