package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadAcceptType struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:允许类型ID"`
	Name      string    `dorm:"type:varchar(64);comment:允许类型名称"`
	Accept    string    `dorm:"type:varchar(255);comment:Accept"`
	CreatedAt time.Time `dorm:"comment:创建时间"`
}

type UploadAcceptTypeIndex struct {
	Name struct{} `index:"name,id"`
}

var uploadAcceptTypeSeed = []map[string]any{
	{"id": 1, "name": "图片", "accept": "image/*"},
	{"id": 2, "name": "视频", "accept": "video/*"},
	{"id": 3, "name": "音频", "accept": "audio/*"},
	{"id": 4, "name": "Office 文件", "accept": ".doc,.docx,.xls,.xlsx,.ppt,.pptx"},
	{"id": 5, "name": "PDF 文件", "accept": ".pdf"},
}

func NewUploadAcceptTypeModel() *orm.Model[UploadAcceptType] {
	return orm.LoadModel[UploadAcceptType]("允许类型", "upload_accept_type", orm.ModelConfig{
		Index:    UploadAcceptTypeIndex{},
		Seeds:    uploadAcceptTypeSeed,
		Order:    "id asc",
		Database: "default",
	})
}
