package cron

import (
	"context"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmodel "my/package/front/model"
)

type CronAction struct{}

func (CronAction) ProviderRunNow(c *server.Context, params []any) any {
	ctx := context.Background()
	if c != nil {
		ctx = c.Context()
	}

	cronID := cronActionID(params)
	if cronID == 0 {
		panic("计划任务ID不能为空")
	}

	row := frontmodel.NewCronModel().FindMap(ctx, map[string]any{"id": cronID})
	if len(row) == 0 {
		panic("计划任务不存在")
	}

	item, ok := snapshotFromRow(row)
	if !ok {
		panic("计划任务数据无效")
	}

	now := time.Now()
	runID, requestID, reason := startCronRun(ctx, item, now, now)
	if reason != "" {
		panic(reason)
	}

	frontmodel.NewCronModel().Update(ctx, map[string]any{"id": cronID}, map[string]any{
		"last_run_at": now,
		"updated_at":  now,
	})

	return map[string]any{
		"id":         cronID,
		"run_id":     runID,
		"request_id": requestID,
	}
}

func cronActionID(params []any) uint64 {
	if len(params) == 0 {
		return 0
	}
	return cronActionValueID(params[0])
}

func cronActionValueID(value any) uint64 {
	switch current := value.(type) {
	case map[string]any:
		for _, key := range []string{"id", "cron_id"} {
			if id := util.ToUint64(current[key]); id > 0 {
				return id
			}
		}
		for _, key := range []string{"payload", "data", "result"} {
			if id := cronActionValueID(current[key]); id > 0 {
				return id
			}
		}
	default:
		return util.ToUint64(current)
	}
	return 0
}
