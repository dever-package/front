package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadRuleAcceptType struct {
	ID           uint64    `dorm:"primaryKey;autoIncrement;comment:规则允许类型关联ID"`
	UploadRuleID uint64    `dorm:"not null;comment:规则ID"`
	AcceptTypeID uint64    `dorm:"not null;comment:允许类型ID"`
	CreatedAt    time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

type UploadRuleAcceptTypeIndex struct {
	UploadRuleAcceptType struct{} `unique:"upload_rule_id,accept_type_id"`
}

var uploadRuleAcceptTypeSeed = []map[string]any{
	{"id": 1, "upload_rule_id": 1, "accept_type_id": 1},
	{"id": 2, "upload_rule_id": 2, "accept_type_id": 2},
	{"id": 3, "upload_rule_id": 3, "accept_type_id": 3},
	{"id": 4, "upload_rule_id": 4, "accept_type_id": 4},
	{"id": 5, "upload_rule_id": 5, "accept_type_id": 5},
	{"id": 6, "upload_rule_id": 6, "accept_type_id": 4},
	{"id": 7, "upload_rule_id": 6, "accept_type_id": 5},
}

func NewUploadRuleAcceptTypeModel() *orm.Model[UploadRuleAcceptType] {
	return orm.LoadModel[UploadRuleAcceptType]("upload_rule_accept_type", UploadRuleAcceptType{}, UploadRuleAcceptTypeIndex{}, uploadRuleAcceptTypeSeed, "id asc", "default")
}
