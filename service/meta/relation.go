package meta

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/util"

	frontoption "github.com/dever-package/front/service/option"
	frontrecord "github.com/dever-package/front/service/record"
)

type OptionModel interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
}

type RelationModel interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
	Delete(ctx context.Context, filters any) int64
	Insert(ctx context.Context, record map[string]any) int64
}

type Relation struct {
	Kind             string
	Name             string
	Field            string
	Through          string
	Option           string
	Mode             string
	OptionKeys       []string
	RowKey           string
	OwnerField       string
	TargetField      string
	OptionValueField string
	OptionLabelField string
	EmptyValue       any
	Order            string
	ThroughOrder     string
	OptionOrder      string
}

func BuildRelationOptions(ctx context.Context, config Relation) map[string]any {
	if strings.ToLower(strings.TrimSpace(config.Kind)) != "option" {
		return nil
	}

	modelValue := resolveRelationOptionModel(config)
	if modelValue == nil {
		return nil
	}

	rows := modelValue.SelectMap(ctx, nil, mergeRelationQueryOption(nil, resolveRelationOptionOrder(config)))

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := util.CloneMap(row)
		item["id"] = row[config.OptionValueField]
		item["value"] = row[config.OptionLabelField]
		items = append(items, item)
	}

	if len(config.OptionKeys) == 0 {
		return nil
	}

	result := make(map[string]any, len(config.OptionKeys))
	for _, key := range config.OptionKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		result[key] = util.CloneMapSlice(items)
	}

	return result
}

func AttachRelation(ctx context.Context, rows []map[string]any, config Relation) []map[string]any {
	if strings.ToLower(strings.TrimSpace(config.Kind)) == "children" {
		return attachChildrenRelation(ctx, rows, config)
	}

	if len(rows) == 0 {
		return rows
	}

	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		id := util.ToUint64(row[config.RowKey])
		if id == 0 {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return rows
	}

	targetIDsByOwner := map[uint64][]uint64{}
	targetIDSet := map[uint64]struct{}{}
	targetIDs := make([]any, 0)
	if strings.TrimSpace(config.Through) == "" {
		for _, row := range rows {
			ownerID := util.ToUint64(row[config.RowKey])
			if ownerID == 0 {
				continue
			}
			for _, targetID := range collectRelationIDs(row[config.Field], config.Mode) {
				targetIDsByOwner[ownerID] = append(targetIDsByOwner[ownerID], targetID)
				if _, exists := targetIDSet[targetID]; exists {
					continue
				}
				targetIDSet[targetID] = struct{}{}
				targetIDs = append(targetIDs, targetID)
			}
		}
	} else {
		relationValue := resolveRelationModel(config)
		if relationValue == nil {
			return rows
		}

		relations := relationValue.SelectMap(
			ctx,
			map[string]any{config.OwnerField: ids},
			mergeRelationQueryOption(
				map[string]any{
					"field": "main." + config.OwnerField + ", main." + config.TargetField,
				},
				resolveRelationThroughOrder(config),
			),
		)

		for _, relation := range relations {
			ownerID := util.ToUint64(relation[config.OwnerField])
			targetID := util.ToUint64(relation[config.TargetField])
			if ownerID == 0 || targetID == 0 {
				continue
			}
			targetIDsByOwner[ownerID] = append(targetIDsByOwner[ownerID], targetID)
			if _, exists := targetIDSet[targetID]; exists {
				continue
			}
			targetIDSet[targetID] = struct{}{}
			targetIDs = append(targetIDs, targetID)
		}
	}

	for _, row := range rows {
		ownerID := util.ToUint64(row[config.RowKey])
		if ownerID == 0 || len(targetIDsByOwner[ownerID]) > 0 {
			continue
		}
		fallbackIDs := collectFallbackRelationIDs(row, config)
		if len(fallbackIDs) == 0 {
			continue
		}
		targetIDsByOwner[ownerID] = append(targetIDsByOwner[ownerID], fallbackIDs...)
		for _, targetID := range fallbackIDs {
			if _, exists := targetIDSet[targetID]; exists {
				continue
			}
			targetIDSet[targetID] = struct{}{}
			targetIDs = append(targetIDs, targetID)
		}
	}

	targetByID := loadRelationTargets(ctx, config, targetIDs)
	for _, row := range rows {
		ownerID := util.ToUint64(row[config.RowKey])
		if ownerID == 0 {
			continue
		}

		ownerTargetIDs := util.UniqueUint64s(targetIDsByOwner[ownerID])
		if config.Mode == "multiple" {
			idList := make([]any, 0, len(ownerTargetIDs))
			targetList := make([]map[string]any, 0, len(ownerTargetIDs))
			for _, targetID := range ownerTargetIDs {
				idList = append(idList, targetID)
				if target, ok := targetByID[targetID]; ok {
					targetList = append(targetList, util.CloneMap(target))
				}
			}
			row[config.Field] = idList
			row[config.Name] = targetList
			continue
		}

		if len(ownerTargetIDs) == 0 {
			row[config.Field] = config.EmptyValue
			row[config.Name] = nil
			continue
		}

		targetID := ownerTargetIDs[0]
		row[config.Field] = targetID
		if target, ok := targetByID[targetID]; ok {
			row[config.Name] = util.CloneMap(target)
		} else {
			row[config.Name] = nil
		}
	}

	return rows
}

