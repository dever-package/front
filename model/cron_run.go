package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type CronRun struct {
	ID           uint64     `dorm:"primaryKey;autoIncrement;comment:运行记录ID"`
	CronID       uint64     `dorm:"type:bigint;not null;default:0;comment:计划任务"`
	RequestID    string     `dorm:"type:varchar(128);not null;comment:请求ID"`
	Status       string     `dorm:"type:varchar(32);not null;default:'running';comment:状态"`
	ScheduledAt  time.Time  `dorm:"comment:计划运行时间"`
	StartedAt    time.Time  `dorm:"comment:开始时间"`
	FinishedAt   *time.Time `dorm:"null;comment:完成时间"`
	PayloadJSON  string     `dorm:"type:text;not null;comment:执行参数"`
	ResultJSON   string     `dorm:"type:text;not null;default:'{}';comment:执行结果"`
	ErrorMessage string     `dorm:"type:text;not null;default:'';comment:错误信息"`
	CreatedAt    time.Time  `dorm:"comment:创建时间"`
	UpdatedAt    time.Time  `dorm:"comment:更新时间"`
}

type CronRunIndex struct {
	CronCreated    struct{} `index:"cron_id,created_at"`
	CronStatus     struct{} `index:"cron_id,status,started_at"`
	StatusStarted  struct{} `index:"status,started_at"`
	RequestID      struct{} `index:"request_id"`
	ScheduledStart struct{} `index:"scheduled_at"`
}

var cronRunCronRelation = orm.Relation{
	Field:  "cron_id",
	Option: "front.NewCronModel",
}

func NewCronRunModel() *orm.Model[CronRun] {
	return orm.LoadModel[CronRun]("计划任务运行记录", "cron_run", orm.ModelConfig{
		Index:    CronRunIndex{},
		Order:    "id desc",
		Database: "default",
		Options: map[string]any{
			"status": cronLastStatusOptions,
		},
		Relations: []orm.Relation{
			cronRunCronRelation,
		},
		Labels: map[string]string{
			"cron_id":       "计划任务",
			"request_id":    "请求ID",
			"status":        "状态",
			"scheduled_at":  "计划运行时间",
			"started_at":    "开始时间",
			"finished_at":   "完成时间",
			"payload_json":  "执行参数",
			"result_json":   "执行结果",
			"error_message": "错误信息",
			"created_at":    "创建时间",
			"updated_at":    "更新时间",
		},
	})
}
