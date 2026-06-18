package service

import (
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"
)

type TemplateDemoService struct{}

func (TemplateDemoService) ProviderLoadHomeMessage(_ *server.Context, params []any) any {
	payload := map[string]any{}
	if len(params) > 0 {
		if current, ok := params[0].(map[string]any); ok && current != nil {
			payload = util.CloneMap(current)
		}
	}

	return map[string]any{
		"title":   "Template render demo",
		"message": "This block is loaded from front.TemplateDemoService.LoadHomeMessage.",
		"path":    util.ToStringTrimmed(payload["path"]),
		"time":    time.Now().Format(time.RFC3339),
	}
}
