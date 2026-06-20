package export

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/shemic/dever/util"
)

var exportWorker struct {
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

func Start() {
	exportWorker.mu.Lock()
	defer exportWorker.mu.Unlock()

	if exportWorker.stop != nil {
		return
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	exportWorker.stop = stop
	exportWorker.done = done

	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			if err := processPendingExportTask(context.Background()); err != nil {
				time.Sleep(2 * time.Second)
			}
			select {
			case <-ticker.C:
			case <-stop:
				return
			}
		}
	}()
}

func Stop(ctx context.Context) error {
	exportWorker.mu.Lock()
	stop := exportWorker.stop
	done := exportWorker.done
	if stop == nil {
		exportWorker.mu.Unlock()
		return nil
	}
	exportWorker.stop = nil
	exportWorker.done = nil
	close(stop)
	exportWorker.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func processPendingExportTask(ctx context.Context) error {
	task, ok, err := claimPendingTask(ctx)
	if err != nil || !ok {
		return err
	}

	if err := runExportTask(ctx, task); err != nil {
		finishTaskFailed(ctx, task.ID, err)
		return err
	}

	return nil
}

func runExportTask(ctx context.Context, task taskSnapshot) error {
	updateTaskProgress(ctx, task.ID, 5, "解析导出配置")
	plan, err := resolveTaskPlan(ctx, task)
	if err != nil {
		return err
	}

	updateTaskProgress(ctx, task.ID, 10, "创建工作簿")
	resultName, resultPath, err := writeWorkbookPlan(ctx, task, plan)
	if err != nil {
		return err
	}

	finishTaskSuccess(ctx, task.ID, resultName, resultPath)
	return nil
}

func writeWorkbookPlan(ctx context.Context, task taskSnapshot, plan workbookPlan) (string, string, error) {
	var workbook *excelize.File
	var err error
	if strings.TrimSpace(plan.TemplatePath) != "" {
		workbook, err = excelize.OpenFile(strings.TrimSpace(plan.TemplatePath))
		if err != nil {
			return "", "", fmt.Errorf("打开导出模板失败: %w", err)
		}
	} else {
		workbook = excelize.NewFile()
	}
	defer func() {
		_ = workbook.Close()
	}()

	styleRegistry, err := registerWorkbookStyles(workbook, plan)
	if err != nil {
		return "", "", err
	}

	for index, sheet := range plan.Sheets {
		baseProgress := 10 + index*70/max(len(plan.Sheets), 1)
		updateTaskProgress(ctx, task.ID, baseProgress, fmt.Sprintf("写入工作表：%s", sheet.Name))

		if sheet.Source != nil {
			if err := writeSourceSheet(ctx, workbook, task, sheet, styleRegistry); err != nil {
				return "", "", err
			}
			continue
		}

		if err := writeStaticSheet(workbook, sheet, styleRegistry); err != nil {
			return "", "", err
		}
	}

	if strings.TrimSpace(plan.TemplatePath) == "" {
		sheets := workbook.GetSheetList()
		if len(sheets) > 1 {
			for _, sheetName := range sheets {
				if sheetName == "Sheet1" {
					_ = workbook.DeleteSheet(sheetName)
					break
				}
			}
		}
	}

	if len(plan.Sheets) > 0 {
		for index, sheetName := range workbook.GetSheetList() {
			if sheetName == plan.Sheets[0].Name {
				workbook.SetActiveSheet(index)
				break
			}
		}
	}

	updateTaskProgress(ctx, task.ID, 95, "保存导出文件")
	resultName := sanitizeFileName(plan.FileName)
	if !strings.HasSuffix(strings.ToLower(resultName), ".xlsx") {
		resultName += ".xlsx"
	}
	resultPath, err := buildExportResultPath(task.ID, resultName)
	if err != nil {
		return "", "", err
	}
	if err := workbook.SaveAs(resultPath); err != nil {
		return "", "", fmt.Errorf("保存导出文件失败: %w", err)
	}

	return resultName, resultPath, nil
}

func writeStaticSheet(workbook *excelize.File, sheet sheetPlan, styles styleRegistry) error {
	sheetName := ensureWorkbookSheet(workbook, sheet.Name)
	startCol, startRow, err := excelize.CellNameToCoordinates(sheet.StartCell)
	if err != nil {
		return err
	}

	if err := applySheetMerges(workbook, sheetName, sheet.Merges); err != nil {
		return err
	}
	if err := applySheetCells(workbook, sheetName, sheet.Cells, styles); err != nil {
		return err
	}
	if len(sheet.Head) > 0 {
		if err := applyColumnWidths(workbook, sheetName, startCol, sheet.Head); err != nil {
			return err
		}
		headerRow := startRow
		if err := writeGridHeader(workbook, sheetName, startCol, headerRow, sheet.Head, styles.headerStyle(sheet)); err != nil {
			return err
		}
		optionLabels := resolveSourceOptionLabels(sheet.Source)
		for index, row := range sheet.Body {
			if err := writeGridRow(workbook, sheetName, startCol, headerRow+1+index, sheet.Head, row, optionLabels, styles, styles.bodyStyle(sheet)); err != nil {
				return err
			}
		}
		lastRow := headerRow + len(sheet.Body)
		if sheet.AutoFilter && len(sheet.Body) > 0 {
			rangeRef := fmt.Sprintf("%s:%s", sheet.StartCell, mustCellName(startCol+len(sheet.Head)-1, lastRow))
			_ = workbook.AutoFilter(sheetName, rangeRef, nil)
		}
	}
	if panes := buildFreezePanes(sheet.Freeze); panes != nil {
		_ = workbook.SetPanes(sheetName, panes)
	}
	return nil
}

func writeSourceSheet(
	ctx context.Context,
	workbook *excelize.File,
	task taskSnapshot,
	sheet sheetPlan,
	styles styleRegistry,
) error {
	head := sheet.Head
	optionLabels := resolveSourceOptionLabels(sheet.Source)
	part := 1
	page := 1
	writtenRows := 0
	totalRows := int64(0)
	var currentSheetName string
	var streamWriter *excelize.StreamWriter
	var currentRow int
	var startCol int
	var startRow int
	var dataRowsInPart int
	var headerRowIndex int
	var staticBodyWritten bool

	openPart := func() error {
		currentSheetName = ensureWorkbookSheet(workbook, buildSheetPartName(sheet.Name, part))
		writer, err := workbook.NewStreamWriter(currentSheetName)
		if err != nil {
			return err
		}
		streamWriter = writer
		startCol, startRow, err = excelize.CellNameToCoordinates(sheet.StartCell)
		if err != nil {
			return err
		}
		currentRow = startRow
		dataRowsInPart = 0
		headerRowIndex = 0
		if panes := buildFreezePanes(sheet.Freeze); panes != nil {
			if err := streamWriter.SetPanes(panes); err != nil {
				return err
			}
		}
		if err := applyStreamColumnWidths(streamWriter, startCol, head); err != nil {
			return err
		}
		if err := applyStreamMerges(streamWriter, sheet.Merges); err != nil {
			return err
		}
		if len(head) > 0 {
			if err := streamSetHeaderRow(streamWriter, startCol, currentRow, head, styles.headerStyle(sheet)); err != nil {
				return err
			}
			headerRowIndex = currentRow
			currentRow += 1
		}
		if !staticBodyWritten && len(sheet.Body) > 0 && len(head) > 0 {
			for _, row := range sheet.Body {
				if err := streamSetDataRow(streamWriter, startCol, currentRow, head, row, optionLabels, styles, styles.bodyStyle(sheet)); err != nil {
					return err
				}
				currentRow += 1
				dataRowsInPart += 1
			}
			staticBodyWritten = true
		}
		return nil
	}

	closePart := func() error {
		if streamWriter == nil {
			return nil
		}
		if err := streamWriter.Flush(); err != nil {
			return err
		}
		if sheet.AutoFilter && len(head) > 0 && headerRowIndex > 0 && currentRow > headerRowIndex {
			ref := fmt.Sprintf("%s:%s", mustCellName(startCol, headerRowIndex), mustCellName(startCol+len(head)-1, currentRow-1))
			_ = workbook.AutoFilter(currentSheetName, ref, nil)
		}
		streamWriter = nil
		return nil
	}

	loadPage := func() (pageResult, error) {
		switch strings.ToLower(strings.TrimSpace(sheet.Source.Mode)) {
		case "service":
			return loadServicePageResult(ctx, task, sheet, sheet.Source, page)
		default:
			return loadGenericPageResult(ctx, sheet.Source, page)
		}
	}

	for {
		result, err := loadPage()
		if err != nil {
			return err
		}
		if page == 1 && len(head) == 0 && len(result.Head) > 0 {
			head = result.Head
		}
		if len(head) == 0 && len(result.Body) > 0 {
			return fmt.Errorf("导出工作表 %s 缺少表头定义", sheet.Name)
		}
		if streamWriter == nil {
			if err := openPart(); err != nil {
				return err
			}
		}
		if totalRows == 0 && result.Total > 0 {
			totalRows = result.Total
		}

		for _, row := range result.Body {
			if currentRow > maxExcelSheetRows || dataRowsInPart >= sheet.MaxRowsPerSheet {
				if err := closePart(); err != nil {
					return err
				}
				part += 1
				if err := openPart(); err != nil {
					return err
				}
			}
			if err := streamSetDataRow(streamWriter, startCol, currentRow, head, row, optionLabels, styles, styles.bodyStyle(sheet)); err != nil {
				return err
			}
			currentRow += 1
			dataRowsInPart += 1
			writtenRows += 1
		}

		if totalRows > 0 {
			progress := 20 + int(float64(writtenRows)/float64(totalRows)*70)
			updateTaskProgress(ctx, task.ID, progress, fmt.Sprintf("已导出 %d / %d 行", writtenRows, totalRows))
		}

		if len(result.Body) == 0 {
			break
		}
		if totalRows > 0 && int64(writtenRows) >= totalRows {
			break
		}
		if len(result.Body) < sheet.Source.PageSize {
			break
		}
		page += 1
	}

	if err := closePart(); err != nil {
		return err
	}
	return nil
}

type styleRegistry struct {
	ids map[string]int
}

func registerWorkbookStyles(workbook *excelize.File, plan workbookPlan) (styleRegistry, error) {
	result := styleRegistry{ids: map[string]int{}}
	styleDefs := builtinStyleDefinitions()
	mergeStyleDefs(styleDefs, plan.StyleDefs)
	for _, sheet := range plan.Sheets {
		mergeStyleDefs(styleDefs, sheet.StyleDefs)
	}
	for name, def := range styleDefs {
		style, err := decodeExcelStyle(def)
		if err != nil {
			return result, fmt.Errorf("导出样式 %s 配置错误: %w", name, err)
		}
		styleID, err := workbook.NewStyle(style)
		if err != nil {
			return result, err
		}
		result.ids[name] = styleID
	}
	return result, nil
}

func (s styleRegistry) id(name string) int {
	return s.ids[strings.TrimSpace(name)]
}

func (s styleRegistry) headerStyle(sheet sheetPlan) int {
	if id := s.id(sheet.Styles.Header); id > 0 {
		return id
	}
	return s.id("header")
}

func (s styleRegistry) bodyStyle(sheet sheetPlan) int {
	if id := s.id(sheet.Styles.Body); id > 0 {
		return id
	}
	return s.id("body")
}

func (s styleRegistry) dataCellStyle(field exportField, fallback int) int {
	if id := s.id(field.Style); id > 0 {
		return id
	}
	switch field.Format {
	case "number":
		if id := s.id("number"); id > 0 {
			return id
		}
	case "date":
		if id := s.id("date"); id > 0 {
			return id
		}
	case "datetime":
		if id := s.id("datetime"); id > 0 {
			return id
		}
	case "currency":
		if id := s.id("currency"); id > 0 {
			return id
		}
	}
	return fallback
}

func builtinStyleDefinitions() map[string]any {
	return map[string]any{
		"title": map[string]any{
			"font":      map[string]any{"bold": true, "size": 16},
			"alignment": map[string]any{"vertical": "center"},
		},
		"header": map[string]any{
			"font": map[string]any{"bold": true, "color": "#FFFFFF"},
			"fill": map[string]any{
				"type":    "pattern",
				"pattern": 1,
				"color":   []string{"#2563EB"},
			},
			"alignment": map[string]any{"horizontal": "center", "vertical": "center"},
		},
		"body": map[string]any{
			"alignment": map[string]any{"vertical": "center"},
		},
		"summary": map[string]any{
			"font": map[string]any{"bold": true},
			"fill": map[string]any{
				"type":    "pattern",
				"pattern": 1,
				"color":   []string{"#EEF2FF"},
			},
		},
		"number": map[string]any{
			"alignment": map[string]any{"horizontal": "right", "vertical": "center"},
		},
		"date": map[string]any{
			"customNumFmt": "yyyy-mm-dd",
		},
		"datetime": map[string]any{
			"customNumFmt": "yyyy-mm-dd hh:mm:ss",
		},
		"currency": map[string]any{
			"customNumFmt": "#,##0.00",
			"alignment":    map[string]any{"horizontal": "right", "vertical": "center"},
		},
	}
}

func mergeStyleDefs(target map[string]any, source map[string]any) {
	for key, value := range source {
		target[strings.TrimSpace(key)] = value
	}
}

func decodeExcelStyle(raw any) (*excelize.Style, error) {
	style := &excelize.Style{}
	if raw == nil {
		return style, nil
	}
	if err := decodeJSONValue(raw, style); err != nil {
		return nil, err
	}
	return style, nil
}

func ensureWorkbookSheet(workbook *excelize.File, name string) string {
	for _, sheetName := range workbook.GetSheetList() {
		if sheetName == name {
			return name
		}
	}
	_, _ = workbook.NewSheet(name)
	return name
}

func applySheetMerges(workbook *excelize.File, sheetName string, merges []sheetMerge) error {
	for _, merge := range merges {
		if strings.TrimSpace(merge.Start) == "" || strings.TrimSpace(merge.End) == "" {
			continue
		}
		if err := workbook.MergeCell(sheetName, strings.TrimSpace(merge.Start), strings.TrimSpace(merge.End)); err != nil {
			return err
		}
	}
	return nil
}

func applyStreamMerges(writer *excelize.StreamWriter, merges []sheetMerge) error {
	for _, merge := range merges {
		if strings.TrimSpace(merge.Start) == "" || strings.TrimSpace(merge.End) == "" {
			continue
		}
		if err := writer.MergeCell(strings.TrimSpace(merge.Start), strings.TrimSpace(merge.End)); err != nil {
			return err
		}
	}
	return nil
}

func applySheetCells(workbook *excelize.File, sheetName string, cells []sheetCell, styles styleRegistry) error {
	for _, cell := range cells {
		cellRef := strings.TrimSpace(cell.Cell)
		if cellRef == "" {
			continue
		}
		if strings.TrimSpace(cell.Formula) != "" {
			if err := workbook.SetCellFormula(sheetName, cellRef, cell.Formula); err != nil {
				return err
			}
		} else if err := workbook.SetCellValue(sheetName, cellRef, cell.Value); err != nil {
			return err
		}
		if styleID := styles.id(cell.Style); styleID > 0 {
			if err := workbook.SetCellStyle(sheetName, cellRef, cellRef, styleID); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyColumnWidths(workbook *excelize.File, sheetName string, startCol int, fields []exportField) error {
	for index, field := range fields {
		if field.Width <= 0 {
			continue
		}
		columnName, err := excelize.ColumnNumberToName(startCol + index)
		if err != nil {
			return err
		}
		if err := workbook.SetColWidth(sheetName, columnName, columnName, field.Width); err != nil {
			return err
		}
	}
	return nil
}

func applyStreamColumnWidths(writer *excelize.StreamWriter, startCol int, fields []exportField) error {
	for index, field := range fields {
		if field.Width <= 0 {
			continue
		}
		column := startCol + index
		if err := writer.SetColWidth(column, column, field.Width); err != nil {
			return err
		}
	}
	return nil
}

func writeGridHeader(workbook *excelize.File, sheetName string, startCol, row int, fields []exportField, styleID int) error {
	for index, field := range fields {
		cellName := mustCellName(startCol+index, row)
		if err := workbook.SetCellValue(sheetName, cellName, field.Title); err != nil {
			return err
		}
		if styleID > 0 {
			if err := workbook.SetCellStyle(sheetName, cellName, cellName, styleID); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeGridRow(
	workbook *excelize.File,
	sheetName string,
	startCol,
	row int,
	fields []exportField,
	data map[string]any,
	optionLabels map[string]map[string]string,
	styles styleRegistry,
	defaultStyleID int,
) error {
	for index, field := range fields {
		cellName := mustCellName(startCol+index, row)
		cell := buildExcelCell(field, data, optionLabels, styles, defaultStyleID)
		if cell.Formula != "" {
			if err := workbook.SetCellFormula(sheetName, cellName, cell.Formula); err != nil {
				return err
			}
		} else if err := workbook.SetCellValue(sheetName, cellName, cell.Value); err != nil {
			return err
		}
		if cell.StyleID > 0 {
			if err := workbook.SetCellStyle(sheetName, cellName, cellName, cell.StyleID); err != nil {
				return err
			}
		}
	}
	return nil
}

func streamSetHeaderRow(writer *excelize.StreamWriter, startCol, row int, fields []exportField, styleID int) error {
	values := make([]interface{}, 0, len(fields))
	for _, field := range fields {
		values = append(values, excelize.Cell{
			StyleID: styleID,
			Value:   field.Title,
		})
	}
	return writer.SetRow(mustCellName(startCol, row), values)
}

func streamSetDataRow(
	writer *excelize.StreamWriter,
	startCol,
	row int,
	fields []exportField,
	data map[string]any,
	optionLabels map[string]map[string]string,
	styles styleRegistry,
	defaultStyleID int,
) error {
	values := make([]interface{}, 0, len(fields))
	for _, field := range fields {
		values = append(values, buildExcelCell(field, data, optionLabels, styles, defaultStyleID))
	}
	return writer.SetRow(mustCellName(startCol, row), values)
}

func buildExcelCell(
	field exportField,
	row map[string]any,
	optionLabels map[string]map[string]string,
	styles styleRegistry,
	defaultStyleID int,
) excelize.Cell {
	value := resolveFieldValue(row, field, optionLabels)
	styleID := styles.dataCellStyle(field, defaultStyleID)
	return excelize.Cell{
		StyleID: styleID,
		Value:   value,
	}
}

func resolveFieldValue(row map[string]any, field exportField, optionLabels map[string]map[string]string) any {
	value := lookupAnyPath(row, field.Value)
	switch field.Format {
	case "join":
		return joinFieldValues(value, field)
	case "option":
		return resolveOptionLabel(value, field, optionLabels)
	case "date", "datetime", "datetime-minute", "datetime-second":
		return normalizeDateTimeValue(value, field.Format)
	case "number", "currency":
		if value == nil {
			return nil
		}
		if number, ok := util.ParseFloat64(fmt.Sprint(value)); ok {
			return number
		}
		return fmt.Sprint(value)
	case "bool":
		return util.ToBool(value)
	default:
		return normalizeScalarValue(value, field)
	}
}

func normalizeDateTimeValue(value any, format string) any {
	switch current := value.(type) {
	case nil:
		return ""
	case time.Time:
		return formatExportDateTime(current, format)
	case *time.Time:
		if current == nil {
			return ""
		}
		return formatExportDateTime(*current, format)
	}

	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return ""
	}

	parsed, ok := parseExportDateTimeValue(text)
	if !ok {
		return text
	}

	return formatExportDateTime(parsed, format)
}

func formatExportDateTime(value time.Time, format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "date":
		return value.Format("2006-01-02")
	case "datetime-minute":
		return value.Format("2006-01-02 15:04")
	case "datetime-second", "datetime":
		return value.Format("2006-01-02 15:04:05")
	default:
		return value.Format("2006-01-02 15:04:05")
	}
}

func parseExportDateTimeValue(text string) (time.Time, bool) {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return time.Time{}, false
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, normalized)
		if err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}

func normalizeScalarValue(value any, field exportField) any {
	switch current := value.(type) {
	case nil:
		return ""
	case []any:
		return normalizeScalarListValue(current, field)
	case []map[string]any:
		return normalizeScalarListValue(normalizeAnySlice(current), field)
	case string:
		if formatted, ok := normalizeJSONStringList(current, field); ok {
			return formatted
		}
		return current
	case map[string]any:
		return formatScalarObject(current, field)
	default:
		return current
	}
}

func normalizeScalarListValue(items []any, field exportField) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(fmt.Sprint(normalizeScalarValue(item, field)))
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return joinScalarParts(parts, field)
}

func normalizeJSONStringList(value string, field exportField) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return "", false
	}

	var items []any
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return "", false
	}
	if len(items) == 0 {
		return "", true
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch current := item.(type) {
		case nil:
			continue
		case string:
			current = strings.TrimSpace(current)
			if current == "" {
				continue
			}
			parts = append(parts, current)
		case bool, float64:
			parts = append(parts, fmt.Sprint(current))
		case map[string]any:
			text := formatJSONListObject(current)
			if text == "" {
				return "", false
			}
			parts = append(parts, text)
		default:
			return "", false
		}
	}
	return joinScalarParts(parts, field), true
}

