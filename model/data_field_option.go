package model

import "github.com/shemic/dever/orm"

type DataFieldOption struct {
	ID          uint64 `dorm:"primaryKey;autoIncrement;comment:字段选项ID"`
	DataFieldID uint64 `dorm:"type:bigint;not null;default:0;comment:数据字段"`
	Name        string `dorm:"type:varchar(128);not null;comment:选项名"`
	Value       string `dorm:"type:varchar(255);not null;comment:选项值"`
	Sort        int    `dorm:"type:int;not null;default:100;comment:排序"`
}

type DataFieldOptionIndex struct {
	FieldValue struct{} `unique:"data_field_id,value"`
	FieldSort  struct{} `index:"data_field_id,sort,id"`
}

var dataFieldOptionFieldRelation = orm.Relation{
	Field:      "data_field_id",
	Name:       "field",
	Option:     "front.NewDataFieldModel",
	OptionKeys: []string{"name", "field_type"},
}

func NewDataFieldOptionModel() *orm.Model[DataFieldOption] {
	return orm.LoadModel[DataFieldOption]("字段选项", "data_field_option", orm.ModelConfig{
		Index:    DataFieldOptionIndex{},
		Order:    "sort asc,id asc",
		Database: "default",
		Relations: []orm.Relation{
			dataFieldOptionFieldRelation,
		},
	})
}