func collectFallbackRelationIDs(row map[string]any, config Relation) []uint64 {
	if strings.TrimSpace(config.Mode) != "multiple" {
		return nil
	}

	field := strings.TrimSpace(config.Field)
	if !strings.HasSuffix(field, "_ids") {
		return nil
	}

	fallbackField := strings.TrimSuffix(field, "_ids") + "_id"
	value, exists := row[fallbackField]
	if !exists {
		return nil
	}
	return collectRelationIDs(value, "single")
}

func SaveRelation(ctx context.Context, ownerID any, record map[string]any, config Relation) error {
	if strings.ToLower(strings.TrimSpace(config.Kind)) == "children" {
		return saveChildrenRelation(ctx, ownerID, record, config)
	}
	if strings.TrimSpace(config.Through) == "" {
		return nil
	}

	targetValue, exists := record[config.Field]
	if !exists {
		return nil
	}

	owner := util.ToUint64(ownerID)
	if owner == 0 {
		return nil
	}

	relationValue := resolveRelationModel(config)
	if relationValue == nil {
		return nil
	}

	modelName := strings.TrimSpace(config.Through)
	modelValue := load.Model(modelName)
	columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)

	relationValue.Delete(ctx, map[string]any{config.OwnerField: owner})

	targetIDs := collectRelationIDs(targetValue, config.Mode)
	if len(targetIDs) == 0 {
		return nil
	}

	for index, targetID := range targetIDs {
		record := map[string]any{
			config.OwnerField:  owner,
			config.TargetField: targetID,
		}
		if sortColumn := frontrecord.ResolveColumnName(columnLookup, "sort"); sortColumn != "" {
			record[sortColumn] = index + 1
		}
		frontrecord.ApplyCreatedAt(record, columnLookup)
		if len(columnLookup) == 0 {
			record["created_at"] = time.Now()
		}
		relationValue.Insert(ctx, record)
	}

	return nil
}

func DeleteRelation(ctx context.Context, payload any, config Relation) error {
	if strings.ToLower(strings.TrimSpace(config.Kind)) != "children" && strings.TrimSpace(config.Through) == "" {
		return nil
	}

	ids := CollectIDs(payload, config.RowKey)
	if len(ids) == 0 {
		return nil
	}

	relationValue := resolveRelationModel(config)
	if relationValue == nil {
		return nil
	}

	relationValue.Delete(ctx, map[string]any{config.OwnerField: ids})
	return nil
}