func formatJSONListObject(value map[string]any) string {
	if text := formatLabeledValue(value); text != "" {
		return text
	}

	if text := formatNamedURLValue(value); text != "" {
		return text
	}

	name := mapStringValue(value, "name")
	urlText := mapStringValue(value, "url")
	switch {
	case name != "" && urlText != "":
		return name + " | " + urlText
	case name != "":
		return name
	case urlText != "":
		return urlText
	default:
		return ""
	}
}

func formatScalarObject(value map[string]any, field exportField) string {
	if text := normalizeScalarText(lookupAnyPath(value, field.Field)); text != "" && field.Field != "" {
		return text
	}

	if text := formatLabeledValue(value); text != "" {
		return text
	}

	if text := formatNamedURLValue(value); text != "" {
		return text
	}

	for _, candidate := range []string{"name", "title", "label", "value", "url", "path", "content", "text"} {
		if text := mapStringValue(value, candidate); text != "" {
			return text
		}
	}

	return ""
}

func formatNamedURLValue(value map[string]any) string {
	name := mapStringValue(value, "name")
	urlText := mapStringValue(value, "url")
	switch {
	case name != "" && urlText != "":
		return name + " | " + urlText
	case name != "":
		return name
	case urlText != "":
		return urlText
	default:
		return ""
	}
}

