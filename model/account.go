package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type Account struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:账户ID"`
	Name      string    `dorm:"type:varchar(64);not null;comment:姓名"`
	Account   string    `dorm:"type:varchar(32);not null;comment:账户"`
	Password  string    `dorm:"type:varchar(128);not null;comment:密码"`
	RoleID    uint64    `dorm:"not null;default:1;comment:角色"`
	CreatedAt time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

type AccountIndex struct {
	Account struct{} `unique:"account"`
}

var accountRoleRelation = orm.Relation{
	Field:        "role_ids",
	Through:      "front.NewAccountRoleModel",
	Option:       "front.NewRoleModel",
	ThroughOrder: "id asc",
	OptionKeys:   []string{"role"},
}

func NewAccountModel() *orm.Model[Account] {
	return orm.LoadModel[Account]("账户", "account", orm.ModelConfig{
		Index:     AccountIndex{},
		Order:     "id desc",
		Database:  "default",
		Relations: []orm.Relation{accountRoleRelation},
		Fields: map[string]orm.FieldConfig{
			"password": {Type: orm.FieldTypePassword},
		},
	})
}
