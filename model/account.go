package model

import (
	"time"

	"github.com/shemic/dever/orm"

	frontmeta "github.com/dever-package/front/service/meta"
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

var accountRoleRelation = frontmeta.Relation{
	Field:        "role_ids",
	Through:      "front.NewAccountRoleModel",
	Option:       "front.NewRoleModel",
	ThroughOrder: "id asc",
	OptionKeys:   []string{"role"},
}

func init() {
	frontmeta.RegisterModelMeta("front.NewAccountModel", frontmeta.ModelMeta{
		Relations:      []frontmeta.Relation{accountRoleRelation},
		HiddenFields:   []string{"password"},
		PasswordFields: []string{"password"},
	})
}

func NewAccountModel() *orm.Model[Account] {
	return orm.LoadModel[Account]("account", Account{}, AccountIndex{}, "id desc", "default")
}
