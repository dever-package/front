package model

import (
	"context"
	"time"

	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"

	"my/package/front/service/cronexpr"
)

const (
	CronStatusDisabled = 0
	CronStatusEnabled  = 1

	CronKindProvider = "provider"

	CronScheduleEverySeconds = "every_seconds"
	CronScheduleEveryMinutes = "every_minutes"
	CronScheduleHourly       = "hourly"
	CronScheduleDaily        = "daily"
	CronScheduleWeekly       = "weekly"
	CronScheduleMonthly      = "monthly"
	CronScheduleCron         = "cron"

	CronRunStatusRunning = "running"
	CronRunStatusSuccess = "success"
	CronRunStatusFailed  = "failed"
	CronRunStatusSkipped = "skipped"
)

type Cron struct {
	ID             uint64     `dorm:"primaryKey;autoIncrement;comment:计划任务ID"`
	Name           string     `dorm:"type:varchar(128);not null;comment:任务名称"`
	Status         int        `dorm:"type:int;not null;default:1;comment:状态"`
	Spec           string     `dorm:"type:varchar(128);not null;comment:Cron表达式"`
	ScheduleMode   string     `dorm:"type:varchar(32);not null;default:'every_minutes';comment:触发规则"`
	ScheduleConfig string     `dorm:"type:text;not null;default:'{}';comment:触发配置"`
	Timezone       string     `dorm:"type:varchar(64);not null;default:'Asia/Shanghai';comment:时区"`
	Kind           string     `dorm:"type:varchar(32);not null;default:'provider';comment:执行类型"`
	Use            string     `dorm:"type:varchar(255);not null;comment:内部业务"`
	PayloadJSON    string     `dorm:"type:text;not null;comment:执行参数"`
	TimeoutSeconds int        `dorm:"type:int;not null;default:300;comment:超时时间"`
	NextRunAt      *time.Time `dorm:"null;comment:下次运行时间"`
	LastRunAt      *time.Time `dorm:"null;comment:最近运行时间"`
	CreatedAt      time.Time  `dorm:"comment:创建时间"`
	UpdatedAt      time.Time  `dorm:"comment:更新时间"`
}

type CronIndex struct {
	Name          struct{} `unique:"name"`
	StatusNextRun struct{} `index:"status,next_run_at"`
}

var (
	cronStatusOptions = []map[string]any{
		{"id": CronStatusEnabled, "value": "启用", "label": "启用", "color": "#0f766e"},
		{"id": CronStatusDisabled, "value": "停用", "label": "停用", "color": "#737373"},
	}

	cronKindOptions = []map[string]any{
		{"id": CronKindProvider, "value": "内部业务"},
	}

	cronScheduleModeOptions = []map[string]any{
		{"id": CronScheduleEverySeconds, "value": "每隔N秒"},
		{"id": CronScheduleEveryMinutes, "value": "每隔N分钟"},
		{"id": CronScheduleHourly, "value": "每小时"},
		{"id": CronScheduleDaily, "value": "每天"},
		{"id": CronScheduleWeekly, "value": "每周"},
		{"id": CronScheduleMonthly, "value": "每月"},
		{"id": CronScheduleCron, "value": "高级Cron"},
	}

	cronLastStatusOptions = []map[string]any{
		{"id": "", "value": "未运行", "label": "未运行", "color": "#737373"},
		{"id": CronRunStatusRunning, "value": "运行中", "label": "运行中", "color": "#2563eb"},
		{"id": CronRunStatusSuccess, "value": "成功", "label": "成功", "color": "#0f766e"},
		{"id": CronRunStatusFailed, "value": "失败", "label": "失败", "color": "#dc2626"},
		{"id": CronRunStatusSkipped, "value": "已跳过", "label": "已跳过", "color": "#d97706"},
	}
)

type CronModel struct {
	*orm.Model[Cron]
}

var cronParamRelation = orm.Relation{
	Field:      "params",
	Through:    "front.NewCronParamModel",
	OwnerField: "cron_id",
	Order:      "sort asc, id asc",
}

