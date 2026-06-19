package action

import (
	"fmt"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontcall "github.com/dever-package/front/service/internal/call"
)

type hookConfig struct {
	Service string `json:"service"`
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
			Service: util.ToStringTrimmed(raw["service"]),
		}
		if hook.Service == "" {
			return nil, fmt.Errorf("action hook.service 不能为空")
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
		result, err := frontcall.Service(c, hook.Service, current)
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
		if _, err := frontcall.Service(c, hook.Service, payload); err != nil {
			return err
		}
	}

	return nil
}
