package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"
)

type CronParam struct {
	ID         uint64    `dorm:"primaryKey;autoIncrement;comment:参数ID"`
	CronID     uint64    `dorm:"type:bigint;not null;default:0;comment:计划任务"`
	ParamKey   string    `dorm:"type:varchar(128);not null;comment:参数名"`
	ParamValue string    `dorm:"type:text;not null;default:'';comment:参数值"`
	Status     int       `dorm:"type:int;not null;default:1;comment:状态"`
	Sort       int       `dorm:"type:int;not null;default:100;comment:排序"`
	CreatedAt  time.Time `dorm:"comment:创建时间"`
	UpdatedAt  time.Time `dorm:"comment:更新时间"`
}

type CronParamIndex struct {
	CronSort struct{} `index:"cron_id,status,sort,id"`
	CronKey  struct{} `unique:"cron_id,param_key"`
}

func NewCronParamModel() *orm.Model[CronParam] {
	return orm.LoadModel[CronParam]("计划任务参数", "cron_param", orm.ModelConfig{
		Index:    CronParamIndex{},
		Order:    "sort asc,id asc",
		Database: "default",
		Options: map[string]any{
			"status": cronStatusOptions,
		},
		Relations: []orm.Relation{
			cronRunCronRelation,
		},
		Labels: map[string]string{
			"cron_id":     "计划任务",
			"param_key":   "参数名",
			"param_value": "参数值",
			"status":      "状态",
			"sort":        "排序",
			"created_at":  "创建时间",
			"updated_at":  "更新时间",
		},
	})
}

func BuildCronPayloadJSON(ctx context.Context, cronID uint64) (string, error) {
	rows := NewCronParamModel().SelectMap(ctx, map[string]any{
		"cron_id": cronID,
		"status":  CronStatusEnabled,
	}, map[string]any{"order": "main.sort asc, main.id asc"})

	payload := make(map[string]any, len(rows))
	for _, row := range rows {
		paramKey := util.ToStringTrimmed(row["param_key"])
		if paramKey == "" {
			continue
		}
		value, err := ParseCronParamValue(util.ToString(row["param_value"]))
		if err != nil {
			return "", fmt.Errorf("参数 %s: %w", paramKey, err)
		}
		payload[paramKey] = value
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("执行参数生成失败: %w", err)
	}
	return string(data), nil
}

func ParseCronParamValue(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	switch strings.ToLower(raw) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, fmt.Errorf("JSON 格式无效: %w", err)
		}
		return value, nil
	}
	if number, ok := util.ParseInt64(raw); ok {
		return number, nil
	}
	if strings.Contains(raw, ".") {
		if number, err := strconv.ParseFloat(raw, 64); err == nil {
			return number, nil
		}
	}
	return raw, nil
}
