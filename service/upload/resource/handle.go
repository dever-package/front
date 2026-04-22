package resource

import (
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontrecord "github.com/dever-package/front/service/record"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type assignCategoryInput struct {
	FileID     uint64   `json:"file_id"`
	FileIDs    []uint64 `json:"file_ids"`
	CategoryID uint64   `json:"category_id"`
}

func HandleList(c *server.Context) error {
	fileModel, err := bootstrapFileModel(c)
	if err != nil {
		return c.Error(err)
	}

	page := maxPage(util.ToIntDefault(c.Input("page"), 0))
	pageSize := normalizePageSize(resolvePageSize(c))
	bizKey := uploadrepo.NormalizeBizKey(c.Input("biz_key"))
	kind := normalizeKind(c.Input("kind"))
	keyword := strings.TrimSpace(c.Input("keyword"))
	fileType := strings.ToLower(strings.TrimSpace(c.Input("file_type")))
	categoryID := strings.TrimSpace(c.Input("category_id"))

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
	fileModel, err := bootstrapFileModel(c)
	if err != nil {
		return c.Error(err)
	}

	bizKey := uploadrepo.NormalizeBizKey(c.Input("biz_key"))
	kind := normalizeKind(c.Input("kind"))
	withStats := util.ToBool(c.Input("stats"))
	items, err := loadCategoryItems(c.Context(), fileModel, bizKey, kind, withStats)
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(items)
}

func HandleListSources(c *server.Context) error {
	fileModel, err := bootstrapFileModel(c)
	if err != nil {
		return c.Error(err)
	}

	kind := normalizeKind(c.Input("kind"))
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

	return c.JSON(map[string]any{
		"updated":     updated,
		"file_ids":    fileIDs,
		"category_id": categoryID,
	})
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