func NewCronModel() *CronModel {
	return &CronModel{Model: orm.LoadModel[Cron]("计划任务", "cron", orm.ModelConfig{
		Index:    CronIndex{},
		Order:    "id desc",
		Database: "default",
		Options: map[string]any{
			"schedule_mode": cronScheduleModeOptions,
			"status":        cronStatusOptions,
			"kind":          cronKindOptions,
		},
		Relations: []orm.Relation{
			cronParamRelation,
		},
		Labels: map[string]string{
			"name":            "任务名称",
			"status":          "状态",
			"spec":            "Cron表达式",
			"schedule_mode":   "触发规则",
			"schedule_config": "触发配置",
			"timezone":        "时区",
			"kind":            "执行类型",
			"use":             "内部业务",
			"payload_json":    "执行参数",
			"timeout_seconds": "超时时间",
			"next_run_at":     "下次运行时间",
			"last_run_at":     "最近运行时间",
			"created_at":      "创建时间",
			"updated_at":      "更新时间",
		},
	})}
}

func (m *CronModel) AfterSave(ctx context.Context, id any, _ map[string]any, _ bool) error {
	cronID := util.ToUint64(id)
	if cronID == 0 {
		return nil
	}

	payloadJSON, err := BuildCronPayloadJSON(ctx, cronID)
	if err != nil {
		return err
	}

	row := m.FindMap(ctx, map[string]any{"id": cronID})
	if len(row) == 0 {
		return nil
	}

	next := any(nil)
	if util.ToIntDefault(row["status"], CronStatusDisabled) == CronStatusEnabled {
		nextRunAt, err := cronexpr.Next(util.ToStringTrimmed(row["spec"]), util.ToStringTrimmed(row["timezone"]), time.Now())
		if err != nil {
			return err
		}
		next = nextRunAt
	}

	m.Update(ctx, map[string]any{"id": cronID}, map[string]any{
		"payload_json": payloadJSON,
		"next_run_at":  next,
		"updated_at":   time.Now(),
	})
	return nil
}

func (m *CronModel) AfterDelete(ctx context.Context, payload any) error {
	ids := cronDeleteIDs(payload)
	if len(ids) == 0 {
		return nil
	}
	NewCronRunModel().Delete(ctx, map[string]any{"cron_id": ids})
	NewCronParamModel().Delete(ctx, map[string]any{"cron_id": ids})
	return nil
}

func cronDeleteIDs(payload any) []any {
	switch current := payload.(type) {
	case map[string]any:
		if id := util.ToUint64(current["id"]); id > 0 {
			return []any{id}
		}
	case []any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			switch value := item.(type) {
			case map[string]any:
				if id := util.ToUint64(value["id"]); id > 0 {
					result = append(result, id)
				}
			default:
				if id := util.ToUint64(value); id > 0 {
					result = append(result, id)
				}
			}
		}
		return result
	default:
		if id := util.ToUint64(current); id > 0 {
			return []any{id}
		}
	}
	return nil
}

func NormalizeCronKind(value string) string {
	if value == CronKindProvider {
		return CronKindProvider
	}
	return CronKindProvider
}

func NormalizeCronTimeoutSeconds(value any) int {
	timeout := util.ToIntDefault(value, 300)
	if timeout <= 0 {
		return 300
	}
	if timeout > 86400 {
		return 86400
	}
	return timeout
}

func RequireCronEnabledStatus(value any) int {
	status := util.ToIntDefault(value, CronStatusEnabled)
	if status == CronStatusDisabled {
		return CronStatusDisabled
	}
	return CronStatusEnabled
}

func NormalizeCronScheduleMode(value string) string {
	switch value {
	case CronScheduleEverySeconds, CronScheduleEveryMinutes, CronScheduleHourly, CronScheduleDaily, CronScheduleWeekly, CronScheduleMonthly, CronScheduleCron:
		return value
	default:
		return CronScheduleEveryMinutes
	}
}

func FormatCronRunTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return time.Duration(timeoutSeconds) * time.Second
}
