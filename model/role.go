package model

import (
	"time"

	"github.com/shemic/dever/orm"

	frontmeta "github.com/dever-package/front/service/meta"
)

type Role struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:角色ID"`
	Name      string    `dorm:"type:varchar(64);not null;comment:角色名称"`
	CreatedAt time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

type RoleIndex struct {
	Name struct{} `unique:"name"`
}

var roleAuthRelation = frontmeta.Relation{
	Field:      "auth_ids",
	Through:    "front.NewRoleAuthModel",
	Option:     "front.NewAuthModel",
	OptionKeys: []string{},
}

func init() {
	frontmeta.RegisterModelMeta("front.NewRoleModel", frontmeta.ModelMeta{
		Relations: []frontmeta.Relation{roleAuthRelation},
	})
}

func NewRoleModel() *orm.Model[Role] {
	return orm.LoadModel[Role](
		"role",
		Role{},
		RoleIndex{},
		[]map[string]any{
			{"id": 1, "name": "超级管理员"},
		},
		"id asc",
		"default",
	)
}
