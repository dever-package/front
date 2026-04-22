package importer

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmeta "github.com/dever-package/front/service/meta"
	permissionservice "github.com/dever-package/front/service/permission"
	"my/authstate"
)

func Analyze(c *server.Context) error {
	pagePath := strings.TrimSpace(c.Input("path", "required", "页面路径"))
	importKey := strings.TrimSpace(c.Input("importKey"))
	fileID := util.ToUint64(c.Input("fileId", "required", "导入文件"))
	sheetName := strings.TrimSpace(c.Input("sheetName"))

	if err := permissionservice.EnsurePageAccess(c.Context(), pagePath); err != nil {
		return c.JSONPayload(http.StatusForbidden, map[string]any{
			"code":   http.StatusForbidden,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}
	if err := permissionservice.EnsureActionAccess(c.Context(), pagePath, importKey); err != nil {
		return c.JSONPayload(http.StatusForbidden, map[string]any{
			"code":   http.StatusForbidden,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}

	config, err := resolveImportConfig(pagePath, importKey)
	if err != nil {
		return c.Error(err)
	}

	fileRecord, filePath, err := resolveImportFilePath(c.Context(), fileID)
	if err != nil {
		return c.Error(err)
	}

	analysis, err := analyzeWorkbook(filePath, sheetName, config.Fields)
	if err != nil {
		return c.Error(err)
	}

	return c.JSON(map[string]any{
		"name":           config.Name,
		"model":          config.Model,
		"upload_rule_id": config.UploadRuleID,
		"file": map[string]any{
			"id":   fileRecord.ID,
			"name": fileRecord.Name,
		},
		"sheets":       analysis.Sheets,
		"sheet_name":   analysis.SheetName,
		"columns":      analysis.Columns,
		"fields":       normalizeImportFieldPayloads(config.Fields),
		"match_fields": config.MatchFields,
		"match_mode":   config.MatchMode,
		"match_options": normalizeImportMatchFieldPayloads(
			frontmeta.ResolveImportMatchCandidates(config.Model, config.MatchFields),
			config.Fields,
		),
	})
}

func CreateTask(c *server.Context) error {
	pagePath := strings.TrimSpace(c.Input("path", "required", "页面路径"))
	importKey := strings.TrimSpace(c.Input("importKey"))
	fileID := util.ToUint64(c.Input("fileId", "required", "导入文件"))
	sheetName := strings.TrimSpace(c.Input("sheetName"))
	mappings := decodeTaskInput(c.Input("mappings")).Mappings
	settings := importTaskInput{
		Mappings:      filterMappings(mappings),
		MatchFields:   decodeInputStringSlice(c.Input("matchFields")),
		MatchMode:     strings.TrimSpace(c.Input("matchMode")),
		FieldSettings: decodeImportFieldSettings(c.Input("fieldSettings")),
	}

	if err := permissionservice.EnsurePageAccess(c.Context(), pagePath); err != nil {
		return c.JSONPayload(http.StatusForbidden, map[string]any{
			"code":   http.StatusForbidden,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}
	if err := permissionservice.EnsureActionAccess(c.Context(), pagePath, importKey); err != nil {
		return c.JSONPayload(http.StatusForbidden, map[string]any{
			"code":   http.StatusForbidden,
			"status": 2,
			"msg":    err.Error(),
			"data":   nil,
		})
	}

	if _, err := resolveImportConfig(pagePath, importKey); err != nil {
		return c.Error(err)
	}
	if _, _, err := resolveImportFilePath(c.Context(), fileID); err != nil {
		return c.Error(err)
	}
	if len(settings.Mappings) == 0 {
		return c.Error("请至少映射一个导入字段")
	}

	accountID := uint64(authstate.OptionalUID(c.Context()))
	task, err := createTask(c.Context(), accountID, pagePath, importKey, fileID, sheetName, settings)
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(taskPayload(task))
}

func GetTaskInfo(c *server.Context) error {
	taskID := util.ToUint64(c.Input("id", "required", "导入任务"))
	accountID := uint64(authstate.OptionalUID(c.Context()))
	task, err := findTaskByOwner(c.Context(), taskID, accountID)
	if err != nil {
		return c.Error(err, http.StatusNotFound)
	}
	return c.JSON(taskPayload(task))
}

func filterMappings(items []mappingItem) []mappingItem {
	result := make([]mappingItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Field) == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func decodeInputStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	values := make([]string, 0)
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	values = frontmeta.NormalizeImportMatchFields(values)
	if len(values) == 0 {
		return nil
	}
	return values
}

func decodeImportFieldSettings(raw string) map[string]importFieldSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	settings := map[string]importFieldSetting{}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil
	}
	return normalizeImportFieldSettings(settings)
}
