package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadRule struct {
	ID           uint64    `dorm:"primaryKey;autoIncrement;comment:规则ID"`
	Name         string    `dorm:"type:varchar(64);comment:规则名称"`
	StorageID    uint64    `dorm:"type:bigint;not null;default:1;comment:存储方式"`
	AcceptTypeID uint64    `dorm:"type:bigint;not null;default:1;comment:允许类型"`
	Transport    string    `dorm:"type:varchar(32);default:relay;comment:上传方式"`
	ChunkSize    int64     `dorm:"type:bigint;not null;default:2;comment:分片大小"`
	MaxSize      int64     `dorm:"type:bigint;not null;default:10;comment:单文件上限"`
	Status       int       `dorm:"type:int;not null;default:1;comment:状态"`
	CreatedAt    time.Time `dorm:"comment:创建时间"`
}

type UploadRuleIndex struct {
	Name struct{} `index:"name,id"`
}

var (
	uploadRuleSeed = []map[string]any{
		{
			"id":             1,
			"name":           "图片",
			"storage_id":     1,
			"accept_type_id": 1,
			"transport":      "relay",
			"chunk_size":     2,
			"max_size":       10,
			"status":         1,
		},
		{
			"id":             2,
			"name":           "视频",
			"storage_id":     1,
			"accept_type_id": 2,
			"transport":      "relay",
			"chunk_size":     4,
			"max_size":       200,
			"status":         1,
		},
		{
			"id":             3,
			"name":           "音频",
			"storage_id":     1,
			"accept_type_id": 3,
			"transport":      "relay",
			"chunk_size":     4,
			"max_size":       100,
			"status":         1,
		},
		{
			"id":             4,
			"name":           "Office 文件",
			"storage_id":     1,
			"accept_type_id": 4,
			"transport":      "relay",
			"chunk_size":     4,
			"max_size":       200,
			"status":         1,
		},
		{
			"id":             5,
			"name":           "PDF 文件",
			"storage_id":     1,
			"accept_type_id": 5,
			"transport":      "relay",
			"chunk_size":     4,
			"max_size":       200,
			"status":         1,
		},
		{
			"id":             6,
			"name":           "用户附件",
			"storage_id":     1,
			"accept_type_id": 4,
			"transport":      "relay",
			"chunk_size":     4,
			"max_size":       200,
			"status":         1,
		},
	}

	uploadRuleTransportOptions = []map[string]any{
		{"id": "relay", "value": "后端中转"},
		{"id": "direct", "value": "前端直传"},
	}

	uploadRuleStatusOptions = []map[string]any{
		{"id": 1, "value": "启用", "label": "启用", "color": "#0f766e"},
		{"id": 0, "value": "停用", "label": "停用", "color": "#737373"},
	}
)

var uploadRuleStorageRelation = orm.Relation{
	Field:  "storage_id",
	Option: "front.NewUploadStorageModel",
}

var uploadRuleAcceptTypeRelation = orm.Relation{
	Field:      "accept_type_id",
	Name:       "accept_type",
	Option:     "front.NewUploadAcceptTypeModel",
	OptionKeys: []string{},
}

var uploadRuleAcceptTypesRelation = orm.Relation{
	Field:        "accept_type_ids",
	Name:         "accept_types",
	Through:      "front.NewUploadRuleAcceptTypeModel",
	Option:       "front.NewUploadAcceptTypeModel",
	ThroughOrder: "id asc",
}

func NewUploadRuleModel() *orm.Model[UploadRule] {
	return orm.LoadModel[UploadRule]("上传规则", "upload_rule", orm.ModelConfig{
		Index:    UploadRuleIndex{},
		Seeds:    uploadRuleSeed,
		Order:    "id asc",
		Database: "default",
		Options: map[string]any{
			"transport": uploadRuleTransportOptions,
			"status":    uploadRuleStatusOptions,
		},
		Relations: []orm.Relation{
			uploadRuleStorageRelation,
			uploadRuleAcceptTypeRelation,
			uploadRuleAcceptTypesRelation,
		},
	})
}
