package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadStorage struct {
	ID         uint64    `dorm:"primaryKey;autoIncrement;comment:存储方式ID"`
	Name       string    `dorm:"type:varchar(64);comment:存储方式"`
	Type       string    `dorm:"type:varchar(32);comment:类型"`
	AccessKey  string    `dorm:"type:varchar(255);comment:AccessKey"`
	SecretKey  string    `dorm:"type:varchar(255);comment:SecretKey"`
	Bucket     string    `dorm:"type:varchar(255);comment:Bucket"`
	Domain     string    `dorm:"type:varchar(255);comment:访问域名"`
	UploadHost string    `dorm:"type:varchar(255);comment:上传域名"`
	TokenTTL   int64     `dorm:"type:bigint;not null;default:3600;comment:凭证有效期(秒)"`
	CreatedAt  time.Time `dorm:"comment:创建时间"`
}

var (
	uploadStorageSeed = []map[string]any{
		{
			"id":          1,
			"name":        "本地存储",
			"type":        "local",
			"access_key":  "",
			"secret_key":  "",
			"bucket":      "",
			"domain":      "",
			"upload_host": "",
			"token_ttl":   3600,
		},
		{
			"id":          2,
			"name":        "七牛云",
			"type":        "qiniu",
			"access_key":  "",
			"secret_key":  "",
			"bucket":      "",
			"domain":      "",
			"upload_host": "",
			"token_ttl":   3600,
		},
	}

	uploadStorageTypeOptions = []map[string]any{
		{"id": "local", "value": "本地"},
		{"id": "qiniu", "value": "七牛云"},
	}
)

func NewUploadStorageModel() *orm.Model[UploadStorage] {
	return orm.LoadModel[UploadStorage]("存储方式", "upload_storage", orm.ModelConfig{
		Seeds:    uploadStorageSeed,
		Order:    "id asc",
		Database: "default",
		Options: map[string]any{
			"type": uploadStorageTypeOptions,
		},
	})
}
