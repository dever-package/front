package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shemic/dever/load"
	dlog "github.com/shemic/dever/log"
	"github.com/shemic/dever/util"

	frontmodel "github.com/dever-package/front/model"
)

type cronSnapshot struct {
	ID             uint64
	Name           string
	Status         int
	Spec           string
	Timezone       string
	Kind           string
	Use            string
	PayloadJSON    string
	TimeoutSeconds int
	NextRunAt      time.Time
}

const (
	cronStartReasonRunning      = "上一次运行尚未结束"
	cronStartReasonCreateFailed = "创建计划任务运行记录失败"
)

var cronRunLocks [64]sync.Mutex

func runClaimedCron(parent context.Context, item cronSnapshot, scheduledAt time.Time) {
	now := time.Now()
	_, _, reason := startCronRun(parent, item, scheduledAt, now)
	if reason == cronStartReasonRunning {
		ctx := parent
		if ctx == nil {
			ctx = context.Background()
		}
		recordSkippedRun(ctx, item, scheduledAt, now, "上一次运行尚未结束")
	}
}

func startCronRun(parent context.Context, item cronSnapshot, scheduledAt time.Time, now time.Time) (uint64, string, string) {
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}

	lock := cronRunLock(item.ID)
	lock.Lock()
	defer lock.Unlock()

	failExpiredRuns(ctx, item, now)
	if hasRunningRun(ctx, item.ID) {
		return 0, "", cronStartReasonRunning
	}

	requestID := buildRequestID(item.ID, now)
	runID := createRun(ctx, item, requestID, scheduledAt, now)
	if runID == 0 {
		return 0, "", cronStartReasonCreateFailed
	}

	go executeRun(context.Background(), item, runID, requestID)
	return runID, requestID, ""
}

func cronRunLock(cronID uint64) *sync.Mutex {
	index := int(cronID % uint64(len(cronRunLocks)))
	return &cronRunLocks[index]
}

func executeRun(parent context.Context, item cronSnapshot, runID uint64, requestID string) {
	timeout := frontmodel.FormatCronRunTimeout(item.TimeoutSeconds)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	payload := decodeJSONObject(item.PayloadJSON)
	payload["request_id"] = firstNonEmptyString(util.ToStringTrimmed(payload["request_id"]), requestID)
	payload["_cron_id"] = item.ID
	payload["_cron_run_id"] = runID
	payload["_cron_name"] = item.Name

	result, err := callProvider(ctx, item.Use, payload)
	if err != nil {
		finishRun(ctx, runID, frontmodel.CronRunStatusFailed, nil, err)
		return
	}
	finishRun(ctx, runID, frontmodel.CronRunStatusSuccess, result, nil)
}

func callProvider(ctx context.Context, serviceName string, payload map[string]any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = providerPanicError(r)
		}
	}()

	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return nil, fmt.Errorf("内部业务不能为空")
	}
	if result, ok, err := callRegisteredProvider(ctx, serviceName, payload); ok {
		return result, err
	}
	return load.Service(serviceName, payload, ctx), nil
}

func createRun(ctx context.Context, item cronSnapshot, requestID string, scheduledAt time.Time, startedAt time.Time) uint64 {
	runID := util.ToUint64(frontmodel.NewCronRunModel().Insert(ctx, map[string]any{
		"cron_id":       item.ID,
		"request_id":    requestID,
		"status":        frontmodel.CronRunStatusRunning,
		"scheduled_at":  scheduledAt,
		"started_at":    startedAt,
		"payload_json":  item.PayloadJSON,
		"result_json":   "{}",
		"error_message": "",
		"created_at":    startedAt,
		"updated_at":    startedAt,
	}))
	if runID == 0 {
		dlog.ErrorFields("front.cron.create_run_failed", "创建计划任务运行记录失败", dlog.Fields{
			"cron_id": item.ID,
			"name":    item.Name,
		})
	}
	return runID
}

func recordSkippedRun(ctx context.Context, item cronSnapshot, scheduledAt time.Time, now time.Time, reason string) {
	frontmodel.NewCronRunModel().Insert(ctx, map[string]any{
		"cron_id":       item.ID,
		"request_id":    buildRequestID(item.ID, now),
		"status":        frontmodel.CronRunStatusSkipped,
		"scheduled_at":  scheduledAt,
		"started_at":    now,
		"finished_at":   now,
		"payload_json":  item.PayloadJSON,
		"result_json":   "{}",
		"error_message": reason,
		"created_at":    now,
		"updated_at":    now,
	})
}

func finishRun(ctx context.Context, runID uint64, status string, result any, runErr error) {
	now := time.Now()
	message := ""
	if runErr != nil {
		message = strings.TrimSpace(runErr.Error())
	}
	if message == "" && status == frontmodel.CronRunStatusFailed {
		message = "计划任务执行失败"
	}

	updated := frontmodel.NewCronRunModel().Update(ctx, map[string]any{
		"id":     runID,
		"status": frontmodel.CronRunStatusRunning,
	}, map[string]any{
		"status":        status,
		"finished_at":   now,
		"result_json":   encodeJSONValue(result),
		"error_message": message,
		"updated_at":    now,
	})
	if updated == 0 {
		return
	}
}

func hasRunningRun(ctx context.Context, cronID uint64) bool {
	return frontmodel.NewCronRunModel().Count(ctx, map[string]any{
		"cron_id": cronID,
		"status":  frontmodel.CronRunStatusRunning,
	}) > 0
}

func failExpiredRuns(ctx context.Context, item cronSnapshot, now time.Time) {
	timeout := frontmodel.FormatCronRunTimeout(item.TimeoutSeconds)
	deadline := now.Add(-timeout)
	frontmodel.NewCronRunModel().Update(ctx, map[string]any{
		"cron_id":    item.ID,
		"status":     frontmodel.CronRunStatusRunning,
		"started_at": map[string]any{"lte": deadline},
	}, map[string]any{
		"status":        frontmodel.CronRunStatusFailed,
		"finished_at":   now,
		"error_message": "执行超时",
		"updated_at":    now,
	})
}

func buildRequestID(cronID uint64, now time.Time) string {
	return fmt.Sprintf("cron-%d-%d", cronID, now.UnixNano())
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
