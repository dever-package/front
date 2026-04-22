package record

import (
	"context"
	"reflect"
	"strings"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/util"
)

type Model interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
	FindMap(ctx context.Context, filters any, options ...map[string]any) map[string]any
	Count(ctx context.Context, filters any, options ...map[string]any) int64
	Insert(ctx context.Context, record map[string]any) int64
	Update(ctx context.Context, filters any, record map[string]any) int64
	Delete(ctx context.Context, filters any) int64
}

type Adapter struct {
	value any
}

var adapterCache util.ConcurrentMap[string, *Adapter]

func Resolve(modelName string) Model {
	if modelValue, ok := LoadSafe(modelName).(Model); ok {
		return modelValue
	}
	return ResolveAdapter(modelName)
}

func ResolveAdapter(modelName string) *Adapter {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	if cached, ok := adapterCache.Load(modelName); ok {
		return cached
	}

	modelValue := LoadSafe(modelName)
	if modelValue == nil {
		return nil
	}
	adapter := &Adapter{value: modelValue}
	adapterCache.Store(modelName, adapter)
	return adapter
}

func LoadSafe(modelName string) (result any) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	return load.Model(modelName)
}

func (m *Adapter) Labels() map[string]string {
	if m == nil || m.value == nil {
		return nil
	}
	method := reflect.ValueOf(m.value).MethodByName("Labels")
	if !method.IsValid() || method.Type().NumIn() != 0 {
		return nil
	}
	out := method.Call(nil)
	if len(out) == 0 || !out[0].IsValid() || out[0].IsNil() {
		return nil
	}
	labels, _ := out[0].Interface().(map[string]string)
	if len(labels) == 0 {
		return nil
	}
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[normalizeColumnKey(key)] = strings.TrimSpace(value)
	}
	return result
}

func (m *Adapter) Label(field string) string {
	field = strings.TrimSpace(field)
	if field == "" || m == nil || m.value == nil {
		return ""
	}
	method := reflect.ValueOf(m.value).MethodByName("Label")
	if !method.IsValid() || method.Type().NumIn() != 1 {
		return ""
	}
	out := method.Call([]reflect.Value{reflectCallArg(method.Type().In(0), field)})
	if len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(util.ToString(out[0].Interface()))
}

func (m *Adapter) UniqueIndexes() [][]string {
	if m == nil || m.value == nil {
		return nil
	}
	method := reflect.ValueOf(m.value).MethodByName("UniqueIndexes")
	if !method.IsValid() || method.Type().NumIn() != 0 {
		return nil
	}
	out := method.Call(nil)
	if len(out) == 0 || !out[0].IsValid() || out[0].IsNil() {
		return nil
	}
	indexes, _ := out[0].Interface().([][]string)
	if len(indexes) == 0 {
		return nil
	}
	result := make([][]string, 0, len(indexes))
	for _, group := range indexes {
		columns := make([]string, 0, len(group))
		for _, field := range group {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			columns = append(columns, field)
		}
		if len(columns) == 0 {
			continue
		}
		result = append(result, columns)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (m *Adapter) HasMethod(name string, argc int) bool {
	if m == nil || m.value == nil {
		return false
	}
	method := reflect.ValueOf(m.value).MethodByName(name)
	return method.IsValid() && method.Type().NumIn() == argc
}

func (m *Adapter) SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any {
	method := reflect.ValueOf(m.value).MethodByName("SelectMap")
	if !method.IsValid() {
		return nil
	}
	args := []reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	}
	if len(options) > 0 {
		args = append(args, reflectCallArg(method.Type().In(2), options[0]))
	}
	out := method.Call(args)
	if len(out) == 0 || out[0].IsNil() {
		return nil
	}
	rows, _ := out[0].Interface().([]map[string]any)
	return rows
}

func (m *Adapter) FindMap(ctx context.Context, filters any, options ...map[string]any) map[string]any {
	method := reflect.ValueOf(m.value).MethodByName("FindMap")
	if !method.IsValid() {
		return nil
	}
	args := []reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	}
	if len(options) > 0 {
		args = append(args, reflectCallArg(method.Type().In(2), options[0]))
	}
	out := method.Call(args)
	if len(out) == 0 || out[0].IsNil() {
		return nil
	}
	row, _ := out[0].Interface().(map[string]any)
	return row
}

func (m *Adapter) Count(ctx context.Context, filters any, options ...map[string]any) int64 {
	method := reflect.ValueOf(m.value).MethodByName("Count")
	if !method.IsValid() {
		return 0
	}
	args := []reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	}
	if len(options) > 0 {
		args = append(args, reflectCallArg(method.Type().In(2), options[0]))
	}
	out := method.Call(args)
	if len(out) == 0 {
		return 0
	}
	return util.ToInt64(out[0].Interface())
}

func (m *Adapter) Insert(ctx context.Context, record map[string]any) int64 {
	method := reflect.ValueOf(m.value).MethodByName("Insert")
	if !method.IsValid() {
		return 0
	}
	out := method.Call([]reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), record),
	})
	if len(out) == 0 {
		return 0
	}
	return util.ToInt64(out[0].Interface())
}

func (m *Adapter) Update(ctx context.Context, filters any, record map[string]any) int64 {
	method := reflect.ValueOf(m.value).MethodByName("Update")
	if !method.IsValid() {
		return 0
	}
	out := method.Call([]reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
		reflectCallArg(method.Type().In(2), record),
	})
	if len(out) == 0 {
		return 0
	}
	return util.ToInt64(out[0].Interface())
}

func (m *Adapter) Delete(ctx context.Context, filters any) int64 {
	method := reflect.ValueOf(m.value).MethodByName("Delete")
	if !method.IsValid() {
		return 0
	}
	out := method.Call([]reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	})
	if len(out) == 0 {
		return 0
	}
	return util.ToInt64(out[0].Interface())
}

func reflectCallArg(target reflect.Type, value any) reflect.Value {
	if value == nil {
		return reflect.Zero(target)
	}
	current := reflect.ValueOf(value)
	if !current.IsValid() {
		return reflect.Zero(target)
	}
	if current.Type().AssignableTo(target) {
		return current
	}
	if current.Type().ConvertibleTo(target) {
		return current.Convert(target)
	}
	if target.Kind() == reflect.Interface && current.Type().Implements(target) {
		return current
	}
	return current
}
