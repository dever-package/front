package page

import (
	"fmt"
	"strings"

	"github.com/shemic/dever/util"
)

type ActionConfig struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	Model   string `json:"model"`
	PK      string `json:"pk"`
	Data    any    `json:"data"`
	Params  any    `json:"params"`
	Filters any    `json:"filters"`
	Upsert  bool   `json:"upsert"`
	Before  any    `json:"before"`
	After   any    `json:"after"`
	Api     string `json:"api"`
	Method  string `json:"method"`
	Target  string `json:"target"`
	Key     string `json:"key"`
	Value   any    `json:"value"`
}

type ActionRequest struct {
	Key     string `json:"key"`
	Payload any    `json:"payload"`
}

type actionEnvelope struct {
	Action map[string]ActionConfig `json:"action"`
}

type actionEnvelopeCacheEntry struct {
	signature ContentSignature
	envelope  actionEnvelope
}

var actionEnvelopeCache util.ConcurrentMap[ContentSignature, actionEnvelopeCacheEntry]

func NormalizeAction(config ActionConfig) ActionConfig {
	config.Type = strings.ToLower(strings.TrimSpace(config.Type))
	config.Path = normalizePath(config.Path)
	config.Model = strings.TrimSpace(config.Model)
	config.PK = strings.TrimSpace(config.PK)
	config.Key = normalizeActionKey(config.Key)
	return config
}

func SubmitModelName(content []byte, pathValue string) string {
	config, ok, err := parseNamedAction(content, "submit")
	if err == nil && ok && config.Type == "save" {
		return ActionModelName(pathValue, config)
	}

	if strings.HasSuffix(normalizePath(pathValue), "/list") {
		return ""
	}

	return DefaultModelName(pathValue)
}

func ActionModelName(pathValue string, config ActionConfig) string {
	if modelName := strings.TrimSpace(config.Model); modelName != "" {
		return modelName
	}
	return DefaultModelName(ActionPath(pathValue, config))
}

func ActionPrimaryKey(config ActionConfig) string {
	return util.FirstNonEmpty(strings.TrimSpace(config.PK), "id")
}

func ActionPath(pathValue string, config ActionConfig) string {
	if current := normalizePath(config.Path); current != "" {
		return current
	}
	return normalizePath(pathValue)
}

func ParseNamedAction(content []byte, name string) (ActionConfig, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ActionConfig{}, false, nil
	}

	envelope, err := parseActionEnvelope(content)
	if err != nil {
		return ActionConfig{}, false, err
	}

	config, ok := envelope.Action[name]
	if !ok {
		return ActionConfig{}, false, nil
	}
	return NormalizeAction(config), true, nil
}

func parseNamedAction(content []byte, name string) (ActionConfig, bool, error) {
	return ParseNamedAction(content, name)
}

func normalizeActionKey(key string) string {
	return strings.Trim(strings.TrimSpace(key), "/")
}

func parseActionEnvelope(content []byte) (actionEnvelope, error) {
	signature := Signature(content)
	if cached, ok := actionEnvelopeCache.Load(signature); ok {
		return cached.envelope, nil
	}

	var envelope actionEnvelope
	if err := util.UnmarshalNormalizedJSON(content, &envelope); err != nil {
		return actionEnvelope{}, fmt.Errorf("页面 action 配置解析失败")
	}

	actionEnvelopeCache.Store(signature, actionEnvelopeCacheEntry{
		signature: signature,
		envelope:  envelope,
	})
	return envelope, nil
}
