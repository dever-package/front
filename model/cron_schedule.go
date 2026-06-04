package model

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shemic/dever/util"
)

func (m *CronModel) SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any {
	rows := m.Model.SelectMap(ctx, filters, options...)
	for _, row := range rows {
		HydrateCronScheduleFields(row)
		HydrateCronDisplayFields(row)
	}
	return rows
}

func (m *CronModel) FindMap(ctx context.Context, filters any, options ...map[string]any) map[string]any {
	row := m.Model.FindMap(ctx, filters, options...)
	HydrateCronScheduleFields(row)
	HydrateCronDisplayFields(row)
	return row
}

func HydrateCronScheduleFields(row map[string]any) {
	if len(row) == 0 {
		return
	}

	rawMode := util.ToStringTrimmed(row["schedule_mode"])
	mode := NormalizeCronScheduleMode(rawMode)
	if rawMode == "" && util.ToStringTrimmed(row["spec"]) != "" {
		mode = CronScheduleCron
	}
	row["schedule_mode"] = mode

	config := map[string]any{}
	rawConfig := util.ToStringTrimmed(row["schedule_config"])
	if rawConfig != "" {
		_ = json.Unmarshal([]byte(rawConfig), &config)
	}

	row["interval_seconds"] = util.ToIntDefault(config["interval_seconds"], 5)
	row["interval_minutes"] = util.ToIntDefault(config["interval_minutes"], 5)
	row["hourly_minute"] = util.ToIntDefault(config["minute"], 0)
	row["daily_time"] = stringDefault(config["time"], "02:00")
	row["weekly_weekday"] = util.ToIntDefault(config["weekday"], 1)
	row["weekly_time"] = stringDefault(config["time"], "02:00")
	row["monthly_day"] = util.ToIntDefault(config["day"], 1)
	row["monthly_time"] = stringDefault(config["time"], "02:00")
}

func HydrateCronDisplayFields(row map[string]any) {
	if len(row) == 0 {
		return
	}
	row["schedule_label"] = CronScheduleLabel(row)
	row["use_label"] = CronProviderLabel(util.ToStringTrimmed(row["use"]))
}

func CronScheduleLabel(row map[string]any) string {
	switch NormalizeCronScheduleMode(util.ToStringTrimmed(row["schedule_mode"])) {
	case CronScheduleEverySeconds:
		return fmt.Sprintf("每隔%d秒", util.ToIntDefault(row["interval_seconds"], 5))
	case CronScheduleEveryMinutes:
		return fmt.Sprintf("每隔%d分钟", util.ToIntDefault(row["interval_minutes"], 5))
	case CronScheduleHourly:
		return fmt.Sprintf("每小时第%d分钟", util.ToIntDefault(row["hourly_minute"], 0))
	case CronScheduleDaily:
		return "每天 " + stringDefault(row["daily_time"], "02:00")
	case CronScheduleWeekly:
		return fmt.Sprintf("每周%s %s", weekdayLabel(util.ToIntDefault(row["weekly_weekday"], 1)), stringDefault(row["weekly_time"], "02:00"))
	case CronScheduleMonthly:
		return fmt.Sprintf("每月%d号 %s", util.ToIntDefault(row["monthly_day"], 1), stringDefault(row["monthly_time"], "02:00"))
	case CronScheduleCron:
		return util.ToStringTrimmed(row["spec"])
	default:
		return util.ToStringTrimmed(row["spec"])
	}
}

func weekdayLabel(weekday int) string {
	switch weekday {
	case 0:
		return "周日"
	case 1:
		return "周一"
	case 2:
		return "周二"
	case 3:
		return "周三"
	case 4:
		return "周四"
	case 5:
		return "周五"
	case 6:
		return "周六"
	default:
		return "周一"
	}
}

func stringDefault(value any, fallback string) string {
	text := util.ToStringTrimmed(value)
	if text == "" {
		return fallback
	}
	return text
}
