package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type ExportTask struct {
	ID           uint64     `dorm:"primaryKey;autoIncrement;comment:导出任务ID"`
	AccountID    uint64     `dorm:"type:bigint;not null;default:0;comment:账户ID"`
	PagePath     string     `dorm:"type:varchar(255);not null;comment:页面路径"`
	TableID      string     `dorm:"type:varchar(128);not null;comment:表格ID"`
	ExportKey    string     `dorm:"type:varchar(128);not null;comment:导出标识"`
	Status       string     `dorm:"type:varchar(32);not null;default:pending;comment:状态"`
	Progress     int        `dorm:"type:int;not null;default:0;comment:进度"`
	Stage        string     `dorm:"type:varchar(255);not null;comment:阶段"`
	QueryJSON    string     `dorm:"type:text;not null;comment:筛选参数"`
	ResultName   string     `dorm:"type:varchar(255);not null;comment:结果文件名"`
	ResultPath   string     `dorm:"type:varchar(255);not null;comment:结果路径"`
	ErrorMessage string     `dorm:"type:text;not null;comment:错误信息"`
	StartedAt    *time.Time `dorm:"comment:开始时间"`
	FinishedAt   *time.Time `dorm:"comment:完成时间"`
	CreatedAt    time.Time  `dorm:"comment:创建时间"`
	UpdatedAt    time.Time  `dorm:"comment:更新时间"`
}

type ExportTaskIndex struct {
	Status    struct{} `index:"status,created_at"`
	AccountID struct{} `index:"account_id,created_at"`
}

func NewExportTaskModel() *orm.Model[ExportTask] {
	return orm.LoadModel[ExportTask]("导出任务", "export_task", orm.ModelConfig{
		Index:    ExportTaskIndex{},
		Order:    "id desc",
		Database: "default",
	})
}
