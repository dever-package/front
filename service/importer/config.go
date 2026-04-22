package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shemic/dever/util"

	frontmeta "github.com/dever-package/front/service/meta"
	frontpage "github.com/dever-package/front/service/page"
)

type rawPageSchema struct {
	Nodes map[string][]map[string]any `json:"nodes"`
}

func loadPageConfig(pathValue, importKey string) (importConfig, error) {
	content, err := frontpage.ReadContent(pathValue)
	if err != nil {
		return importConfig{}, err
	}

	var payload rawPageSchema
	if err := json.Unmarshal(content, &payload); err != nil {
		return importConfig{}, fmt.Errorf("页面配置解析失败")
	}

	items := flattenPageItems(payload.Nodes)
	candidates, err := collectPageImportCandidates(items)
	if err != nil {
		return importConfig{}, err
	}

	matched, ok := matchPageImportCandidate(candidates, strings.TrimSpace(importKey))
	if !ok {
		return importConfig{}, fmt.Errorf("未找到可用的导入配置")
	}

	return matched, nil
}

func flattenPageItems(nodes map[string][]map[string]any) []map[string]any {
	items := make([]map[string]any, 0)
	for layoutID, group := range nodes {
		for index, item := range group {
			itemID := strings.TrimSpace(util.ToString(item["id"]))
			if itemID == "" {
				itemID = fmt.Sprintf("%s-%d", layoutID, index)
				item["id"] = itemID
			}
			items = append(items, item)
			for _, child := range normalizeNestedItems(item["items"]) {
				items = append(items, child)
			}
		}
	}
	return items
}

func normalizeNestedItems(raw any) []map[string]any {
	list := make([]map[string]any, 0)
	_ = parseJSONValue(raw, &list)
	return list
}

func collectPageImportCandidates(items []map[string]any) ([]importConfig, error) {
	candidates := make([]importConfig, 0)
	for _, item := range items {
		candidate, ok, err := collectItemImportCandidate(item)
		if err != nil {
			return nil, err
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func collectItemImportCandidate(item map[string]any) (importConfig, bool, error) {
	actionMap, _ := item["action"].(map[string]any)
	clickConfig, ok, err := parseImportAction(actionMap["click"])
	if err != nil {
		return importConfig{}, false, err
	}
	if !ok {
		return importConfig{}, false, nil
	}

	importKey := resolveImportCandidateKey(item, clickConfig)
	if importKey == "" {
		return importConfig{}, false, fmt.Errorf("导入项缺少 key")
	}

	modelName := strings.TrimSpace(clickConfig.Model)
	return importConfig{
		Key:          importKey,
		Name:         strings.TrimSpace(util.FirstNonEmpty(util.ToString(item["name"]), importKey)),
		Model:        modelName,
		UploadRuleID: clickConfig.UploadRuleID,
		MatchFields:  append([]string(nil), clickConfig.MatchFields...),
		MatchMode:    strings.TrimSpace(clickConfig.MatchMode),
		Fields:       append([]frontmeta.ImportField(nil), clickConfig.Fields...),
	}, true, nil
}

func matchPageImportCandidate(
	candidates []importConfig,
	targetImportKey string,
) (importConfig, bool) {
	if len(candidates) == 0 {
		return importConfig{}, false
	}

	if targetImportKey != "" {
		for _, candidate := range candidates {
			if candidate.Key == targetImportKey {
				return candidate, true
			}
		}
		return importConfig{}, false
	}

	if len(candidates) == 1 {
		return candidates[0], true
	}

	return importConfig{}, false
}

func parseImportAction(raw any) (importerActionSnapshot, bool, error) {
	if raw == nil {
		return importerActionSnapshot{}, false, nil
	}

	switch current := raw.(type) {
	case string:
		return importerActionSnapshot{}, false, nil
	case []any:
		for _, item := range current {
			config, ok, err := parseImportAction(item)
			if err != nil {
				return importerActionSnapshot{}, false, err
			}
			if ok {
				return config, true, nil
			}
		}
		return importerActionSnapshot{}, false, nil
	case map[string]any, map[string]string:
		var config importerActionSnapshot
		if err := parseJSONValue(current, &config); err != nil {
			return importerActionSnapshot{}, false, fmt.Errorf("导入动作配置格式错误")
		}

		if strings.ToLower(strings.TrimSpace(config.Type)) != "import" {
			return importerActionSnapshot{}, false, nil
		}
		return config, true, nil
	}

	return importerActionSnapshot{}, false, nil
}

func resolveImportCandidateKey(item map[string]any, action importerActionSnapshot) string {
	if key := strings.TrimSpace(action.ImportKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(util.ToString(item["key"])); key != "" {
		return key
	}
	return strings.TrimSpace(util.ToString(item["id"]))
}

func resolveImportConfig(pathValue, importKey string) (importConfig, error) {
	pageConfig, err := loadPageConfig(pathValue, importKey)
	if err != nil {
		return importConfig{}, err
	}

	modelName := strings.TrimSpace(pageConfig.Model)
	if modelName == "" {
		modelName = frontpage.DefaultModelName(pathValue)
	}
	if modelName == "" {
		return importConfig{}, fmt.Errorf("导入模型未配置")
	}

	meta, ok := frontmeta.ResolveModelImportMeta(modelName)
	if !ok || len(meta.Fields) == 0 {
		return importConfig{}, fmt.Errorf("当前模型没有可导入字段")
	}

	config := pageConfig
	config.Model = modelName
	config.MatchFields = append([]string(nil), meta.MatchFields...)
	config.MatchMode = meta.MatchMode
	config.Fields = append([]frontmeta.ImportField(nil), meta.Fields...)
	if len(pageConfig.MatchFields) > 0 {
		config.MatchFields = frontmeta.NormalizeImportMatchFields(pageConfig.MatchFields)
	}
	if strings.TrimSpace(pageConfig.MatchMode) != "" {
		config.MatchMode = frontmeta.NormalizeImportMatchMode(pageConfig.MatchMode)
	}
	if len(pageConfig.Fields) > 0 {
		config.Fields = frontmeta.ApplyImportFieldOverrides(config.Fields, pageConfig.Fields)
	}
	if config.UploadRuleID <= 0 {
		config.UploadRuleID = defaultImportUploadRuleID
	}
	return config, nil
}
