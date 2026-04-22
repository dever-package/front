package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type AccountRole struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:账户角色关联ID"`
	AccountID uint64    `dorm:"not null;comment:账户ID"`
	RoleID    uint64    `dorm:"not null;comment:角色ID"`
	CreatedAt time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

type AccountRoleIndex struct {
	AccountRole struct{} `unique:"account_id,role_id"`
}

func NewAccountRoleModel() *orm.Model[AccountRole] {
	return orm.LoadModel[AccountRole]("account_role", AccountRole{}, AccountRoleIndex{}, "id asc", "default")
}
