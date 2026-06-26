package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type DataTemplateCate struct {
	ID          uint64    `dorm:"primaryKey;autoIncrement;comment:模板分类ID"`
	Name        string    `dorm:"type:varchar(128);not null;comment:分类名称"`
	Description string    `dorm:"type:text;not null;default:'';comment:分类介绍"`
	TargetID    uint64    `dorm:"type:bigint;not null;default:0;comment:扩展数据表"`
	Status      int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort        int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt   time.Time `dorm:"comment:创建时间"`
}

type DataTemplateCateIndex struct {
	TargetStatus struct{} `index:"target_id,status,sort,id"`
	Status       struct{} `index:"status,sort,id"`
}

const DefaultDataTemplateCateID uint64 = 1

var dataTemplateCateSeed = []map[string]any{
	{"id": DefaultDataTemplateCateID, "name": "默认模板", "description": "", "target_id": 0, "status": DataStatusEnabled, "sort": 10},
}

var dataTemplateCateTargetRelation = orm.Relation{
	Field:      "target_id",
	Name:       "target",
	Option:     "front.NewDataTargetModel",
	OptionKeys: []string{"name", "model_name", "table_key", "primary_key", "label_field"},
}

func NewDataTemplateCateModel() *orm.Model[DataTemplateCate] {
	return orm.LoadModel[DataTemplateCate]("数据模板分类", "data_template_cate", orm.ModelConfig{
		Index:    DataTemplateCateIndex{},
		Seeds:    dataTemplateCateSeed,
		Order:    "sort asc,id asc",
		Database: "default",
		Options: map[string]any{
			"status": dataStatusOptions,
		},
		Relations: []orm.Relation{
			dataTemplateCateTargetRelation,
		},
	})
}
