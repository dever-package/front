package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type DataField struct {
	ID             uint64    `dorm:"primaryKey;autoIncrement;comment:数据字段ID"`
	DataTemplateID uint64    `dorm:"type:bigint;not null;comment:数据模板"`
	Name           string    `dorm:"type:varchar(128);not null;comment:字段名称"`
	FieldKey       string    `dorm:"type:varchar(128);not null;default:'';comment:字段Key"`
	FieldType      string    `dorm:"type:varchar(32);not null;default:'text';comment:字段类型"`
	DefaultValue   string    `dorm:"type:text;not null;default:'';comment:默认值"`
	Placeholder    string    `dorm:"type:varchar(255);not null;default:'';comment:占位提示"`
	HelpText       string    `dorm:"type:text;not null;default:'';comment:字段说明"`
	MaxCount       int       `dorm:"type:int;not null;default:0;comment:最大上传数量"`
	Required       bool      `dorm:"not null;default:false;comment:是否必填"`
	Status         int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort           int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt      time.Time `dorm:"comment:创建时间"`
}

type DataFieldIndex struct {
	TemplateKey    struct{} `index:"data_template_id,field_key"`
	TemplateStatus struct{} `index:"data_template_id,status,sort,id"`
	TypeStatus     struct{} `index:"field_type,status,id"`
}

var dataFieldTemplateRelation = orm.Relation{
	Field:      "data_template_id",
	Name:       "template",
	Option:     "front.NewDataTemplateModel",
	OptionKeys: []string{"name", "template_key", "cate_id"},
}

var dataFieldOptionRelation = orm.Relation{
	Field:      "options",
	Kind:       "children",
	Through:    "front.NewDataFieldOptionModel",
	OwnerField: "data_field_id",
	Order:      "sort asc,id asc",
}

func NewDataFieldModel() *orm.Model[DataField] {
	return orm.LoadModel[DataField]("数据字段", "data_field", orm.ModelConfig{
		Index:    DataFieldIndex{},
		Order:    "sort asc,id asc",
		Database: "default",
		Options: map[string]any{
			"field_type": dataFieldTypeOptions,
			"status":     dataStatusOptions,
		},
		Relations: []orm.Relation{
			dataFieldTemplateRelation,
			dataFieldOptionRelation,
		},
	})
}
