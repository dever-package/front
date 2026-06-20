package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xuri/excelize/v2"

	frontmeta "github.com/dever-package/front/service/meta"
	uploadservice "github.com/dever-package/front/service/upload"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type importUploadContext struct {
	WorkbookDir    string
	PicturesByCell map[string][]excelize.Picture
	PictureColumns map[int]struct{}
}

type importUploadURLItem struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func buildImportUploadContext(workbook *excelize.File, filePath, sheetName string) importUploadContext {
	picturesByCell, pictureColumns := collectSheetPictures(workbook, sheetName)
	return importUploadContext{
		WorkbookDir:    filepath.Dir(strings.TrimSpace(filePath)),
		PicturesByCell: picturesByCell,
		PictureColumns: pictureColumns,
	}
}

func collectSheetPictures(workbook *excelize.File, sheetName string) (map[string][]excelize.Picture, map[int]struct{}) {
	picturesByCell := map[string][]excelize.Picture{}
	pictureColumns := map[int]struct{}{}
	if workbook == nil || strings.TrimSpace(sheetName) == "" {
		return picturesByCell, pictureColumns
	}

	cells, err := workbook.GetPictureCells(sheetName)
	if err != nil {
		return picturesByCell, pictureColumns
	}

	for _, cellName := range cells {
		cellName = strings.TrimSpace(cellName)
		if cellName == "" {
			continue
		}
		pictures, err := workbook.GetPictures(sheetName, cellName)
		if err != nil || len(pictures) == 0 {
			continue
		}
		picturesByCell[cellName] = pictures
		columnIndex, rowIndex, err := excelize.CellNameToCoordinates(cellName)
		if err != nil || rowIndex <= 1 || columnIndex <= 0 {
			continue
		}
		pictureColumns[columnIndex-1] = struct{}{}
	}
	return picturesByCell, pictureColumns
}

func columnHasPicture(uploadCtx importUploadContext, columnIndex int) bool {
	_, ok := uploadCtx.PictureColumns[columnIndex]
	return ok
}

func resolveUploadFieldValue(
	ctx context.Context,
	modelName string,
	field frontmeta.ImportField,
	rawValue string,
	cellName string,
	uploadCtx importUploadContext,
) (any, error) {
	sourceMode := strings.ToLower(strings.TrimSpace(field.SourceMode))
	if sourceMode == "" {
		sourceMode = "auto"
	}

	if sourceMode != "path" {
		pictures := uploadCtx.PicturesByCell[strings.TrimSpace(cellName)]
		if len(pictures) > 0 {
			value, err := importUploadPictures(ctx, modelName, field, cellName, pictures)
			if err != nil {
				if field.MissingPolicy == "skip" {
					return nil, nil
				}
				return nil, err
			}
			return value, nil
		} else if sourceMode == "embed" {
			if field.MissingPolicy == "skip" {
				return nil, nil
			}
			return nil, fmt.Errorf("未识别到贴图")
		}
	}

	if sourceMode == "embed" {
		if field.MissingPolicy == "skip" {
			return nil, nil
		}
		return nil, fmt.Errorf("未识别到贴图")
	}

	value, err := importUploadPaths(ctx, modelName, field, rawValue, uploadCtx.WorkbookDir)
	if err != nil && field.MissingPolicy == "skip" {
		return nil, nil
	}
	return value, err
}

