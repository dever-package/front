package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadFile struct {
	ID         uint64    `dorm:"primaryKey;autoIncrement;comment:文件ID"`
	RuleID     uint64    `dorm:"type:bigint;not null;comment:上传规则"`
	StorageID  uint64    `dorm:"type:bigint;not null;default:0;comment:存储方式"`
	Kind       string    `dorm:"type:varchar(32);comment:资源类型"`
	BizID      uint64    `dorm:"type:bigint;not null;default:0;comment:业务来源"`
	CategoryID uint64    `dorm:"type:bigint;not null;default:0;comment:分类"`
	Name       string    `dorm:"type:varchar(255);comment:文件名称"`
	Ext        string    `dorm:"type:varchar(32);comment:文件后缀"`
	Mime       string    `dorm:"type:varchar(128);comment:MIME类型"`
	Size       int64     `dorm:"type:bigint;not null;default:0;comment:文件大小"`
	Hash       string    `dorm:"type:varchar(64);comment:文件哈希"`
	Path       string    `dorm:"type:varchar(255);comment:存储路径"`
	CreatedAt  time.Time `dorm:"comment:创建时间"`
}

type UploadFileIndex struct {
	Path       struct{} `unique:"path"`
	Hash       struct{} `index:"rule_id,hash"`
	BizID      struct{} `index:"biz_id,created_at"`
	Kind       struct{} `index:"kind,created_at"`
	CategoryID struct{} `index:"category_id,created_at"`
}

func NewUploadFileModel() *orm.Model[UploadFile] {
	return orm.LoadModel[UploadFile]("upload_file", UploadFile{}, UploadFileIndex{}, "id desc", "default")
}
