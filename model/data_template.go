package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type DataTemplate struct {
	ID          uint64    `dorm:"primaryKey;autoIncrement;comment:数据模板ID"`
	CateID      uint64    `dorm:"type:bigint;not null;default:0;comment:模板分类"`
	Name        string    `dorm:"type:varchar(128);not null;comment:模板名称"`
	TemplateKey string    `dorm:"type:varchar(128);not null;default:'';comment:模板Key"`
	Status      int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort        int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt   time.Time `dorm:"comment:创建时间"`
}

type DataTemplateIndex struct {
	TemplateKey struct{} `index:"template_key"`
	CateStatus  struct{} `index:"cate_id,status,sort,id"`
	Status      struct{} `index:"status,sort,id"`
}

var dataTemplateCateRelation = orm.Relation{
	Field:      "cate_id",
	Name:       "cate",
	Option:     "front.NewDataTemplateCateModel",
	OptionKeys: []string{"name", "target_id"},
}

var dataTemplateFieldRelation = orm.Relation{
	Field:      "fields",
	Kind:       "children",
	Through:    "front.NewDataFieldModel",
	OwnerField: "data_template_id",
	Order:      "sort asc,id asc",
}

func NewDataTemplateModel() *orm.Model[DataTemplate] {
	return orm.LoadModel[DataTemplate]("数据模板", "data_template", orm.ModelConfig{
		Index:    DataTemplateIndex{},
		Order:    "sort asc,id asc",
		Database: "default",
		Options: map[string]any{
			"status": dataStatusOptions,
		},
		Relations: []orm.Relation{
			dataTemplateCateRelation,
			dataTemplateFieldRelation,
		},
	})
}
