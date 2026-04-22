package importer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	frontmeta "github.com/dever-package/front/service/meta"
	uploadprovider "github.com/dever-package/front/service/upload/provider"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type workbookAnalysis struct {
	Sheets    []string
	SheetName string
	Columns   []columnAnalysis
}

func analyzeWorkbook(filePath, sheetName string, fields []frontmeta.ImportField) (workbookAnalysis, error) {
	workbook, err := excelize.OpenFile(filePath)
	if err != nil {
		return workbookAnalysis{}, fmt.Errorf("读取 Excel 失败: %w", err)
	}
	defer func() {
		_ = workbook.Close()
	}()

	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return workbookAnalysis{}, fmt.Errorf("Excel 中没有可读取的工作表")
	}

	selectedSheet := strings.TrimSpace(sheetName)
	if selectedSheet == "" {
		selectedSheet = sheets[0]
	}
	if !containsString(sheets, selectedSheet) {
		return workbookAnalysis{}, fmt.Errorf("工作表不存在")
	}

	rows, err := workbook.GetRows(selectedSheet)
	if err != nil {
		return workbookAnalysis{}, fmt.Errorf("读取工作表失败: %w", err)
	}

	headers := []string{}
	if len(rows) > 0 {
		headers = rows[0]
	}

	columns := make([]columnAnalysis, 0, len(headers))
	uploadCtx := buildImportUploadContext(workbook, filePath, selectedSheet)
	for index, header := range headers {
		sample := ""
		for rowIndex := 1; rowIndex < len(rows); rowIndex++ {
			value := readRowCell(rows[rowIndex], index)
			if strings.TrimSpace(value) == "" {
				continue
			}
			sample = value
			break
		}
		if sample == "" && columnHasPicture(uploadCtx, index) {
			sample = "已识别贴图"
		}

		header = normalizeColumnHeader(header, index)
		columns = append(columns, columnAnalysis{
			Index:       index,
			Header:      header,
			Sample:      sample,
			MappedField: suggestImportField(header, fields),
		})
	}

	if len(columns) == 0 {
		return workbookAnalysis{}, fmt.Errorf("未识别到表头，请确保第一行为列标题")
	}

	return workbookAnalysis{
		Sheets:    sheets,
		SheetName: selectedSheet,
		Columns:   columns,
	}, nil
}

func resolveImportFilePath(ctx context.Context, fileID uint64) (uploadrepo.UploadFile, string, error) {
	record, err := uploadrepo.FindUploadFile(ctx, fileID)
	if err != nil {
		return uploadrepo.UploadFile{}, "", err
	}

	if strings.ToLower(strings.TrimSpace(record.Storage.Type)) != "local" {
		return uploadrepo.UploadFile{}, "", fmt.Errorf("当前仅支持本地存储的 Excel 导入")
	}

	localPath := uploadprovider.ResolveLocalObjectPath(record.Path)
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(record.Name)))
	if ext != ".xlsx" {
		return uploadrepo.UploadFile{}, "", fmt.Errorf("当前仅支持 .xlsx 文件导入")
	}

	return record, localPath, nil
}

func suggestImportField(header string, fields []frontmeta.ImportField) string {
	normalizedHeader := normalizeHeaderKey(header)
	if normalizedHeader == "" {
		return ""
	}

	for _, field := range fields {
		for _, alias := range field.Aliases {
			if normalizeHeaderKey(alias) == normalizedHeader {
				return field.Field
			}
		}
	}
	return ""
}

func normalizeHeaderKey(value string) string {
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", "/", "", "\\", "", "（", "", "）", "", "(", "", ")", "", "：", "", ":", "")
	return strings.ToLower(strings.TrimSpace(replacer.Replace(value)))
}

func readRowCell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}
