package export

import (
	"context"
	"encoding/json"
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
	tableID string,
	exportKey string,
	query map[string]string,
) (taskSnapshot, error) {
	model := frontmodel.NewExportTaskModel()
	now := time.Now()
	taskID := util.ToUint64(model.Insert(ctx, map[string]any{
		"account_id":    accountID,
		"page_path":     strings.TrimSpace(pagePath),
		"table_id":      strings.TrimSpace(tableID),
		"export_key":    strings.TrimSpace(exportKey),
		"status":        taskStatusPending,
		"progress":      0,
		"stage":         "等待执行",
		"query_json":    encodeTaskQuery(query),
		"result_name":   "",
		"result_path":   "",
		"error_message": "",
		"created_at":    now,
		"updated_at":    now,
	}))
	if taskID == 0 {
		return taskSnapshot{}, fmt.Errorf("创建导出任务失败")
	}
	return findTask(ctx, taskID)
}

func claimPendingTask(ctx context.Context) (taskSnapshot, bool, error) {
	model := frontmodel.NewExportTaskModel()
	rows := model.SelectMap(ctx, map[string]any{"status": taskStatusPending}, map[string]any{
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
		"status": taskStatusPending,
	}, map[string]any{
		"status":     taskStatusRunning,
		"progress":   1,
		"stage":      "准备导出任务",
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
		return taskSnapshot{}, fmt.Errorf("导出任务不存在")
	}

	row := frontmodel.NewExportTaskModel().FindMap(ctx, map[string]any{"id": taskID})
	if len(row) == 0 {
		return taskSnapshot{}, fmt.Errorf("导出任务不存在")
	}

	return taskFromRow(row), nil
}

func findTaskByOwner(ctx context.Context, taskID, accountID uint64) (taskSnapshot, error) {
	task, err := findTask(ctx, taskID)
	if err != nil {
		return taskSnapshot{}, err
	}
	if accountID != 0 && task.AccountID != 0 && task.AccountID != accountID {
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
	frontmodel.NewExportTaskModel().Update(ctx, map[string]any{"id": taskID}, map[string]any{
		"progress":   progress,
		"stage":      strings.TrimSpace(stage),
		"updated_at": time.Now(),
	})
}

func finishTaskSuccess(ctx context.Context, taskID uint64, resultName, resultPath string) {
	now := time.Now()
	frontmodel.NewExportTaskModel().Update(ctx, map[string]any{"id": taskID}, map[string]any{
		"status":        taskStatusSuccess,
		"progress":      100,
		"stage":         "导出完成",
		"result_name":   strings.TrimSpace(resultName),
		"result_path":   strings.TrimSpace(resultPath),
		"error_message": "",
		"finished_at":   now,
		"updated_at":    now,
	})
}

func finishTaskFailed(ctx context.Context, taskID uint64, err error) {
	now := time.Now()
	frontmodel.NewExportTaskModel().Update(ctx, map[string]any{"id": taskID}, map[string]any{
		"status":        taskStatusFailed,
		"stage":         "导出失败",
		"error_message": normalizeErrorMessage(err),
		"finished_at":   now,
		"updated_at":    now,
	})
}

func encodeTaskQuery(query map[string]string) string {
	if len(query) == 0 {
		return "{}"
	}
	data, err := json.Marshal(query)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeTaskQuery(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}
	}
	result := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]string{}
	}
	return result
}

func taskFromRow(row map[string]any) taskSnapshot {
	return taskSnapshot{
		ID:           util.ToUint64(row["id"]),
		AccountID:    util.ToUint64(row["account_id"]),
		PagePath:     strings.TrimSpace(util.ToString(row["page_path"])),
		TableID:      strings.TrimSpace(util.ToString(row["table_id"])),
		ExportKey:    strings.TrimSpace(util.ToString(row["export_key"])),
		Status:       strings.TrimSpace(util.ToString(row["status"])),
		Progress:     util.ToIntDefault(row["progress"], 0),
		Stage:        strings.TrimSpace(util.ToString(row["stage"])),
		Query:        decodeTaskQuery(util.ToString(row["query_json"])),
		ResultName:   strings.TrimSpace(util.ToString(row["result_name"])),
		ResultPath:   strings.TrimSpace(util.ToString(row["result_path"])),
		ErrorMessage: strings.TrimSpace(util.ToString(row["error_message"])),
	}
}

func taskPayload(task taskSnapshot) map[string]any {
	return map[string]any{
		"id":            task.ID,
		"page_path":     task.PagePath,
		"table_id":      task.TableID,
		"export_key":    task.ExportKey,
		"status":        task.Status,
		"progress":      task.Progress,
		"stage":         task.Stage,
		"result_name":   task.ResultName,
		"error_message": task.ErrorMessage,
	}
}

func normalizeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "导出失败"
	}
	return message
}
