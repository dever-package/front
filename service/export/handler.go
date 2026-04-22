package export

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	permissionservice "github.com/dever-package/front/service/permission"
	"my/authstate"
)

func CreateTask(c *server.Context) error {
	pagePath := strings.TrimSpace(c.Input("path", "required", "页面路径"))
	tableID := strings.TrimSpace(c.Input("tableId"))
	exportKey := strings.TrimSpace(c.Input("exportKey"))
	query := decodeTaskQuery(c.Input("query"))

	if err := permissionservice.EnsurePageAccess(c.Context(), pagePath); err != nil {
		return c.JSONPayload(http.StatusForbidden, map[string]any{
			"code":   http.StatusForbidden,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}
	if err := permissionservice.EnsureActionAccess(c.Context(), pagePath, exportKey); err != nil {
		return c.JSONPayload(http.StatusForbidden, map[string]any{
			"code":   http.StatusForbidden,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}
	if _, err := loadPageConfig(pagePath, tableID, exportKey); err != nil {
		return c.Error(err)
	}

	accountID := uint64(authstate.OptionalUID(c.Context()))
	task, err := createTask(c.Context(), accountID, pagePath, tableID, exportKey, query)
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(taskPayload(task))
}

func GetTaskInfo(c *server.Context) error {
	taskID := util.ToUint64(c.Input("id", "required", "导出任务"))
	accountID := uint64(authstate.OptionalUID(c.Context()))
	task, err := findTaskByOwner(c.Context(), taskID, accountID)
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}
	return c.JSON(taskPayload(task))
}

func DownloadTask(c *server.Context) error {
	taskID := util.ToUint64(c.Input("id", "required", "导出任务"))
	accountID := uint64(authstate.OptionalUID(c.Context()))
	task, err := findTaskByOwner(c.Context(), taskID, accountID)
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}
	if task.Status != taskStatusSuccess || strings.TrimSpace(task.ResultPath) == "" {
		return c.Error("导出文件尚未生成")
	}
	if _, err := os.Stat(task.ResultPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.Error("导出文件不存在", http.StatusNotFound)
		}
		return c.Error(fmt.Errorf("读取导出文件失败: %w", err))
	}

	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前环境不支持文件下载")
	}
	raw.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	raw.Set("Content-Disposition", buildDownloadFilenameHeader(util.FirstNonEmpty(task.ResultName, "export.xlsx")))
	return raw.SendFile(task.ResultPath)
}
