package model

import (
	"strings"
	"time"

	"github.com/shemic/dever/orm"
)

type UploadStorage struct {
	ID         uint64    `dorm:"primaryKey;autoIncrement;comment:存储方式ID"`
	Name       string    `dorm:"type:varchar(64);comment:存储方式"`
	Type       string    `dorm:"type:varchar(32);comment:类型"`
	PathMode   string    `dorm:"type:varchar(16);not null;default:auto;comment:文件组织方式"`
	AccessKey  string    `dorm:"type:varchar(255);comment:AccessKey"`
	SecretKey  string    `dorm:"type:varchar(255);comment:SecretKey"`
	Bucket     string    `dorm:"type:varchar(255);comment:Bucket"`
	Domain     string    `dorm:"type:varchar(255);comment:访问域名"`
	UploadHost string    `dorm:"type:varchar(255);comment:上传域名"`
	TokenTTL   int64     `dorm:"type:bigint;not null;default:3600;comment:凭证有效期(秒)"`
	CreatedAt  time.Time `dorm:"comment:创建时间"`
}

const (
	UploadStoragePathModeAuto     = "auto"
	UploadStoragePathModeHash     = "hash"
	UploadStoragePathModeReadable = "readable"
)

func NormalizeUploadStoragePathMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case UploadStoragePathModeHash:
		return UploadStoragePathModeHash
	case UploadStoragePathModeReadable:
		return UploadStoragePathModeReadable
	default:
		return UploadStoragePathModeAuto
	}
}

var (
	uploadStorageSeed = []map[string]any{
		{
			"id":          1,
			"name":        "本地存储",
			"type":        "local",
			"path_mode":   UploadStoragePathModeAuto,
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
			"path_mode":   UploadStoragePathModeAuto,
			"access_key":  "",
			"secret_key":  "",
			"bucket":      "",
			"domain":      "",
			"upload_host": "",
			"token_ttl":   3600,
		},
		{
			"id":          3,
			"name":        "飞书云盘",
			"type":        "feishu",
			"path_mode":   UploadStoragePathModeAuto,
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
		{"id": "feishu", "value": "飞书云盘"},
	}

	uploadStoragePathModeOptions = []map[string]any{
		{"id": UploadStoragePathModeAuto, "value": "自动适配"},
		{"id": UploadStoragePathModeHash, "value": "内容哈希"},
		{"id": UploadStoragePathModeReadable, "value": "来源目录"},
	}
)

func NewUploadStorageModel() *orm.Model[UploadStorage] {
	return orm.LoadModel[UploadStorage]("存储方式", "upload_storage", orm.ModelConfig{
		Seeds:    uploadStorageSeed,
		Order:    "id asc",
		Database: "default",
		Options: map[string]any{
			"type":      uploadStorageTypeOptions,
			"path_mode": uploadStoragePathModeOptions,
		},
	})
}
