package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type Role struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:角色ID"`
	Name      string    `dorm:"type:varchar(64);not null;comment:角色名称"`
	CreatedAt time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

var roleAuthRelation = orm.Relation{
	Field:      "auth_ids",
	Through:    "front.NewRoleAuthModel",
	Option:     "front.NewAuthModel",
	OptionKeys: []string{},
}

func NewRoleModel() *orm.Model[Role] {
	return orm.LoadModel[Role]("角色", "role", orm.ModelConfig{
		Seeds: []map[string]any{
			{"id": 1, "name": "超级管理员"},
		},
		Order:     "id asc",
		Database:  "default",
		Relations: []orm.Relation{roleAuthRelation},
	})
}
