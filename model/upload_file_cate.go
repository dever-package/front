package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadFileCate struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:资源分类ID"`
	Name      string    `dorm:"type:varchar(64);comment:分类名称"`
	Status    int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort      int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt time.Time `dorm:"comment:创建时间"`
}

type UploadFileCateIndex struct {
	Name struct{} `unique:"name"`
	Sort struct{} `index:"sort,id"`
}

var uploadFileCateStatusOptions = []map[string]any{
	{"id": 1, "value": "启用", "label": "启用", "color": "#0f766e"},
	{"id": 0, "value": "停用", "label": "停用", "color": "#737373"},
}

func NewUploadFileCateModel() *orm.Model[UploadFileCate] {
	return orm.LoadModel[UploadFileCate]("资源分类", "upload_file_cate", orm.ModelConfig{
		Index:    UploadFileCateIndex{},
		Order:    "sort asc,id asc",
		Database: "default",
		Options: map[string]any{
			"status": uploadFileCateStatusOptions,
		},
	})
}
