package model

const (
	DataStatusDisabled = 0
	DataStatusEnabled  = 1
)

var dataStatusOptions = []map[string]any{
	{"id": DataStatusEnabled, "value": "启用", "label": "启用", "color": "#0f766e"},
	{"id": DataStatusDisabled, "value": "停用", "label": "停用", "color": "#737373"},
}

var dataFieldTypeOptions = []map[string]any{
	{"id": "text", "value": "单行文本", "icon": "type"},
	{"id": "textarea", "value": "多行文本", "icon": "align-left"},
	{"id": "editor", "value": "富文本编辑器", "icon": "file-text"},
	{"id": "date", "value": "日期", "icon": "calendar"},
	{"id": "datetime", "value": "日期时间", "icon": "calendar-clock"},
	{"id": "boolean", "value": "开关", "icon": "toggle-left"},
	{"id": "radio", "value": "单选", "icon": "circle-dot"},
	{"id": "checkbox", "value": "多选", "icon": "list-checks"},
	{"id": "select", "value": "下拉", "icon": "list-filter"},
	{"id": "multi_select", "value": "多选下拉", "icon": "list-checks"},
	{"id": "image", "value": "上传图片", "icon": "image"},
	{"id": "video", "value": "上传视频", "icon": "video"},
	{"id": "audio", "value": "上传音频", "icon": "audio-lines"},
}
