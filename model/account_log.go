package model

import (
	"time"

	"github.com/shemic/dever/orm"
)

type AccountLog struct {
	ID          uint64    `dorm:"primaryKey;autoIncrement;comment:日志ID"`
	AccountID   uint64    `dorm:"type:bigint;not null;default:0;comment:管理员ID"`
	AccountName string    `dorm:"type:varchar(64);not null;comment:管理员姓名"`
	Account     string    `dorm:"type:varchar(64);not null;comment:管理员账户"`
	Action      string    `dorm:"type:varchar(32);not null;comment:操作类型"`
	Method      string    `dorm:"type:varchar(16);not null;comment:请求方法"`
	Path        string    `dorm:"type:varchar(255);not null;comment:请求路径"`
	PagePath    string    `dorm:"type:varchar(255);not null;comment:页面路径"`
	TargetModel string    `dorm:"type:varchar(128);not null;comment:目标模型"`
	TargetID    string    `dorm:"type:varchar(128);not null;comment:目标ID"`
	Message     string    `dorm:"type:varchar(255);not null;comment:操作说明"`
	IP          string    `dorm:"type:varchar(64);not null;comment:IP地址"`
	UserAgent   string    `dorm:"type:varchar(255);not null;comment:浏览器"`
	Payload     string    `dorm:"type:text;not null;comment:操作参数"`
	CreatedAt   time.Time `dorm:"not null;default:CURRENT_TIMESTAMP;comment:操作时间"`
}

type AccountLogIndex struct {
	AccountCreatedAt struct{} `index:"account_id,created_at"`
	ActionCreatedAt  struct{} `index:"action,created_at"`
	PageCreatedAt    struct{} `index:"page_path,created_at"`
}

var accountLogActionOptions = []map[string]any{
	{"id": "login", "value": "登录", "label": "登录", "color": "#2563eb"},
	{"id": "create", "value": "新增", "label": "新增", "color": "#0f766e"},
	{"id": "update", "value": "编辑", "label": "编辑", "color": "#7c3aed"},
	{"id": "delete", "value": "删除", "label": "删除", "color": "#dc2626"},
	{"id": "sync", "value": "同步", "label": "同步", "color": "#d97706"},
	{"id": "export", "value": "导出", "label": "导出", "color": "#0891b2"},
	{"id": "import", "value": "导入", "label": "导入", "color": "#4f46e5"},
	{"id": "upload", "value": "上传", "label": "上传", "color": "#65a30d"},
	{"id": "request", "value": "请求", "label": "请求", "color": "#525252"},
}

func NewAccountLogModel() *orm.Model[AccountLog] {
	return orm.LoadModel[AccountLog]("操作日志", "account_log", orm.ModelConfig{
		Index:    AccountLogIndex{},
		Order:    "created_at desc,id desc",
		Database: "default",
		Labels: map[string]string{
			"account_id":   "管理员",
			"account_name": "管理员姓名",
			"account":      "管理员账户",
			"action":       "操作类型",
			"method":       "请求方法",
			"path":         "请求路径",
			"page_path":    "页面路径",
			"target_model": "目标模型",
			"target_id":    "目标ID",
			"message":      "操作说明",
			"ip":           "IP地址",
			"user_agent":   "浏览器",
			"payload":      "操作参数",
			"created_at":   "操作时间",
		},
		Options: map[string]any{
			"action": accountLogActionOptions,
		},
	})
}