func BuildRelationFilter(ctx context.Context, modelName, field string, value any) (map[string]any, bool) {
	field = strings.TrimSpace(field)
	if field == "" {
		return nil, false
	}

	var relationConfig Relation
	found := false
	for _, relation := range ResolveModelRelations(modelName) {
		if strings.TrimSpace(relation.Field) != field || strings.TrimSpace(relation.Through) == "" {
			continue
		}
		relationConfig = relation
		found = true
		break
	}
	if !found {
		return nil, false
	}

	targetIDs := collectRelationIDs(value, relationConfig.Mode)
	if len(targetIDs) == 0 {
		return nil, false
	}

	relationValue := resolveRelationModel(relationConfig)
	if relationValue == nil {
		return map[string]any{"main." + util.ToSnake(relationConfig.RowKey): 0}, true
	}

	targetFilters := make([]any, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		targetFilters = append(targetFilters, targetID)
	}

	relations := relationValue.SelectMap(
		ctx,
		map[string]any{relationConfig.TargetField: targetFilters},
		map[string]any{"field": "main." + relationConfig.OwnerField},
	)

	ownerIDs := make([]any, 0, len(relations))
	seen := map[uint64]struct{}{}
	for _, relation := range relations {
		ownerID := util.ToUint64(relation[relationConfig.OwnerField])
		if ownerID == 0 {
			continue
		}
		if _, exists := seen[ownerID]; exists {
			continue
		}
		seen[ownerID] = struct{}{}
		ownerIDs = append(ownerIDs, ownerID)
	}

	if len(ownerIDs) == 0 {
		return map[string]any{"main." + util.ToSnake(relationConfig.RowKey): 0}, true
	}

	return map[string]any{"main." + util.ToSnake(relationConfig.RowKey): ownerIDs}, true
}

func attachChildrenRelation(ctx context.Context, rows []map[string]any, config Relation) []map[string]any {
	if len(rows) == 0 {
		return rows
	}

	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		id := util.ToUint64(row[config.RowKey])
		if id == 0 {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return rows
	}

	relationValue := resolveRelationModel(config)
	if relationValue == nil {
		return rows
	}

	options := []map[string]any{}
	if order := resolveRelationThroughOrder(config); strings.TrimSpace(order) != "" {
		options = append(options, map[string]any{"order": order})
	}
	children := relationValue.SelectMap(ctx, map[string]any{config.OwnerField: ids}, options...)
	grouped := make(map[uint64][]map[string]any)
	for _, child := range children {
		ownerID := util.ToUint64(child[config.OwnerField])
		if ownerID == 0 {
			continue
		}
		grouped[ownerID] = append(grouped[ownerID], util.CloneMap(child))
	}

	for _, row := range rows {
		ownerID := util.ToUint64(row[config.RowKey])
		if ownerID == 0 {
			continue
		}
		items := util.CloneMapSlice(grouped[ownerID])
		row[config.Field] = items
		if config.Name != "" && config.Name != config.Field {
			row[config.Name] = util.CloneMapSlice(items)
		}
	}

	return rows
}

func saveChildrenRelation(ctx context.Context, ownerID any, record map[string]any, config Relation) error {
	childrenValue, exists := record[config.Field]
	if !exists {
		return nil
	}

	owner := util.ToUint64(ownerID)
	if owner == 0 {
		return nil
	}

	relationValue := resolveRelationModel(config)
	if relationValue == nil {
		return nil
	}

	relationValue.Delete(ctx, map[string]any{config.OwnerField: owner})

	items, ok := childrenValue.([]any)
	if !ok || len(items) == 0 {
		return nil
	}

	modelValue := load.Model(strings.TrimSpace(config.Through))
	columnLookup := frontrecord.ResolveColumnLookup(strings.TrimSpace(config.Through), modelValue)
	if len(columnLookup) == 0 {
		return nil
	}

	primaryColumn := frontrecord.ResolveColumnName(columnLookup, "id")
	ownerColumn := frontrecord.ResolveColumnName(columnLookup, config.OwnerField)
	createdAtColumn := frontrecord.ResolveColumnName(columnLookup, "created_at")
	for _, item := range items {
		child, ok := item.(map[string]any)
		if !ok {
			continue
		}

		data := frontrecord.SanitizeRecord(child, columnLookup)

		if primaryColumn != "" {
			delete(data, primaryColumn)
		}
		if ownerColumn != "" {
			delete(data, ownerColumn)
		}
		if createdAtColumn != "" {
			delete(data, createdAtColumn)
		}
		if isEmptyChildRecord(data) {
			continue
		}

		data[config.OwnerField] = owner
		frontrecord.ApplyCreatedAt(data, columnLookup)
		relationValue.Insert(ctx, data)
	}

	return nil
}

