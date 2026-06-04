package model

import "github.com/shemic/dever/util"

var cronProviderOptions = []map[string]any{
	{"id": "front.cron.CronService.EchoHello", "value": "测试 Echo Hello"},
}

func CronProviderOptions() []map[string]any {
	return util.CloneMapSlice(cronProviderOptions)
}

func CronProviderLabel(provider string) string {
	provider = util.ToStringTrimmed(provider)
	for _, option := range cronProviderOptions {
		if util.ToStringTrimmed(option["id"]) == provider {
			return util.ToStringTrimmed(option["value"])
		}
	}
	return provider
}
