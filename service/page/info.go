package page

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmeta "my/package/front/service/meta"
	"my/package/front/service/siteconfig"
)

type Schema struct {
	Page   json.RawMessage `json:"page"`
	Layout json.RawMessage `json:"layout"`
	Nodes  json.RawMessage `json:"nodes"`
	Data   json.RawMessage `json:"data"`
	State  json.RawMessage `json:"state"`
	Action json.RawMessage `json:"action"`
}

type schemaCacheEntry struct {
	signature ContentSignature
	schema    Schema
}

var schemaCache util.ConcurrentMap[string, schemaCacheEntry]

func GetInfo(c *server.Context, pathValue string) error {
	currentSchema, err := BuildInfo(c, pathValue)
	if err != nil {
		return c.Error(err)
	}

	return c.JSON(currentSchema)
}

func BuildInfo(c *server.Context, pathValue string) (Schema, error) {
	pageName := siteconfig.PageFromContext(c.Context())
	content, err := ReadContentForPage(pageName, pathValue)
	if err != nil {
		return Schema{}, err
	}

	currentSchema, err := parseSchema(pageName, pathValue, content)
	if err != nil {
		return Schema{}, err
	}

	defaultedData, err := applyNodeDefaults(c, currentSchema.Layout, currentSchema.Nodes, currentSchema.Data, pathValue)
	if err != nil {
		return Schema{}, err
	}

	resolvedData, err := resolvePageData(c, defaultedData, content, pathValue)
	if err != nil {
		return Schema{}, err
	}
	currentSchema.Data = resolvedData

	return currentSchema, nil
}

func parseSchema(pageName, pathValue string, content []byte) (Schema, error) {
	signature := Signature(content)
	cacheKey := pageName + ":" + pathValue
	if cached, ok := schemaCache.Load(cacheKey); ok {
		entry := cached
		if entry.signature == signature {
			return entry.schema, nil
		}
	}

	var current Schema
	if err := json.Unmarshal(content, &current); err != nil {
		return Schema{}, fmt.Errorf("页面配置解析失败")
	}
	if normalizedPage, err := applyPageMetaDefaults(current.Page, pathValue); err != nil {
		return Schema{}, fmt.Errorf("页面 page 默认值解析失败")
	} else {
		current.Page = normalizedPage
	}
	if normalizedNodes, err := applyNodeLabels(current.Nodes, pathValue, content); err != nil {
		return Schema{}, fmt.Errorf("页面 nodes 标签解析失败")
	} else {
		current.Nodes = normalizedNodes
	}

	schemaCache.Store(cacheKey, schemaCacheEntry{
		signature: signature,
		schema:    current,
	})
	return current, nil
}

func applyPageMetaDefaults(rawPage json.RawMessage, pathValue string) (json.RawMessage, error) {
	page := map[string]any{}
	if len(rawPage) > 0 {
		if err := json.Unmarshal(rawPage, &page); err != nil {
			return nil, err
		}
	}

	defaultTitle := DefaultPageTitle(pathValue)
	if defaultTitle == "" {
		if len(rawPage) == 0 {
			return json.RawMessage(`{}`), nil
		}
		return rawPage, nil
	}

	changed := false
	if strings.TrimSpace(util.ToString(page["name"])) == "" {
		page["name"] = defaultTitle
		changed = true
	}
	if strings.TrimSpace(util.ToString(page["title"])) == "" {
		page["title"] = util.FirstNonEmpty(util.ToString(page["name"]), defaultTitle)
		changed = true
	}
	if !changed {
		return rawPage, nil
	}

	content, err := json.Marshal(page)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(content), nil
}

func resolvePageData(
	c *server.Context,
	raw json.RawMessage,
	content []byte,
	pathValue string,
) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("页面 data 解析失败")
	}

	collectedOptions := map[string]any{}
	resolved, err := resolveDataValue(c, "", payload, collectedOptions, pathValue)
	if err != nil {
		return nil, err
	}

	if len(collectedOptions) > 0 {
		if rootData, ok := resolved.(map[string]any); ok {
			existing, _ := rootData["option"].(map[string]any)
			rootData["option"] = frontmeta.MergeOptionMap(existing, collectedOptions)
			resolved = rootData
		}
	}

	saveOptions := resolveSubmitModelFrontOption(content, pathValue)
	if len(saveOptions) > 0 {
		if rootData, ok := resolved.(map[string]any); ok {
			existing, _ := rootData["option"].(map[string]any)
			rootData["option"] = frontmeta.MergeOptionMap(existing, saveOptions)
			resolved = rootData
		}
	}

	resolvedContent, err := json.Marshal(resolved)
	if err != nil {
		return nil, fmt.Errorf("页面 data 编码失败")
	}
	return json.RawMessage(resolvedContent), nil
}