func CollectIDs(payload any, key string) []any {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "id"
	}

	switch current := payload.(type) {
	case map[string]any:
		id := util.ToUint64(current[key])
		if id == 0 {
			return nil
		}
		return []any{id}
	case []any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			switch typed := item.(type) {
			case map[string]any:
				id := util.ToUint64(typed[key])
				if id != 0 {
					result = append(result, id)
				}
			default:
				id := util.ToUint64(typed)
				if id != 0 {
					result = append(result, id)
				}
			}
		}
		return result
	default:
		id := util.ToUint64(current)
		if id == 0 {
			return nil
		}
		return []any{id}
	}
}

func loadRelationTargets(ctx context.Context, config Relation, targetIDs []any) map[uint64]map[string]any {
	if len(targetIDs) == 0 {
		return nil
	}

	modelValue := resolveRelationOptionModel(config)
	if modelValue == nil {
		return nil
	}

	rows := modelValue.SelectMap(ctx, map[string]any{config.OptionValueField: targetIDs})
	if len(rows) == 0 {
		rows = frontoption.SeedRowsByField(config.Option, config.OptionValueField, targetIDs)
		if len(rows) == 0 {
			return nil
		}
	}

	result := make(map[uint64]map[string]any, len(rows))
	for _, row := range rows {
		id := util.ToUint64(row[config.OptionValueField])
		if id == 0 {
			continue
		}
		result[id] = normalizeRelationTarget(ctx, config, row)
	}

	if len(result) < len(targetIDs) {
		seedRows := frontoption.SeedRowsByField(config.Option, config.OptionValueField, targetIDs)
		for _, row := range seedRows {
			id := util.ToUint64(row[config.OptionValueField])
			if id == 0 {
				continue
			}
			if _, exists := result[id]; exists {
				continue
			}
			result[id] = normalizeRelationTarget(ctx, config, row)
		}
	}

	return result
}

func normalizeRelationTarget(ctx context.Context, config Relation, row map[string]any) map[string]any {
	cloned := util.CloneMap(row)
	if strings.TrimSpace(config.Option) != "front.NewUploadFileModel" {
		return cloned
	}
	return buildUploadRelationPayload(ctx, cloned)
}

func buildUploadRelationPayload(ctx context.Context, row map[string]any) map[string]any {
	fileID := util.ToUint64(row["id"])
	openURL := ""
	if fileID != 0 {
		openURL = fmt.Sprintf("/front/upload/open?id=%d", fileID)
	}

	publicURL := resolveUploadRelationPublicURL(ctx, row)
	if strings.TrimSpace(publicURL) == "" {
		publicURL = openURL
	}

	result := util.CloneMap(row)
	result["url"] = publicURL
	result["thumbnail"] = publicURL
	result["open_url"] = openURL
	result["download"] = openURL
	return result
}

func resolveUploadRelationPublicURL(ctx context.Context, row map[string]any) string {
	pathValue := strings.TrimSpace(util.ToString(row["path"]))
	if pathValue == "" {
		return ""
	}

	storageID := util.ToUint64(row["storage_id"])
	if storageID == 0 {
		return resolveLocalUploadRelationURL("", pathValue)
	}

	storageModel := frontrecord.Resolve("front.NewUploadStorageModel")
	if storageModel == nil {
		return resolveLocalUploadRelationURL("", pathValue)
	}
	storageRow := storageModel.FindMap(ctx, map[string]any{"id": storageID})
	if len(storageRow) == 0 {
		return resolveLocalUploadRelationURL("", pathValue)
	}

	storageType := strings.ToLower(strings.TrimSpace(util.ToString(storageRow["type"])))
	storageDomain := strings.TrimSpace(util.ToString(storageRow["domain"]))

	switch storageType {
	case "qiniu":
		return joinRelationPublicURL(storageDomain, pathValue)
	case "local", "":
		return resolveLocalUploadRelationURL(storageDomain, pathValue)
	default:
		return joinRelationPublicURL(storageDomain, pathValue)
	}
}

