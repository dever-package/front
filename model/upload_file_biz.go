package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type UploadFileBiz struct {
	ID        uint64    `dorm:"primaryKey;autoIncrement;comment:业务来源ID"`
	Key       string    `dorm:"type:varchar(128);comment:业务标识"`
	Name      string    `dorm:"type:varchar(128);comment:业务名称"`
	CreatedAt time.Time `dorm:"comment:创建时间"`
}

type UploadFileBizIndex struct {
	Key  struct{} `unique:"key"`
	Name struct{} `index:"name,id"`
}

func NewUploadFileBizModel() *orm.Model[UploadFileBiz] {
	return orm.LoadModel[UploadFileBiz]("业务来源", "upload_file_biz", orm.ModelConfig{
		Index:    UploadFileBizIndex{},
		Order:    "id desc",
		Database: "default",
	})
}