func formatLabeledValue(value map[string]any) string {
	label := mapStringValue(value, "label")
	currentValue := mapStringValue(value, "value")
	switch {
	case label != "" && currentValue != "":
		return label + ": " + currentValue
	case label != "":
		return label
	case currentValue != "":
		return currentValue
	default:
		return ""
	}
}

func mapStringValue(value map[string]any, key string) string {
	current, ok := value[key]
	if !ok || current == nil {
		return ""
	}
	return normalizeScalarText(current)
}

func normalizeScalarText(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	return text
}

func joinScalarParts(parts []string, field exportField) string {
	if len(parts) == 0 {
		return ""
	}
	delimiter := field.Delimiter
	if delimiter == "" {
		delimiter = "、"
	}
	if allURLLike(parts) {
		delimiter = "\n"
	}
	return strings.Join(parts, delimiter)
}

func allURLLike(parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
			return false
		case strings.HasPrefix(part, "http://"),
			strings.HasPrefix(part, "https://"),
			strings.HasPrefix(part, "/"):
			continue
		default:
			return false
		}
	}
	return true
}

func resolveSourceOptionLabels(source *sheetSource) map[string]map[string]string {
	if source == nil || len(source.Payload) == 0 {
		return map[string]map[string]string{}
	}
	raw, ok := source.Payload["option_labels"]
	if !ok || raw == nil {
		return map[string]map[string]string{}
	}
	labels := map[string]map[string]string{}
	_ = decodeJSONValue(raw, &labels)
	return labels
}

