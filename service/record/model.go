package record

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/orm"
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
	value         any
	config        reflect.Value
	uniqueIndexes reflect.Value
	selectMap     reflect.Value
	findMap       reflect.Value
	count         reflect.Value
	insert        reflect.Value
	update        reflect.Value
	delete        reflect.Value
}

type ConfigProvider interface {
	Config() orm.ModelConfig
}

var (
	loadCache    util.ConcurrentMap[string, any]
	modelCache   util.ConcurrentMap[string, Model]
	adapterCache util.ConcurrentMap[string, *Adapter]
)

func Resolve(modelName string) Model {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	if cached, ok := modelCache.Load(modelName); ok {
		return cached
	}

	modelValue := LoadSafe(modelName)
	if model, ok := modelValue.(Model); ok {
		modelCache.Store(modelName, model)
		return model
	}

	adapter := resolveAdapterValue(modelName, modelValue)
	if adapter != nil {
		modelCache.Store(modelName, adapter)
	}
	return adapter
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
	return resolveAdapterValue(modelName, modelValue)
}

func resolveAdapterValue(modelName string, modelValue any) *Adapter {
	if modelValue == nil {
		return nil
	}
	if cached, ok := adapterCache.Load(modelName); ok {
		return cached
	}
	adapter := newAdapter(modelValue)
	adapterCache.Store(modelName, adapter)
	return adapter
}

func Wrap(value any) *Adapter {
	if value == nil {
		return nil
	}
	return newAdapter(value)
}

func LoadSafe(modelName string) (result any) {
	result, _ = LoadSafeWithError(modelName)
	return result
}

func LoadSafeWithError(modelName string) (result any, err error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, nil
	}
	if cached, ok := loadCache.Load(modelName); ok {
		return cached, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("%v", recovered)
		}
	}()
	result = load.Model(modelName)
	if result != nil {
		loadCache.Store(modelName, result)
	}
	return result, nil
}

func ResolveConfig(modelName string, modelValue any) orm.ModelConfig {
	if provider, ok := modelValue.(ConfigProvider); ok {
		return provider.Config()
	}
	if adapter := resolveAdapterValue(modelName, modelValue); adapter != nil {
		return adapter.Config()
	}
	return orm.ModelConfig{}
}

func (m *Adapter) Config() orm.ModelConfig {
	if m == nil || m.value == nil {
		return orm.ModelConfig{}
	}
	method := m.config
	if !method.IsValid() || method.Type().NumIn() != 0 {
		return orm.ModelConfig{}
	}
	out := method.Call(nil)
	if len(out) == 0 || !out[0].IsValid() {
		return orm.ModelConfig{}
	}
	config, _ := out[0].Interface().(orm.ModelConfig)
	return config
}

func (m *Adapter) UniqueIndexes() [][]string {
	if m == nil || m.value == nil {
		return nil
	}
	method := m.uniqueIndexes
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
	method := m.method(name)
	return method.IsValid() && method.Type().NumIn() == argc
}

func (m *Adapter) SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any {
	method := m.selectMap
	if !hasArgs(method, 2) {
		return nil
	}
	args := []reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	}
	if len(options) > 0 {
		if method.Type().NumIn() < 3 {
			return nil
		}
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
	method := m.findMap
	if !hasArgs(method, 2) {
		return nil
	}
	args := []reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	}
	if len(options) > 0 {
		if method.Type().NumIn() < 3 {
			return nil
		}
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
	method := m.count
	if !hasArgs(method, 2) {
		return 0
	}
	args := []reflect.Value{
		reflectCallArg(method.Type().In(0), ctx),
		reflectCallArg(method.Type().In(1), filters),
	}
	if len(options) > 0 {
		if method.Type().NumIn() < 3 {
			return 0
		}
		args = append(args, reflectCallArg(method.Type().In(2), options[0]))
	}
	out := method.Call(args)
	if len(out) == 0 {
		return 0
	}
	return util.ToInt64(out[0].Interface())
}

func (m *Adapter) Insert(ctx context.Context, record map[string]any) int64 {
	method := m.insert
	if !hasArgs(method, 2) {
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
	method := m.update
	if !hasArgs(method, 3) {
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
	method := m.delete
	if !hasArgs(method, 2) {
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

func newAdapter(value any) *Adapter {
	rv := reflect.ValueOf(value)
	return &Adapter{
		value:         value,
		config:        rv.MethodByName("Config"),
		uniqueIndexes: rv.MethodByName("UniqueIndexes"),
		selectMap:     rv.MethodByName("SelectMap"),
		findMap:       rv.MethodByName("FindMap"),
		count:         rv.MethodByName("Count"),
		insert:        rv.MethodByName("Insert"),
		update:        rv.MethodByName("Update"),
		delete:        rv.MethodByName("Delete"),
	}
}

func (m *Adapter) method(name string) reflect.Value {
	if m == nil {
		return reflect.Value{}
	}
	switch strings.TrimSpace(name) {
	case "Config":
		return m.config
	case "UniqueIndexes":
		return m.uniqueIndexes
	case "SelectMap":
		return m.selectMap
	case "FindMap":
		return m.findMap
	case "Count":
		return m.count
	case "Insert":
		return m.insert
	case "Update":
		return m.update
	case "Delete":
		return m.delete
	default:
		return reflect.Value{}
	}
}

func hasArgs(method reflect.Value, min int) bool {
	return method.IsValid() && method.Type().NumIn() >= min
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
