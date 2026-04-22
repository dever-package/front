package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type RoleAuth struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:角色权限关联ID"`
	RoleID    uint64    `dorm:"not null;comment:角色ID"`
	AuthID    uint64    `dorm:"not null;comment:权限ID"`
	CreatedAt time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

type RoleAuthIndex struct {
	RoleAuth struct{} `unique:"role_id,auth_id"`
}

func NewRoleAuthModel() *orm.Model[RoleAuth] {
	return orm.LoadModel[RoleAuth]("role_auth", RoleAuth{}, RoleAuthIndex{}, "id desc", "default")
}
