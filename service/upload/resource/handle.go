package resource

import (
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	operationlog "my/package/front/service/operationlog"
	frontrecord "my/package/front/service/record"
	uploadaccess "my/package/front/service/upload/access"
	uploadrepo "my/package/front/service/upload/repository"
)

type assignCategoryInput struct {
	FileID     uint64   `json:"file_id"`
	FileIDs    []uint64 `json:"file_ids"`
	CategoryID uint64   `json:"category_id"`
}

func HandleList(c *server.Context) error {
	page := maxPage(util.ToIntDefault(c.Input("page"), 0))
	pageSize := normalizePageSize(resolvePageSize(c))
	bizKey := uploadrepo.NormalizeBizKey(c.Input("biz_key"))
	kind := normalizeKind(c.Input("kind"))
	keyword := strings.TrimSpace(c.Input("keyword"))
	fileType := strings.ToLower(strings.TrimSpace(c.Input("file_type")))
	categoryID := strings.TrimSpace(c.Input("category_id"))
	if err := uploadaccess.EnsureResourceRequest(c, uploadaccess.Request{
		Operation:  uploadaccess.OperationList,
		BizKey:     bizKey,
		Kind:       kind,
		CategoryID: categoryID,
	}); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}

	fileModel, err := bootstrapFileModel(c)
	if err != nil {
		return c.Error(err)
	}

	filters, err := buildFilters(c.Context(), bizKey, kind, keyword, fileType, categoryID)
	if err != nil {
		return c.Error(err)
	}

	rows := fileModel.SelectMap(c.Context(), filters, map[string]any{
		"page":     page,
		"pageSize": pageSize,
		"order":    "main.id desc",
	})
	total := fileModel.Count(c.Context(), filters)

	list := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		record := uploadrepo.NormalizeUploadFileRow(row)
		if err := uploadrepo.HydrateUploadFile(c.Context(), &record); err != nil {
			return c.Error(err)
		}
		list = append(list, uploadrepo.BuildUploadFilePayload(record))
	}

	return c.JSON(map[string]any{
		"list":     list,
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
	})
}

func HandleListCategories(c *server.Context) error {
	bizKey := uploadrepo.NormalizeBizKey(c.Input("biz_key"))
	kind := normalizeKind(c.Input("kind"))
	withStats := util.ToBool(c.Input("stats"))
	if err := uploadaccess.EnsureResourceRequest(c, uploadaccess.Request{
		Operation: uploadaccess.OperationList,
		BizKey:    bizKey,
		Kind:      kind,
	}); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}
	fileModel, err := bootstrapFileModel(c)
	if err != nil {
		return c.Error(err)
	}
	items, err := loadCategoryItems(c.Context(), fileModel, bizKey, kind, withStats)
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(items)
}

func HandleListSources(c *server.Context) error {
	kind := normalizeKind(c.Input("kind"))
	if err := uploadaccess.EnsureResourceRequest(c, uploadaccess.Request{
		Operation: uploadaccess.OperationList,
		Kind:      kind,
	}); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}
	fileModel, err := bootstrapFileModel(c)
	if err != nil {
		return c.Error(err)
	}
	items, err := loadSourceItems(c.Context(), fileModel, kind)
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(items)
}

func HandleAssignCategory(c *server.Context) error {
	var input assignCategoryInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}

	fileIDs := normalizeFileIDs(input.FileID, input.FileIDs)
	if len(fileIDs) == 0 {
		return c.Error("资源文件不能为空")
	}
	if err := ensureManageResourceFiles(c, fileIDs); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}

	categoryID, err := uploadrepo.EnsureUploadCateID(c.Context(), input.CategoryID)
	if err != nil {
		return c.Error(err)
	}

	fileModel, err := uploadrepo.ResolveFileModel()
	if err != nil {
		return c.Error(err)
	}

	updated := fileModel.Update(c.Context(), map[string]any{"id": fileIDs}, map[string]any{
		"category_id": categoryID,
	})

	operationlog.Record(c, operationlog.Entry{
		Action:      "update",
		PagePath:    "front/resource/list",
		TargetModel: "front.NewUploadFileModel",
		TargetID:    joinUint64IDs(fileIDs),
		Message:     "调整资源分类",
		Payload:     input,
	})

	return c.JSON(map[string]any{
		"updated":     updated,
		"file_ids":    fileIDs,
		"category_id": categoryID,
	})
}

func joinUint64IDs(ids []uint64) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		values = append(values, util.ToString(id))
	}
	return strings.Join(values, ",")
}

func ensureManageResourceFiles(c *server.Context, fileIDs []uint64) error {
	for _, fileID := range fileIDs {
		fileRecord, err := uploadrepo.FindUploadFile(c.Context(), fileID)
		if err != nil {
			return err
		}
		if err := uploadaccess.EnsureFile(c, uploadaccess.OperationManage, fileRecord); err != nil {
			return err
		}
	}
	return nil
}

func bootstrapFileModel(c *server.Context) (frontrecord.Model, error) {
	uploadrepo.ScheduleUploadFilesBizRepair()
	return uploadrepo.ResolveFileModel()
}

func resolvePageSize(c *server.Context) int {
	pageSize := util.ToIntDefault(c.Input("page_size"), 0)
	if pageSize > 0 {
		return pageSize
	}
	return util.ToIntDefault(c.Input("pageSize"), 0)
}
