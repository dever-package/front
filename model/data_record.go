package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type DataRecord struct {
	ID             uint64    `dorm:"primaryKey;autoIncrement;comment:数据记录ID"`
	DataTemplateID uint64    `dorm:"type:bigint;not null;comment:数据模板"`
	TargetID       uint64    `dorm:"type:bigint;not null;default:0;comment:扩展数据表"`
	TargetRecordID uint64    `dorm:"type:bigint;not null;default:0;comment:扩展记录ID"`
	RecordJSON     string    `dorm:"type:text;not null;default:'{}';comment:记录JSON"`
	Summary        string    `dorm:"type:varchar(255);not null;default:'';comment:摘要"`
	Status         int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort           int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt      time.Time `dorm:"comment:创建时间"`
	UpdatedAt      time.Time `dorm:"comment:更新时间"`
}

type DataRecordIndex struct {
	TemplateTargetRecord struct{} `unique:"data_template_id,target_id,target_record_id"`
	TemplateStatus       struct{} `index:"data_template_id,status,id"`
	TargetRecord         struct{} `index:"target_id,target_record_id,data_template_id,status,id"`
}

var dataRecordTemplateRelation = orm.Relation{
	Field:      "data_template_id",
	Name:       "template",
	Option:     "front.NewDataTemplateModel",
	OptionKeys: []string{"name", "template_key", "cate_id"},
}

var dataRecordTargetRelation = orm.Relation{
	Field:      "target_id",
	Name:       "target",
	Option:     "front.NewDataTargetModel",
	OptionKeys: []string{"name", "model_name", "table_key", "primary_key", "label_field"},
}

func NewDataRecordModel() *orm.Model[DataRecord] {
	return orm.LoadModel[DataRecord]("模板数据", "data_record", orm.ModelConfig{
		Index:    DataRecordIndex{},
		Order:    "id desc",
		Database: "default",
		Options: map[string]any{
			"status": dataStatusOptions,
		},
		Relations: []orm.Relation{
			dataRecordTemplateRelation,
			dataRecordTargetRelation,
		},
	})
}