func importUploadPictures(
	ctx context.Context,
	modelName string,
	field frontmeta.ImportField,
	cellName string,
	pictures []excelize.Picture,
) (any, error) {
	if len(pictures) == 0 {
		return nil, nil
	}
	if !field.Multiple && len(pictures) > 1 {
		pictures = pictures[:1]
	}

	ids := make([]any, 0, len(pictures))
	urlItems := make([]importUploadURLItem, 0, len(pictures))
	seen := map[uint64]struct{}{}
	seenURL := map[string]struct{}{}
	for index, picture := range pictures {
		ext := strings.ToLower(strings.TrimSpace(picture.Extension))
		if ext == "" {
			ext = ".png"
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		name := fmt.Sprintf("%s-%s", normalizeImportUploadName(field.Field), strings.ToLower(strings.TrimSpace(cellName)))
		if len(pictures) > 1 {
			name = fmt.Sprintf("%s-%d", name, index+1)
		}
		name += ext

		fileRecord, err := uploadservice.ImportFile(ctx, uploadservice.ImportFileInput{
			RuleID:     uint64(field.UploadRuleID),
			Kind:       field.UploadKind,
			Name:       name,
			Mime:       normalizeImportUploadMime(ext),
			Content:    picture.File,
			BizKey:     buildImportUploadBizKey(modelName, field.Field),
			BizName:    field.Label,
			CategoryID: 0,
		})
		if err != nil {
			if field.MissingPolicy == "skip" {
				continue
			}
			return nil, err
		}
		if _, exists := seen[fileRecord.ID]; exists {
			if field.SaveMode != "url" {
				continue
			}
		}
		seen[fileRecord.ID] = struct{}{}
		if field.SaveMode == "url" {
			urlItem, ok := buildImportUploadURLItem(fileRecord)
			if !ok {
				if field.MissingPolicy == "skip" {
					continue
				}
				return nil, fmt.Errorf("上传文件地址无效")
			}
			if _, exists := seenURL[urlItem.URL]; exists {
				continue
			}
			seenURL[urlItem.URL] = struct{}{}
			urlItems = append(urlItems, urlItem)
			continue
		}
		ids = append(ids, fileRecord.ID)
	}
	return normalizeImportUploadValue(ids, urlItems, field)
}

func importUploadPaths(
	ctx context.Context,
	modelName string,
	field frontmeta.ImportField,
	rawValue string,
	workbookDir string,
) (any, error) {
	pathItems := []string{strings.TrimSpace(rawValue)}
	if field.Multiple {
		pathItems = splitImportValues(rawValue, field.Delimiters)
	}

	ids := make([]any, 0, len(pathItems))
	urlItems := make([]importUploadURLItem, 0, len(pathItems))
	seen := map[uint64]struct{}{}
	seenURL := map[string]struct{}{}
	for _, item := range pathItems {
		for _, localPath := range resolveImportUploadPaths(item, field, workbookDir) {
			fileRecord, err := uploadservice.ImportFile(ctx, uploadservice.ImportFileInput{
				RuleID:     uint64(field.UploadRuleID),
				Kind:       field.UploadKind,
				Name:       filepath.Base(localPath),
				LocalPath:  localPath,
				BizKey:     buildImportUploadBizKey(modelName, field.Field),
				BizName:    field.Label,
				CategoryID: 0,
			})
			if err != nil {
				if field.MissingPolicy == "skip" {
					continue
				}
				return nil, err
			}
			if _, exists := seen[fileRecord.ID]; exists {
				if field.SaveMode != "url" {
					continue
				}
			}
			seen[fileRecord.ID] = struct{}{}
			if field.SaveMode == "url" {
				urlItem, ok := buildImportUploadURLItem(fileRecord)
				if !ok {
					if field.MissingPolicy == "skip" {
						continue
					}
					return nil, fmt.Errorf("上传文件地址无效")
				}
				if _, exists := seenURL[urlItem.URL]; exists {
					continue
				}
				seenURL[urlItem.URL] = struct{}{}
				urlItems = append(urlItems, urlItem)
				continue
			}
			ids = append(ids, fileRecord.ID)
		}
	}
	if field.SaveMode == "url" && len(urlItems) == 0 {
		if field.MissingPolicy == "skip" {
			return nil, nil
		}
		return nil, fmt.Errorf("未找到可导入的文件")
	}
	if field.SaveMode != "url" && len(ids) == 0 {
		if field.MissingPolicy == "skip" {
			return nil, nil
		}
		return nil, fmt.Errorf("未找到可导入的文件")
	}
	return normalizeImportUploadValue(ids, urlItems, field)
}

func resolveImportUploadPaths(rawPath string, field frontmeta.ImportField, workbookDir string) []string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return nil
	}
	rootAbs, rootReal, ok := resolveImportUploadRoot(workbookDir)
	if !ok || filepath.IsAbs(rawPath) {
		return nil
	}

	candidates := make([]string, 0, 4)
	appendCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		candidate, ok := resolveImportUploadCandidate(rootAbs, rootReal, value)
		if !ok {
			return
		}
		if _, err := os.Stat(candidate); err == nil {
			candidates = append(candidates, candidate)
		}
	}

	if baseDir := strings.TrimSpace(field.BaseDir); baseDir != "" {
		if !filepath.IsAbs(baseDir) {
			appendCandidate(filepath.Join(baseDir, rawPath))
		}
	}
	appendCandidate(rawPath)

	if len(candidates) == 0 {
		return nil
	}

	expanded := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			expanded = append(expanded, candidate)
			continue
		}

		entries, err := os.ReadDir(candidate)
		if err != nil {
			continue
		}
		dirFiles := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			filePath, ok := resolveExistingImportUploadFile(rootReal, filepath.Join(candidate, entry.Name()))
			if !ok {
				continue
			}
			dirFiles = append(dirFiles, filePath)
		}
		slices.Sort(dirFiles)
		if !field.Multiple {
			if len(dirFiles) == 1 {
				expanded = append(expanded, dirFiles[0])
			}
			continue
		}
		expanded = append(expanded, dirFiles...)
	}
	return uniqueImportStrings(expanded)
}

