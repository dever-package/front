package cron

import (
	"strings"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmodel "my/package/front/model"
)

type CronOptionService struct{}

func (CronOptionService) ProviderLoadCronProviders(_ *server.Context, _ []any) any {
	options := frontmodel.CronProviderOptions()
	rows := make([]map[string]any, 0, len(options))
	for _, option := range options {
		key := util.ToStringTrimmed(option["id"])
		if !strings.Contains(strings.ToLower(key), "cronservice") {
			continue
		}
		rows = append(rows, util.CloneMap(option))
	}
	return rows
}
