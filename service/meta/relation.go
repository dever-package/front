package meta

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"

	frontoption "my/package/front/service/option"
	frontrecord "my/package/front/service/record"
)

type OptionModel interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
}

type RelationModel interface {
	SelectMap(ctx context.Context, filters any, options ...map[string]any) []map[string]any
	Delete(ctx context.Context, filters any) int64
	Insert(ctx context.Context, record map[string]any) int64
}

type Relation = orm.Relation

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

	optionKeys := relationOptionKeys(config)
	if len(optionKeys) == 0 {
		return nil
	}

	result := make(map[string]any, len(optionKeys))
	for _, key := range optionKeys {
		result[key] = util.CloneMapSlice(items)
	}

	return result
}

func relationOptionKeys(config Relation) []string {
	if strings.ToLower(strings.TrimSpace(config.Kind)) != "option" {
		return nil
	}
	if strings.TrimSpace(config.Option) == "" || len(config.OptionKeys) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	keys := make([]string, 0, len(config.OptionKeys)+2)
	appendKey := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	appendKey(config.Field)
	appendKey(config.Name)
	for _, key := range config.OptionKeys {
		appendKey(key)
	}
	return keys
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
		if err := insertRelationRecord(ctx, modelName, relationValue, record); err != nil {
			return err
		}
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

	items, ok := normalizeChildrenRelationItems(childrenValue)
	if !ok {
		return nil
	}

	modelName := strings.TrimSpace(config.Through)
	modelValue := load.Model(modelName)
	columnLookup := frontrecord.ResolveColumnLookup(modelName, modelValue)
	if len(columnLookup) == 0 {
		return nil
	}

	primaryColumn := frontrecord.ResolveColumnName(columnLookup, "id")
	if primaryColumn == "" {
		relationValue.Delete(ctx, map[string]any{config.OwnerField: owner})
		return insertChildrenRelationRows(ctx, modelName, relationValue, owner, items, config, columnLookup)
	}

	existingRows := relationValue.SelectMap(ctx, map[string]any{config.OwnerField: owner})
	existingByID := make(map[uint64]map[string]any, len(existingRows))
	for _, row := range existingRows {
		if id := util.ToUint64(row[primaryColumn]); id > 0 {
			existingByID[id] = row
		}
	}

	keptIDs := map[uint64]struct{}{}
	if len(items) > 0 {
		if err := upsertChildrenRelationRows(ctx, modelName, relationValue, owner, items, config, columnLookup, existingByID, keptIDs); err != nil {
			return err
		}
	}

	for id := range existingByID {
		if _, kept := keptIDs[id]; kept {
			continue
		}
		relationValue.Delete(ctx, map[string]any{
			config.OwnerField: owner,
			primaryColumn:     id,
		})
	}

	return nil
}

func normalizeChildrenRelationItems(value any) ([]any, bool) {
	switch items := value.(type) {
	case []any:
		return items, true
	case []map[string]any:
		result := make([]any, 0, len(items))
		for _, item := range items {
			if item != nil {
				result = append(result, item)
			}
		}
		return result, true
	default:
		return nil, false
	}
}

func insertChildrenRelationRows(
	ctx context.Context,
	modelName string,
	relationValue RelationModel,
	owner uint64,
	items []any,
	config Relation,
	columnLookup map[string]string,
) error {
	if len(items) == 0 {
		return nil
	}

	ownerColumn := frontrecord.ResolveColumnName(columnLookup, config.OwnerField)
	createdAtColumn := frontrecord.ResolveColumnName(columnLookup, "created_at")
	for _, item := range items {
		child, ok := item.(map[string]any)
		if !ok {
			continue
		}

		data := frontrecord.SanitizeRecord(child, columnLookup)
		delete(data, ownerColumn)
		delete(data, createdAtColumn)
		if isEmptyChildRecord(data) {
			continue
		}

		data[config.OwnerField] = owner
		frontrecord.ApplyCreatedAt(data, columnLookup)
		if err := insertRelationRecord(ctx, modelName, relationValue, data); err != nil {
			return err
		}
	}

	return nil
}

func upsertChildrenRelationRows(
	ctx context.Context,
	modelName string,
	relationValue RelationModel,
	owner uint64,
	items []any,
	config Relation,
	columnLookup map[string]string,
	existingByID map[uint64]map[string]any,
	keptIDs map[uint64]struct{},
) error {
	primaryColumn := frontrecord.ResolveColumnName(columnLookup, "id")
	ownerColumn := frontrecord.ResolveColumnName(columnLookup, config.OwnerField)
	createdAtColumn := frontrecord.ResolveColumnName(columnLookup, "created_at")
	updatedAtColumn := frontrecord.ResolveColumnName(columnLookup, "updated_at")
	for _, item := range items {
		child, ok := item.(map[string]any)
		if !ok {
			continue
		}

		data := frontrecord.SanitizeRecord(child, columnLookup)
		childID := util.ToUint64(data[primaryColumn])

		delete(data, primaryColumn)
		delete(data, ownerColumn)
		delete(data, createdAtColumn)
		delete(data, updatedAtColumn)
		if isEmptyChildRecord(data) {
			continue
		}

		if childID > 0 {
			if _, exists := existingByID[childID]; exists {
				updateRelationRecord(ctx, relationValue, map[string]any{
					config.OwnerField: owner,
					primaryColumn:     childID,
				}, data)
				keptIDs[childID] = struct{}{}
				continue
			}
		}

		data[config.OwnerField] = owner
		frontrecord.ApplyCreatedAt(data, columnLookup)
		if err := insertRelationRecord(ctx, modelName, relationValue, data); err != nil {
			return err
		}
	}

	return nil
}

func updateRelationRecord(ctx context.Context, relationValue RelationModel, filters any, record map[string]any) int64 {
	method := reflect.ValueOf(relationValue).MethodByName("Update")
	if !method.IsValid() {
		return 0
	}

	out := method.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(filters),
		reflect.ValueOf(record),
	})
	if len(out) == 0 {
		return 0
	}
	return util.ToInt64(out[0].Interface())
}

func insertRelationRecord(
	ctx context.Context,
	modelName string,
	relationValue RelationModel,
	record map[string]any,
) error {
	err := tryInsertRelationRecord(ctx, relationValue, record)
	if err == nil {
		return nil
	}
	if !isPrimaryKeyDuplicateError(err) {
		return err
	}
	if syncErr := frontrecord.SyncModelPrimarySequence(ctx, modelName); syncErr != nil {
		return err
	}
	return tryInsertRelationRecord(ctx, relationValue, record)
}

func tryInsertRelationRecord(
	ctx context.Context,
	relationValue RelationModel,
	record map[string]any,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	relationValue.Insert(ctx, record)
	return nil
}

func isPrimaryKeyDuplicateError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if !strings.Contains(message, "duplicate key value violates unique constraint") {
		return false
	}
	return strings.Contains(message, "_pkey") || strings.Contains(message, "primary key")
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
	if adapter == nil ||
		!adapter.HasMethod("SelectMap", 3) ||
		!adapter.HasMethod("Delete", 2) ||
		!adapter.HasMethod("Insert", 2) {
		return nil
	}
	return adapter
}
