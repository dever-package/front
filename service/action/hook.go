package action

import (
	"fmt"
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontcall "github.com/dever-package/front/service/call"
)

type hookConfig struct {
	Type string `json:"type"`
	Use  string `json:"use"`
}

func parseHooks(value any) ([]hookConfig, error) {
	if value == nil {
		return nil, nil
	}

	items := make([]any, 0, 1)
	switch current := value.(type) {
	case []any:
		items = current
	default:
		items = append(items, current)
	}

	hooks := make([]hookConfig, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("action hook 配置格式错误")
		}

		hook := hookConfig{
			Type: strings.ToLower(util.ToStringTrimmed(raw["type"])),
			Use:  util.ToStringTrimmed(raw["use"]),
		}
		if hook.Type == "" {
			hook.Type = "service"
		}
		if hook.Type != "service" {
			return nil, fmt.Errorf("action hook.type 不支持: %s", hook.Type)
		}
		if hook.Use == "" {
			return nil, fmt.Errorf("action hook.use 不能为空")
		}

		hooks = append(hooks, hook)
	}

	return hooks, nil
}

func runBeforeHooks(c *server.Context, rawHooks any, payload any) (any, error) {
	hooks, err := parseHooks(rawHooks)
	if err != nil {
		return nil, err
	}

	current := payload
	for _, hook := range hooks {
		result, err := frontcall.Service(c, hook.Use, current)
		if err != nil {
			return nil, err
		}
		if result != nil {
			current = result
		}
	}

	return current, nil
}

func runAfterHooks(c *server.Context, rawHooks any, payload any) error {
	hooks, err := parseHooks(rawHooks)
	if err != nil {
		return err
	}

	for _, hook := range hooks {
		if _, err := frontcall.Service(c, hook.Use, payload); err != nil {
			return err
		}
	}

	return nil
}
