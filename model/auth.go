package model

import (
	"time"

	"github.com/shemic/dever/orm"

	frontmeta "github.com/dever-package/front/service/meta"
)

type Auth struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:权限ID"`
	Key       string    `dorm:"type:varchar(128);not null;comment:标识"`
	Name      string    `dorm:"type:varchar(128);not null;comment:权限名称"`
	Icon      string    `dorm:"type:varchar(64);not null;comment:图标"`
	Path      string    `dorm:"type:varchar(255);not null;comment:路径"`
	Query     string    `dorm:"type:text;not null;comment:匹配参数"`
	ParentID  uint64    `dorm:"not null;default:0;comment:父级权限"`
	Type      int       `dorm:"type:smallint;not null;default:1;comment:类型"`
	Sort      int       `dorm:"type:int;not null;default:0;comment:排序"`
	CreatedAt time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
}

type AuthIndex struct {
	Key struct{} `unique:"key"`
}

var authParentRelation = frontmeta.Relation{
	Field:      "parent_id",
	Option:     "front.NewAuthModel",
	OptionKeys: []string{},
}

func init() {
	frontmeta.RegisterModelMeta("front.NewAuthModel", frontmeta.ModelMeta{
		Relations: []frontmeta.Relation{authParentRelation},
	})
}

func NewAuthModel() *orm.Model[Auth] {
	return orm.LoadModel[Auth]("auth", Auth{}, AuthIndex{}, "sort asc, id asc", "default")
}
