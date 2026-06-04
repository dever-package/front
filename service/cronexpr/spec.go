package cronexpr

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron"
)

const DefaultTimezone = "Asia/Shanghai"

func NormalizeTimezone(value string) (string, *time.Location, error) {
	timezone := strings.TrimSpace(value)
	if timezone == "" {
		timezone = DefaultTimezone
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", nil, fmt.Errorf("时区无效: %s", timezone)
	}
	return timezone, loc, nil
}

func Next(spec string, timezone string, after time.Time) (time.Time, error) {
	schedule, err := parseSchedule(spec)
	if err != nil {
		return time.Time{}, err
	}
	_, loc, err := NormalizeTimezone(timezone)
	if err != nil {
		return time.Time{}, err
	}
	if after.IsZero() {
		after = time.Now()
	}
	next := schedule.Next(after.In(loc))
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("无法计算下一次运行时间")
	}
	return next, nil
}

func Validate(spec string, timezone string) error {
	if _, err := parseSchedule(spec); err != nil {
		return err
	}
	_, _, err := NormalizeTimezone(timezone)
	return err
}

func parseSchedule(spec string) (cron.Schedule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("cron 表达式不能为空")
	}

	var (
		schedule cron.Schedule
		err      error
	)
	if strings.HasPrefix(spec, "@") {
		schedule, err = cron.Parse(spec)
	} else {
		fields := strings.Fields(spec)
		switch len(fields) {
		case 5:
			schedule, err = cron.ParseStandard(spec)
		case 6:
			schedule, err = cron.Parse(spec)
		default:
			return nil, fmt.Errorf("cron 表达式应为 5 段或 6 段，当前为 %d 段", len(fields))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cron 表达式无效: %w", err)
	}
	return schedule, nil
}
