package export

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	taskStatusPending  = "pending"
	taskStatusRunning  = "running"
	taskStatusSuccess  = "success"
	taskStatusFailed   = "failed"
	taskStatusCanceled = "canceled"

	defaultExportPageSize  = 1000
	defaultMaxRowsPerSheet = 1000000
	maxExcelSheetRows      = 1048576
)

type taskSnapshot struct {
	ID           uint64
	AccountID    uint64
	PagePath     string
	TableID      string
	ExportKey    string
	Status       string
	Progress     int
	Stage        string
	Query        map[string]string
	ResultName   string
	ResultPath   string
	ErrorMessage string
}

type exportConfig struct {
	Key          string            `json:"key"`
	Name         string            `json:"name"`
	Mode         string            `json:"mode"`
	Source       string            `json:"source"`
	Scope        string            `json:"scope"`
	Use          string            `json:"use"`
	Model        string            `json:"model"`
	FileName     string            `json:"fileName"`
	SheetName    string            `json:"sheetName"`
	TemplatePath string            `json:"templatePath"`
	PageSize     int               `json:"pageSize"`
	Filters      map[string]string `json:"filters"`
	Fields       []exportField     `json:"fields"`
	Workbook     *workbookPlan     `json:"workbook"`
	StyleDefs    map[string]any    `json:"styleDefs"`
}

type workbookPlan struct {
	FileName     string         `json:"fileName"`
	TemplatePath string         `json:"templatePath"`
	StyleDefs    map[string]any `json:"styleDefs"`
	Sheets       []sheetPlan    `json:"sheets"`
}

type sheetPlan struct {
	Name            string           `json:"name"`
	StartCell       string           `json:"startCell"`
	Stream          bool             `json:"stream"`
	Freeze          string           `json:"freeze"`
	AutoFilter      bool             `json:"autoFilter"`
	MaxRowsPerSheet int              `json:"maxRowsPerSheet"`
	Head            []exportField    `json:"head"`
	Body            []map[string]any `json:"body"`
	Cells           []sheetCell      `json:"cells"`
	Merges          []sheetMerge     `json:"merges"`
	Source          *sheetSource     `json:"source"`
	Styles          sheetStyleRefs   `json:"styles"`
	StyleDefs       map[string]any   `json:"styleDefs"`
}

type sheetSource struct {
	Mode     string            `json:"mode"`
	Model    string            `json:"model"`
	Service  string            `json:"service"`
	PageSize int               `json:"pageSize"`
	Query    map[string]string `json:"query"`
	Table    map[string]any    `json:"table"`
	Payload  map[string]any    `json:"payload"`
}

type exportField struct {
	Key       string  `json:"key"`
	Value     string  `json:"value"`
	Title     string  `json:"title"`
	Format    string  `json:"format"`
	Style     string  `json:"style"`
	Field     string  `json:"field"`
	Delimiter string  `json:"delimiter"`
	Width     float64 `json:"width"`
}

type sheetCell struct {
	Cell    string `json:"cell"`
	Value   any    `json:"value"`
	Formula string `json:"formula"`
	Style   string `json:"style"`
}

type sheetMerge struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type sheetStyleRefs struct {
	Title   string `json:"title"`
	Header  string `json:"header"`
	Body    string `json:"body"`
	Summary string `json:"summary"`
}

type pageConfigSnapshot struct {
	PageTitle string
	Data      map[string]any
	TableItem map[string]any
	Export    exportConfig
}

type pageResult struct {
	Head  []exportField
	Body  []map[string]any
	Total int64
}

func decodeJSONValue(raw any, target any) error {
	if raw == nil {
		return nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, target)
}

func normalizeWorkbookPlan(plan workbookPlan) workbookPlan {
	plan.FileName = strings.TrimSpace(plan.FileName)
	if plan.FileName == "" {
		plan.FileName = "导出结果"
	}
	plan.TemplatePath = strings.TrimSpace(plan.TemplatePath)
	for index := range plan.Sheets {
		current := &plan.Sheets[index]
		current.Name = normalizeSheetName(current.Name, index+1)
		current.StartCell = normalizeStartCell(current.StartCell)
		if current.MaxRowsPerSheet <= 0 {
			current.MaxRowsPerSheet = defaultMaxRowsPerSheet
		}
		if current.MaxRowsPerSheet > maxExcelSheetRows-1 {
			current.MaxRowsPerSheet = maxExcelSheetRows - 1
		}
		if current.Source != nil {
			current.Source.Mode = strings.ToLower(strings.TrimSpace(current.Source.Mode))
			if current.Source.PageSize <= 0 {
				current.Source.PageSize = defaultExportPageSize
			}
		}
		for fieldIndex := range current.Head {
			current.Head[fieldIndex] = normalizeExportField(current.Head[fieldIndex])
		}
	}
	return plan
}

func normalizeExportField(field exportField) exportField {
	field.Key = strings.TrimSpace(field.Key)
	field.Value = strings.TrimSpace(field.Value)
	field.Title = strings.TrimSpace(field.Title)
	field.Format = strings.ToLower(strings.TrimSpace(field.Format))
	field.Style = strings.TrimSpace(field.Style)
	field.Field = strings.TrimSpace(field.Field)
	field.Delimiter = strings.TrimSpace(field.Delimiter)
	if field.Delimiter == "" {
		field.Delimiter = "、"
	}
	if field.Key == "" {
		field.Key = field.Value
	}
	if field.Value == "" {
		field.Value = field.Key
	}
	if field.Title == "" {
		field.Title = field.Key
	}
	return field
}

func normalizeStartCell(cell string) string {
	cell = strings.ToUpper(strings.TrimSpace(cell))
	if cell == "" {
		return "A1"
	}
	return cell
}

func normalizeSheetName(name string, index int) string {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		normalized = fmt.Sprintf("Sheet%d", index)
	}
	normalized = strings.NewReplacer("\\", "", "/", "", "?", "", "*", "", "[", "", "]", "", ":", "").Replace(normalized)
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		normalized = fmt.Sprintf("Sheet%d", index)
	}
	runes := []rune(normalized)
	if len(runes) > 31 {
		return string(runes[:31])
	}
	return normalized
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
