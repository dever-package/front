package cron

import (
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontmodel "my/package/front/model"
	frontaction "my/package/front/service/action"
	"my/package/front/service/cronexpr"
)

type CronHook struct{}

func (CronHook) ProviderBeforeSaveCron(_ *server.Context, params []any) any {
	record := cloneCronRecord(params)
	if len(record) == 0 {
		return record
	}

	now := time.Now()
	if isPartialCronRecord(record) {
		if _, exists := record["status"]; exists {
			record["status"] = frontmodel.RequireCronEnabledStatus(record["status"])
		}
		record["updated_at"] = now
		return record
	}

	name := util.ToStringTrimmed(record["name"])
	if name == "" {
		panic(frontaction.NewFieldError("form.name", "任务名称不能为空。"))
	}

	schedule, err := buildCronSchedule(record)
	if err != nil {
		panic(frontaction.NewFieldError("form.schedule_mode", err.Error()))
	}

	timezone, _, err := cronexpr.NormalizeTimezone(util.ToStringTrimmed(record["timezone"]))
	if err != nil {
		panic(frontaction.NewFieldError("form.timezone", err.Error()))
	}
	if err := cronexpr.Validate(schedule.Spec, timezone); err != nil {
		panic(frontaction.NewFieldError("form.spec", err.Error()))
	}

	use := util.ToStringTrimmed(record["use"])
	if use == "" {
		panic(frontaction.NewFieldError("form.use", "内部业务不能为空。"))
	}

	if _, exists := record["params"]; exists {
		record["params"] = normalizeCronParams(record["params"], now)
	}

	record["name"] = name
	if _, exists := record["status"]; exists || util.ToUint64(record["id"]) == 0 {
		record["status"] = frontmodel.RequireCronEnabledStatus(record["status"])
	}
	record["spec"] = schedule.Spec
	record["schedule_mode"] = schedule.Mode
	record["schedule_config"] = schedule.Config
	record["timezone"] = timezone
	record["kind"] = frontmodel.NormalizeCronKind(util.ToStringTrimmed(record["kind"]))
	record["use"] = use
	record["payload_json"] = "{}"
	record["timeout_seconds"] = frontmodel.NormalizeCronTimeoutSeconds(record["timeout_seconds"])
	record["updated_at"] = now
	if util.ToUint64(record["id"]) == 0 {
		record["created_at"] = now
	}
	return record
}

func cloneCronRecord(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	record, _ := params[0].(map[string]any)
	if record == nil {
		return map[string]any{}
	}
	return util.CloneMap(record)
}

func isPartialCronRecord(record map[string]any) bool {
	if util.ToUint64(record["id"]) == 0 {
		return false
	}
	for _, field := range []string{"name", "schedule_mode", "spec", "use", "params"} {
		if _, exists := record[field]; exists {
			return false
		}
	}
	return true
}

func normalizeCronParams(value any, now time.Time) []any {
	items, _ := value.([]any)
	if typed, ok := value.([]map[string]any); ok {
		items = make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
	}

	result := make([]any, 0, len(items))
	keys := map[string]struct{}{}
	for index, item := range items {
		row, _ := item.(map[string]any)
		if row == nil {
			continue
		}
		record := util.CloneMap(row)
		paramKey := util.ToStringTrimmed(record["param_key"])
		paramValue := util.ToStringTrimmed(record["param_value"])
		status := frontmodel.CronStatusEnabled
		if _, exists := record["status"]; exists {
			status = frontmodel.RequireCronEnabledStatus(record["status"])
		}

		if paramKey == "" && paramValue == "" && util.ToUint64(record["id"]) == 0 {
			continue
		}
		if paramKey == "" {
			panic(frontaction.NewFieldError("form.params", "参数名不能为空。"))
		}
		if _, exists := keys[paramKey]; exists {
			panic(frontaction.NewFieldError("form.params", "参数名不能重复。"))
		}
		keys[paramKey] = struct{}{}

		if status == frontmodel.CronStatusEnabled {
			if _, err := frontmodel.ParseCronParamValue(paramValue); err != nil {
				panic(frontaction.NewFieldError("form.params", paramKey+": "+err.Error()))
			}
		}

		record["param_key"] = paramKey
		record["param_value"] = paramValue
		record["status"] = status
		record["sort"] = util.ToIntDefault(record["sort"], (index+1)*10)
		record["updated_at"] = now
		if util.ToUint64(record["id"]) == 0 {
			record["created_at"] = now
		}
		result = append(result, record)
	}
	return result
}
