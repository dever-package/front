package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadSession struct {
	ID               uint64    `dorm:"primaryKey;autoIncrement;comment:上传会话ID"`
	RuleID           uint64    `dorm:"type:bigint;not null;comment:上传规则"`
	StorageID        uint64    `dorm:"type:bigint;not null;default:0;comment:存储方式"`
	Kind             string    `dorm:"type:varchar(32);comment:资源类型"`
	BizID            uint64    `dorm:"type:bigint;not null;default:0;comment:业务来源"`
	CategoryID       uint64    `dorm:"type:bigint;not null;default:0;comment:分类"`
	Name             string    `dorm:"type:varchar(255);comment:文件名称"`
	Ext              string    `dorm:"type:varchar(32);comment:文件后缀"`
	Mime             string    `dorm:"type:varchar(128);comment:MIME类型"`
	Size             int64     `dorm:"type:bigint;not null;default:0;comment:文件大小"`
	Hash             string    `dorm:"type:varchar(64);comment:文件哈希"`
	ObjectKey        string    `dorm:"type:varchar(255);comment:对象键"`
	ChunkSize        int64     `dorm:"type:bigint;not null;default:0;comment:分片大小"`
	ChunkTotal       int       `dorm:"type:int;not null;default:1;comment:分片总数"`
	UploadedParts    string    `dorm:"type:text;comment:已上传分片"`
	ProviderUploadID string    `dorm:"type:varchar(255);comment:存储侧上传ID"`
	Status           string    `dorm:"type:varchar(32);comment:状态"`
	ExpiredAt        time.Time `dorm:"not null;comment:过期时间"`
	CreatedAt        time.Time `dorm:"not null;comment:创建时间"`
}

type UploadSessionIndex struct {
	RuleStatus struct{} `index:"rule_id,status"`
}

func NewUploadSessionModel() *orm.Model[UploadSession] {
	return orm.LoadModel[UploadSession]("upload_session", UploadSession{}, UploadSessionIndex{}, "id desc", "default")
}