func resolveImportUploadRoot(workbookDir string) (string, string, bool) {
	workbookDir = strings.TrimSpace(workbookDir)
	if workbookDir == "" {
		return "", "", false
	}
	rootAbs, err := filepath.Abs(filepath.Clean(workbookDir))
	if err != nil {
		return "", "", false
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootReal = rootAbs
	}
	return rootAbs, rootReal, true
}

func resolveImportUploadCandidate(rootAbs, rootReal, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return "", false
	}
	candidate, err := filepath.Abs(filepath.Join(rootAbs, filepath.Clean(value)))
	if err != nil || !isImportUploadPathInside(rootAbs, candidate) {
		return "", false
	}
	if realPath, ok := resolveExistingImportUploadPath(rootReal, candidate); ok {
		return realPath, true
	}
	return candidate, true
}

func resolveExistingImportUploadFile(rootReal, filePath string) (string, bool) {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return "", false
	}
	return resolveExistingImportUploadPath(rootReal, filePath)
}

func resolveExistingImportUploadPath(rootReal, filePath string) (string, bool) {
	realPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", false
	}
	if !isImportUploadPathInside(rootReal, realPath) {
		return "", false
	}
	return realPath, true
}

func isImportUploadPathInside(rootPath, targetPath string) bool {
	rel, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func normalizeImportUploadValue(ids []any, urlItems []importUploadURLItem, field frontmeta.ImportField) (any, error) {
	if field.SaveMode == "url" {
		if len(urlItems) == 0 {
			return nil, nil
		}
		encoded, err := json.Marshal(urlItems)
		if err != nil {
			return nil, fmt.Errorf("序列化上传地址失败")
		}
		return string(encoded), nil
	}

	if len(ids) == 0 {
		return nil, nil
	}
	if field.Multiple {
		return ids, nil
	}
	return ids[0], nil
}

func buildImportUploadURLItem(fileRecord uploadrepo.UploadFile) (importUploadURLItem, bool) {
	publicURL := resolveImportUploadPublicURL(fileRecord)
	if publicURL == "" {
		return importUploadURLItem{}, false
	}
	return importUploadURLItem{
		Name: strings.TrimSpace(fileRecord.Name),
		URL:  publicURL,
	}, true
}

func resolveImportUploadPublicURL(fileRecord uploadrepo.UploadFile) string {
	payload := uploadrepo.BuildUploadFilePayload(fileRecord)
	return strings.TrimSpace(fmt.Sprint(payload["url"]))
}

func buildImportUploadBizKey(modelName, fieldName string) string {
	modelSegment := strings.ToLower(strings.TrimSpace(modelName))
	if index := strings.Index(modelSegment, "."); index > 0 {
		modelSegment = modelSegment[:index]
	}
	fieldSegment := strings.ToLower(strings.TrimSpace(fieldName))
	fieldSegment = strings.TrimSuffix(fieldSegment, "_id")
	fieldSegment = strings.TrimSuffix(fieldSegment, "_ids")
	return strings.Trim(strings.Join([]string{"import", modelSegment, fieldSegment}, "."), ".")
}

func normalizeImportUploadName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "_id")
	value = strings.TrimSuffix(value, "_ids")
	value = strings.ReplaceAll(value, "_", "-")
	if value == "" {
		return "upload"
	}
	return value
}

func normalizeImportUploadMime(ext string) string {
	mimeType := strings.TrimSpace(mime.TypeByExtension(ext))
	if mimeType != "" {
		return mimeType
	}
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func uniqueImportStrings(items []string) []string {
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
