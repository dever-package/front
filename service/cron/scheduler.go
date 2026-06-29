package cron

import (
	"context"
	"sync"
	"time"

	dlog "github.com/shemic/dever/log"
	"github.com/shemic/dever/util"

	frontmodel "github.com/dever-package/front/model"
	"github.com/dever-package/front/service/cronexpr"
)

const (
	scanInterval = 5 * time.Second
	claimLimit   = 10
)

var startOnce sync.Once
var runtime cronRuntime

type cronRuntime struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func Start() {
	startOnce.Do(func() {
		runBootstraps(context.Background())
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		runtime.mu.Lock()
		runtime.cancel = cancel
		runtime.done = done
		runtime.mu.Unlock()
		go func() {
			defer close(done)
			loop(ctx)
		}()
	})
}

func Stop(ctx context.Context) error {
	ctx = normalizeStopContext(ctx)
	runtime.mu.Lock()
	cancel := runtime.cancel
	done := runtime.done
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func normalizeStopContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func loop(ctx context.Context) {
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		processDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processDue(ctx context.Context) {
	now := time.Now()
	rows := frontmodel.NewCronModel().SelectMap(ctx, map[string]any{
		"status":      frontmodel.CronStatusEnabled,
		"next_run_at": map[string]any{"lte": now},
	}, map[string]any{
		"order":    "main.next_run_at asc, main.id asc",
		"page":     1,
		"pageSize": claimLimit,
	})

	for _, row := range rows {
		item, ok := snapshotFromRow(row)
		if !ok {
			continue
		}
		claimed, scheduledAt := claimDue(ctx, item, now)
		if !claimed {
			continue
		}
		runClaimedCron(ctx, item, scheduledAt)
	}
}

func claimDue(ctx context.Context, item cronSnapshot, now time.Time) (bool, time.Time) {
	scheduledAt := item.NextRunAt
	if scheduledAt.IsZero() {
		scheduledAt = now
	}
	next, err := cronexpr.Next(item.Spec, item.Timezone, now)
	if err != nil {
		frontmodel.NewCronModel().Update(ctx, map[string]any{"id": item.ID}, map[string]any{
			"status":     frontmodel.CronStatusDisabled,
			"updated_at": now,
		})
		dlog.ErrorFields("front.cron.disable_invalid", "计划任务表达式无效，已停用", dlog.Fields{
			"cron_id": item.ID,
			"name":    item.Name,
			"error":   err.Error(),
		})
		return false, scheduledAt
	}

	updated := frontmodel.NewCronModel().Update(ctx, map[string]any{
		"id":          item.ID,
		"status":      frontmodel.CronStatusEnabled,
		"next_run_at": map[string]any{"lte": now},
	}, map[string]any{
		"next_run_at": next,
		"last_run_at": now,
		"updated_at":  now,
	})
	if updated == 0 {
		return false, scheduledAt
	}
	return true, scheduledAt
}

func snapshotFromRow(row map[string]any) (cronSnapshot, bool) {
	id := util.ToUint64(row["id"])
	if id == 0 {
		return cronSnapshot{}, false
	}
	return cronSnapshot{
		ID:             id,
		Name:           util.ToStringTrimmed(row["name"]),
		Status:         util.ToIntDefault(row["status"], frontmodel.CronStatusDisabled),
		Spec:           util.ToStringTrimmed(row["spec"]),
		Timezone:       util.ToStringTrimmed(row["timezone"]),
		Kind:           frontmodel.NormalizeCronKind(util.ToStringTrimmed(row["kind"])),
		Use:            util.ToStringTrimmed(row["use"]),
		PayloadJSON:    util.ToStringTrimmed(row["payload_json"]),
		TimeoutSeconds: frontmodel.NormalizeCronTimeoutSeconds(row["timeout_seconds"]),
		NextRunAt:      parseTimeValue(row["next_run_at"]),
	}, true
}

func parseTimeValue(value any) time.Time {
	switch current := value.(type) {
	case time.Time:
		return current
	case *time.Time:
		if current != nil {
			return *current
		}
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			parsed, err := time.ParseInLocation(layout, current, time.Local)
			if err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