func resolveOptionLabel(value any, field exportField, labels map[string]map[string]string) string {
	optionKey := inferExportOptionKey(field)
	rawValue := strings.TrimSpace(fmt.Sprint(value))
	if rawValue == "" {
		return ""
	}
	if optionKey == "" {
		return rawValue
	}
	if group, ok := labels[optionKey]; ok {
		if label := strings.TrimSpace(group[rawValue]); label != "" {
			return label
		}
	}
	return rawValue
}

func inferExportOptionKey(field exportField) string {
	valuePath := strings.TrimSpace(field.Value)
	if valuePath == "" {
		valuePath = strings.TrimSpace(field.Key)
	}
	if valuePath == "" {
		return ""
	}
	parts := strings.Split(valuePath, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

func joinFieldValues(value any, field exportField) string {
	fieldName := strings.TrimSpace(field.Field)
	if fieldName == "" {
		fieldName = "name"
	}
	if mapped, ok := value.(map[string]any); ok {
		return strings.TrimSpace(fmt.Sprint(lookupAnyPath(mapped, fieldName)))
	}
	items := normalizeAnySlice(value)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch current := item.(type) {
		case map[string]any:
			if text := strings.TrimSpace(fmt.Sprint(lookupAnyPath(current, fieldName))); text != "" {
				parts = append(parts, text)
			}
		default:
			text := strings.TrimSpace(fmt.Sprint(current))
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, field.Delimiter)
}

func normalizeAnySlice(value any) []any {
	switch current := value.(type) {
	case []any:
		return current
	case []map[string]any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func lookupAnyPath(value any, path string) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return value
	}
	current := value
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil
		}
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapped[segment]
	}
	return current
}

