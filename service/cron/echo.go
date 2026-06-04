package cron

import (
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"
)

type CronService struct{}

func (CronService) ProviderEchoHello(_ *server.Context, params []any) any {
	payload := map[string]any{}
	for _, item := range params {
		if row, ok := item.(map[string]any); ok && row != nil {
			payload = util.CloneMap(row)
			break
		}
	}

	message := util.ToStringTrimmed(payload["message"])
	if message == "" {
		message = "hello"
	}

	return map[string]any{
		"message": message,
		"payload": payload,
		"time":    time.Now().Format(time.RFC3339),
	}
}