func resolveLocalUploadRelationURL(domain, objectPath string) string {
	if url := joinRelationPublicURL(domain, objectPath); strings.TrimSpace(url) != "" {
		return url
	}
	normalizedPath := strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if normalizedPath == "" {
		return ""
	}
	return "/" + normalizedPath
}

func joinRelationPublicURL(domain, objectPath string) string {
	domain = strings.TrimRight(strings.TrimSpace(domain), "/")
	objectPath = strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if domain == "" || objectPath == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	return domain + "/" + objectPath
}

func relationOptionBaseKey(config Relation) string {
	field := strings.TrimSpace(config.Field)
	switch {
	case strings.HasSuffix(field, "_ids"):
		return strings.TrimSuffix(field, "_ids")
	case strings.HasSuffix(field, "_id"):
		return strings.TrimSuffix(field, "_id")
	}

	name := strings.TrimSpace(config.Name)
	if strings.HasSuffix(name, "s") {
		return strings.TrimSuffix(name, "s")
	}
	return name
}

func collectRelationIDs(value any, mode string) []uint64 {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "multiple":
		raw := CollectIDs(value, "id")
		result := make([]uint64, 0, len(raw))
		for _, item := range raw {
			id := util.ToUint64(item)
			if id != 0 {
				result = append(result, id)
			}
		}
		return util.UniqueUint64s(result)
	default:
		id := util.ToUint64(value)
		if id == 0 {
			return nil
		}
		return []uint64{id}
	}
}

func isEmptyChildRecord(data map[string]any) bool {
	for key, value := range data {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "id", "created_at", "updated_at":
			continue
		}
		if frontrecord.HasValue(value) {
			return false
		}
	}
	return true
}

func resolveRelationOptionModel(config Relation) OptionModel {
	modelName := strings.TrimSpace(config.Option)
	if modelName == "" {
		return nil
	}

	if modelValue, ok := frontrecord.LoadSafe(modelName).(OptionModel); ok {
		return modelValue
	}
	adapter := frontrecord.ResolveAdapter(modelName)
	if adapter == nil || !adapter.HasMethod("SelectMap", 3) {
		return nil
	}
	return adapter
}

func mergeRelationQueryOption(option map[string]any, order string) map[string]any {
	if strings.TrimSpace(order) == "" {
		return option
	}
	if option == nil {
		option = map[string]any{}
	}
	option["order"] = order
	return option
}

func resolveRelationThroughOrder(config Relation) string {
	if order := strings.TrimSpace(config.ThroughOrder); order != "" {
		return order
	}
	return strings.TrimSpace(config.Order)
}

func resolveRelationOptionOrder(config Relation) string {
	if order := strings.TrimSpace(config.OptionOrder); order != "" {
		return order
	}
	if strings.TrimSpace(config.Through) != "" {
		return ""
	}
	return strings.TrimSpace(config.Order)
}

func resolveRelationModel(config Relation) RelationModel {
	modelName := strings.TrimSpace(config.Through)
	if modelName == "" {
		return nil
	}

	if modelValue, ok := frontrecord.LoadSafe(modelName).(RelationModel); ok {
		return modelValue
	}
	adapter := frontrecord.ResolveAdapter(modelName)
	if adapter == nil || !adapter.HasMethod("SelectMap", 3) || !adapter.HasMethod("Delete", 2) || !adapter.HasMethod("Insert", 2) {
		return nil
	}
	return adapter
}