func buildFreezePanes(cell string) *excelize.Panes {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return nil
	}
	col, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		return nil
	}

	panes := &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      col - 1,
		YSplit:      row - 1,
		TopLeftCell: cell,
	}
	switch {
	case panes.XSplit > 0 && panes.YSplit > 0:
		panes.ActivePane = "bottomRight"
	case panes.XSplit > 0:
		panes.ActivePane = "topRight"
	default:
		panes.ActivePane = "bottomLeft"
	}
	return panes
}

func buildSheetPartName(name string, part int) string {
	if part <= 1 {
		return name
	}
	suffix := fmt.Sprintf("-%d", part)
	runes := []rune(name)
	if len(runes)+len([]rune(suffix)) > 31 {
		runes = runes[:31-len([]rune(suffix))]
	}
	return string(runes) + suffix
}

func buildExportResultPath(taskID uint64, fileName string) (string, error) {
	dir := filepath.Join("data", "export", time.Now().Format("20060102"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建导出目录失败: %w", err)
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "export.xlsx"
	}
	target := filepath.Join(dir, fmt.Sprintf("task-%d-%s", taskID, fileName))
	return target, nil
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "export"
	}
	name = strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_").Replace(name)
	return strings.TrimSpace(name)
}

func mustCellName(col, row int) string {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return "A1"
	}
	return cell
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func buildDownloadFilenameHeader(name string) string {
	return "attachment; filename*=UTF-8''" + url.QueryEscape(name)
}
