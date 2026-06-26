package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type DataTarget struct {
	ID         uint64    `dorm:"primaryKey;autoIncrement;comment:扩展数据表ID"`
	Name       string    `dorm:"type:varchar(128);not null;comment:名称"`
	ModelName  string    `dorm:"type:varchar(255);not null;comment:Model"`
	TableKey   string    `dorm:"type:varchar(128);not null;default:'';comment:表标识"`
	PrimaryKey string    `dorm:"type:varchar(64);not null;default:'id';comment:主键字段"`
	LabelField string    `dorm:"type:varchar(64);not null;default:'name';comment:展示字段"`
	Status     int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort       int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt  time.Time `dorm:"comment:创建时间"`
}

type DataTargetIndex struct {
	ModelName struct{} `unique:"model_name"`
	Status    struct{} `index:"status,sort,id"`
}

func NewDataTargetModel() *orm.Model[DataTarget] {
	return orm.LoadModel[DataTarget]("扩展数据表", "data_target", orm.ModelConfig{
		Index:    DataTargetIndex{},
		Order:    "sort asc,id asc",
		Database: "default",
		Options: map[string]any{
			"status": dataStatusOptions,
		},
	})
}
