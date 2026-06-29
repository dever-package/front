package model

import (
	"strings"
	"sync"

	"github.com/shemic/dever/util"
)

var cronProviderOptions = []map[string]any{
	{"id": "front.cron.CronService.EchoHello", "value": "测试 Echo Hello"},
}

var cronProviderMu sync.RWMutex

func RegisterCronProvider(id string, value string) {
	id = strings.TrimSpace(id)
	value = strings.TrimSpace(value)
	if id == "" {
		return
	}
	if value == "" {
		value = id
	}

	cronProviderMu.Lock()
	defer cronProviderMu.Unlock()
	for _, option := range cronProviderOptions {
		if util.ToStringTrimmed(option["id"]) == id {
			option["value"] = value
			return
		}
	}
	cronProviderOptions = append(cronProviderOptions, map[string]any{
		"id":    id,
		"value": value,
	})
}

func CronProviderOptions() []map[string]any {
	cronProviderMu.RLock()
	defer cronProviderMu.RUnlock()
	return util.CloneMapSlice(cronProviderOptions)
}

func CronProviderLabel(provider string) string {
	provider = util.ToStringTrimmed(provider)
	for _, option := range CronProviderOptions() {
		if util.ToStringTrimmed(option["id"]) == provider {
			return util.ToStringTrimmed(option["value"])
		}
	}
	return provider
}
