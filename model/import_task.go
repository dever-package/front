package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type ImportTask struct {
	ID           uint64     `dorm:"primaryKey;autoIncrement;comment:导入任务ID"`
	AccountID    uint64     `dorm:"type:bigint;not null;default:0;comment:账户ID"`
	PagePath     string     `dorm:"type:varchar(255);not null;comment:页面路径"`
	ImportKey    string     `dorm:"type:varchar(128);not null;comment:导入标识"`
	FileID       uint64     `dorm:"type:bigint;not null;default:0;comment:文件ID"`
	SheetName    string     `dorm:"type:varchar(255);not null;comment:工作表名称"`
	MappingJSON  string     `dorm:"type:text;not null;comment:映射配置"`
	Status       string     `dorm:"type:varchar(32);not null;default:pending;comment:状态"`
	Progress     int        `dorm:"type:int;not null;default:0;comment:进度"`
	Stage        string     `dorm:"type:varchar(255);not null;comment:阶段"`
	TotalRows    int        `dorm:"type:int;not null;default:0;comment:总行数"`
	SuccessRows  int        `dorm:"type:int;not null;default:0;comment:成功行数"`
	FailedRows   int        `dorm:"type:int;not null;default:0;comment:失败行数"`
	SummaryJSON  string     `dorm:"type:text;not null;comment:摘要数据"`
	ErrorMessage string     `dorm:"type:text;not null;comment:错误信息"`
	StartedAt    *time.Time `dorm:"comment:开始时间"`
	FinishedAt   *time.Time `dorm:"comment:完成时间"`
	CreatedAt    time.Time  `dorm:"comment:创建时间"`
	UpdatedAt    time.Time  `dorm:"comment:更新时间"`
}

type ImportTaskIndex struct {
	Status    struct{} `index:"status,created_at"`
	AccountID struct{} `index:"account_id,created_at"`
}

func NewImportTaskModel() *orm.Model[ImportTask] {
	return orm.LoadModel[ImportTask]("import_task", ImportTask{}, ImportTaskIndex{}, "id desc", "default")
}
