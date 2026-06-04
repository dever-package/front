package cron

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shemic/dever/util"

	frontmodel "my/package/front/model"
)

type cronSchedule struct {
	Mode   string
	Config string
	Spec   string
}

func buildCronSchedule(record map[string]any) (cronSchedule, error) {
	mode := util.ToStringTrimmed(record["schedule_mode"])
	if mode == "" && util.ToStringTrimmed(record["spec"]) != "" {
		mode = frontmodel.CronScheduleCron
	}
	mode = frontmodel.NormalizeCronScheduleMode(mode)

	config := map[string]any{}
	spec := ""
	switch mode {
	case frontmodel.CronScheduleEverySeconds:
		seconds := clampInt(record["interval_seconds"], 5, 1, 59)
		config["interval_seconds"] = seconds
		spec = fmt.Sprintf("*/%d * * * * *", seconds)
	case frontmodel.CronScheduleEveryMinutes:
		minutes := clampInt(record["interval_minutes"], 5, 1, 59)
		config["interval_minutes"] = minutes
		spec = fmt.Sprintf("0 */%d * * * *", minutes)
	case frontmodel.CronScheduleHourly:
		minute := clampInt(record["hourly_minute"], 0, 0, 59)
		config["minute"] = minute
		spec = fmt.Sprintf("0 %d * * * *", minute)
	case frontmodel.CronScheduleDaily:
		hour, minute, err := parseClock(util.ToStringTrimmed(record["daily_time"]), "02:00")
		if err != nil {
			return cronSchedule{}, fmt.Errorf("每日运行时间无效: %w", err)
		}
		config["time"] = formatClock(hour, minute)
		spec = fmt.Sprintf("0 %d %d * * *", minute, hour)
	case frontmodel.CronScheduleWeekly:
		weekday := clampInt(record["weekly_weekday"], 1, 0, 6)
		hour, minute, err := parseClock(util.ToStringTrimmed(record["weekly_time"]), "02:00")
		if err != nil {
			return cronSchedule{}, fmt.Errorf("每周运行时间无效: %w", err)
		}
		config["weekday"] = weekday
		config["time"] = formatClock(hour, minute)
		spec = fmt.Sprintf("0 %d %d * * %d", minute, hour, weekday)
	case frontmodel.CronScheduleMonthly:
		day := clampInt(record["monthly_day"], 1, 1, 28)
		hour, minute, err := parseClock(util.ToStringTrimmed(record["monthly_time"]), "02:00")
		if err != nil {
			return cronSchedule{}, fmt.Errorf("每月运行时间无效: %w", err)
		}
		config["day"] = day
		config["time"] = formatClock(hour, minute)
		spec = fmt.Sprintf("0 %d %d %d * *", minute, hour, day)
	case frontmodel.CronScheduleCron:
		spec = util.ToStringTrimmed(record["spec"])
		config["spec"] = spec
	default:
		return cronSchedule{}, fmt.Errorf("触发规则无效")
	}

	data, err := json.Marshal(config)
	if err != nil {
		return cronSchedule{}, fmt.Errorf("触发配置生成失败: %w", err)
	}
	return cronSchedule{Mode: mode, Config: string(data), Spec: spec}, nil
}

func clampInt(value any, fallback int, min int, max int) int {
	result := util.ToIntDefault(value, fallback)
	if result < min {
		return min
	}
	if result > max {
		return max
	}
	return result
}

func parseClock(value string, fallback string) (int, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("格式应为 HH:mm")
	}
	hour, ok := util.ParseInt64(strings.TrimSpace(parts[0]))
	if !ok || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("小时必须在 0-23 之间")
	}
	minute, ok := util.ParseInt64(strings.TrimSpace(parts[1]))
	if !ok || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("分钟必须在 0-59 之间")
	}
	return int(hour), int(minute), nil
}

func formatClock(hour int, minute int) string {
	return fmt.Sprintf("%02d:%02d", hour, minute)
}
