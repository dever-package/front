package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	frontmeta "github.com/dever-package/front/service/meta"
)

const (
	importTaskStatusPending = "pending"
	importTaskStatusRunning = "running"
	importTaskStatusSuccess = "success"
	importTaskStatusFailed  = "failed"

	defaultImportUploadRuleID = 4
	maxImportErrorSamples     = 20
)

type taskSnapshot struct {
	ID           uint64
	AccountID    uint64
	PagePath     string
	ImportKey    string
	FileID       uint64
	SheetName    string
	MappingJSON  string
	Status       string
	Progress     int
	Stage        string
	TotalRows    int
	SuccessRows  int
	FailedRows   int
	SummaryJSON  string
	ErrorMessage string
}

type importConfig struct {
	Key          string
	Name         string
	Model        string
	UploadRuleID int
	MatchFields  []string
	MatchMode    string
	Fields       []frontmeta.ImportField
}

type importFieldSetting struct {
	MissingPolicy string `json:"missingPolicy"`
	SourceMode    string `json:"sourceMode"`
	BaseDir       string `json:"baseDir"`
}

type importTaskInput struct {
	Mappings      []mappingItem                 `json:"mappings"`
	MatchFields   []string                      `json:"matchFields,omitempty"`
	MatchMode     string                        `json:"matchMode,omitempty"`
	FieldSettings map[string]importFieldSetting `json:"fieldSettings,omitempty"`
}

type importerActionSnapshot struct {
	Type         string                  `json:"type"`
	ImportKey    string                  `json:"importKey"`
	Model        string                  `json:"model"`
	UploadRuleID int                     `json:"uploadRuleId"`
	MatchFields  []string                `json:"matchFields"`
	MatchMode    string                  `json:"matchMode"`
	Fields       []frontmeta.ImportField `json:"fields"`
}

type columnAnalysis struct {
	Index       int    `json:"index"`
	Header      string `json:"header"`
	Sample      string `json:"sample"`
	MappedField string `json:"mappedField"`
}

type mappingItem struct {
	ColumnIndex int    `json:"columnIndex"`
	Field       string `json:"field"`
}

type importSummary struct {
	TotalRows   int              `json:"totalRows"`
	SuccessRows int              `json:"successRows"`
	FailedRows  int              `json:"failedRows"`
	Errors      []map[string]any `json:"errors"`
}

type importFieldPayload struct {
	Field         string   `json:"field"`
	Label         string   `json:"label"`
	Kind          string   `json:"kind"`
	Multiple      bool     `json:"multiple"`
	MissingPolicy string   `json:"missingPolicy"`
	SaveMode      string   `json:"saveMode"`
	UploadKind    string   `json:"uploadKind"`
	UploadRuleID  int      `json:"uploadRuleId"`
	SourceMode    string   `json:"sourceMode"`
	BaseDir       string   `json:"baseDir"`
	Tip           string   `json:"tip"`
	Aliases       []string `json:"aliases"`
}

type importMatchFieldPayload struct {
	Field string `json:"field"`
	Label string `json:"label"`
}

func normalizeImportFieldSetting(setting importFieldSetting) importFieldSetting {
	return importFieldSetting{
		MissingPolicy: strings.ToLower(strings.TrimSpace(setting.MissingPolicy)),
		SourceMode:    strings.ToLower(strings.TrimSpace(setting.SourceMode)),
		BaseDir:       strings.TrimSpace(setting.BaseDir),
	}
}

func normalizeImportFieldSettings(settings map[string]importFieldSetting) map[string]importFieldSetting {
	if len(settings) == 0 {
		return nil
	}

	result := make(map[string]importFieldSetting, len(settings))
	for field, setting := range settings {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		result[field] = normalizeImportFieldSetting(setting)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func taskPayload(task taskSnapshot) map[string]any {
	return map[string]any{
		"id":            task.ID,
		"status":        task.Status,
		"progress":      task.Progress,
		"stage":         task.Stage,
		"total_rows":    task.TotalRows,
		"success_rows":  task.SuccessRows,
		"failed_rows":   task.FailedRows,
		"summary":       decodeSummary(task.SummaryJSON),
		"error_message": task.ErrorMessage,
	}
}

func encodeTaskInput(input importTaskInput) string {
	data, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeTaskInput(raw string) importTaskInput {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return importTaskInput{}
	}

	var input importTaskInput
	if err := json.Unmarshal([]byte(raw), &input); err == nil && len(input.Mappings) > 0 {
		return input
	}

	var mappings []mappingItem
	if err := json.Unmarshal([]byte(raw), &mappings); err == nil {
		return importTaskInput{Mappings: mappings}
	}
	return importTaskInput{}
}

func encodeSummary(summary importSummary) string {
	data, err := json.Marshal(summary)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeSummary(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}

	result := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{}
	}
	return result
}

func normalizeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "导入失败"
	}
	return message
}

func parseJSONValue(raw any, target any) error {
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

func normalizeColumnHeader(header string, index int) string {
	header = strings.TrimSpace(header)
	if header != "" {
		return header
	}
	return fmt.Sprintf("列%d", index+1)
}

func normalizeImportFieldPayloads(fields []frontmeta.ImportField) []importFieldPayload {
	result := make([]importFieldPayload, 0, len(fields))
	for _, field := range fields {
		result = append(result, importFieldPayload{
			Field:         strings.TrimSpace(field.Field),
			Label:         strings.TrimSpace(field.Label),
			Kind:          strings.TrimSpace(field.Kind),
			Multiple:      field.Multiple,
			MissingPolicy: strings.TrimSpace(field.MissingPolicy),
			SaveMode:      strings.TrimSpace(field.SaveMode),
			UploadKind:    strings.TrimSpace(field.UploadKind),
			UploadRuleID:  field.UploadRuleID,
			SourceMode:    strings.TrimSpace(field.SourceMode),
			BaseDir:       strings.TrimSpace(field.BaseDir),
			Tip:           strings.TrimSpace(field.Tip),
			Aliases:       append([]string(nil), field.Aliases...),
		})
	}
	return result
}

func normalizeImportMatchFieldPayloads(fields []string, configs []frontmeta.ImportField) []importMatchFieldPayload {
	if len(fields) == 0 {
		return nil
	}

	fieldLabels := make(map[string]string, len(configs))
	for _, field := range configs {
		fieldName := strings.TrimSpace(field.Field)
		if fieldName == "" {
			continue
		}
		fieldLabels[fieldName] = strings.TrimSpace(field.Label)
	}

	result := make([]importMatchFieldPayload, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		label := strings.TrimSpace(fieldLabels[field])
		if label == "" {
			label = field
		}
		result = append(result, importMatchFieldPayload{
			Field: field,
			Label: label,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
