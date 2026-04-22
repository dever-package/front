package permission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	frontroot "github.com/dever-package/front"
	"github.com/shemic/dever/orm"
	"github.com/shemic/dever/util"

	embedpageservice "github.com/dever-package/front/service/embedpage"
	frontpage "github.com/dever-package/front/service/page"
	frontrecord "github.com/dever-package/front/service/record"
)

func EnsureBootstrap(ctx context.Context) error {
	bootstrapState.mu.Lock()
	defer bootstrapState.mu.Unlock()

	if bootstrapState.done {
		return nil
	}

	if err := runBootstrap(ctx); err != nil {
		return err
	}
	bootstrapState.done = true
	return nil
}

func ForceBootstrap(ctx context.Context) error {
	bootstrapState.mu.Lock()
	defer bootstrapState.mu.Unlock()

	embedpageservice.ClearCache()
	if err := runBootstrap(ctx); err != nil {
		return err
	}
	bootstrapState.done = true
	return nil
}

func runBootstrap(ctx context.Context) error {
	if err := syncAuthRecords(ctx); err != nil {
		return err
	}
	if err := ensureDefaultRole(ctx); err != nil {
		return err
	}
	if err := grantDefaultRoleAllAuths(ctx); err != nil {
		return err
	}
	if err := ensureAccountRoleLinks(ctx); err != nil {
		return err
	}
	if err := SyncModelPrimarySequence(ctx, "front.NewRoleModel"); err != nil {
		return err
	}
	if err := SyncModelPrimarySequence(ctx, "front.NewAccountModel"); err != nil {
		return err
	}
	return nil
}

func BuildAuthTablePayload(ctx context.Context) map[string]any {
	authModel := frontrecord.Resolve("front.NewAuthModel")
	if authModel == nil {
		return map[string]any{
			"page":     1,
			"pageSize": 0,
			"total":    0,
			"tree":     true,
			"list":     []map[string]any{},
		}
	}

	rows := FilterAssignableAuthRows(authModel.SelectMap(ctx, nil))
	return map[string]any{
		"page":     1,
		"pageSize": len(rows),
		"total":    len(rows),
		"tree":     true,
		"list":     rows,
	}
}

func syncAuthRecords(ctx context.Context) error {
	records, err := collectAuthRecords()
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	authModel := frontrecord.Resolve("front.NewAuthModel")
	if authModel == nil {
		return fmt.Errorf("权限模型未注册")
	}

	columnLookup := frontrecord.ResolveColumnLookup("front.NewAuthModel", authModel)
	for _, record := range records {
		if err := upsertAuthRecord(ctx, authModel, columnLookup, record); err != nil {
			return err
		}
	}

	keyToID := loadAuthKeyMap(ctx, authModel)
	for _, record := range records {
		row := authModel.FindMap(ctx, map[string]any{"key": record.Key})
		if len(row) == 0 {
			continue
		}

		parentID := uint64(0)
		if strings.TrimSpace(record.ParentKey) != "" {
			parentID = keyToID[record.ParentKey]
		}
		currentParent := util.ToUint64(row["parent_id"])
		if currentParent == parentID {
			continue
		}
		authModel.Update(ctx, map[string]any{"id": row["id"]}, map[string]any{"parent_id": parentID})
	}

	return nil
}

func collectAuthRecords() ([]authRecord, error) {
	configAuths, err := loadConfigAuthRecords()
	if err != nil {
		return nil, err
	}
	pageAuths, err := loadPageAuthRecords()
	if err != nil {
		return nil, err
	}

	merged := make(map[string]authRecord)
	for _, record := range append(configAuths, pageAuths...) {
		key := strings.TrimSpace(record.Key)
		if key == "" {
			continue
		}
		merged[key] = record
	}

	records := make([]authRecord, 0, len(merged))
	for _, record := range merged {
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Type != records[j].Type {
			return records[i].Type < records[j].Type
		}
		if records[i].Sort != records[j].Sort {
			return records[i].Sort < records[j].Sort
		}
		return records[i].Key < records[j].Key
	})
	return records, nil
}

func loadConfigAuthRecords() ([]authRecord, error) {
	payload, err := loadConfigMeta()
	if err != nil {
		return nil, fmt.Errorf("读取 config/front.json 失败: %w", err)
	}

	records := make([]authRecord, 0)
	var walk func(parent string, items []authSeed)
	walk = func(parent string, items []authSeed) {
		for _, item := range items {
			keyValue := strings.TrimSpace(util.FirstNonEmpty(item.Key, item.ID))
			if keyValue == "" {
				continue
			}
			pathValue := strings.TrimSpace(item.Path)
			record := authRecord{
				Key:       keyValue,
				Name:      util.FirstNonEmpty(item.Name, keyValue, pathValue),
				Icon:      strings.TrimSpace(item.Icon),
				Path:      pathValue,
				ParentKey: strings.TrimSpace(parent),
				Type:      normalizeAuthType(item.Type, keyValue, pathValue),
				Sort:      item.Sort,
				Query:     normalizeAuthQuery(item.Query),
			}
			records = append(records, record)
			walk(keyValue, item.Children)
		}
	}

	walk("", payload.Auth)
	return records, nil
}

func loadPageAuthRecords() ([]authRecord, error) {
	recordMap := make(map[string]authRecord)
	recordPriorityMap := make(map[string]int)
	embeddedPaths := embedpageservice.Paths()
	err := filepath.WalkDir("module", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() || !frontpage.IsPageFileName(entry.Name()) {
			return nil
		}

		cleanPath := filepath.ToSlash(filepath.Clean(path))
		parts := strings.Split(cleanPath, "/")
		if len(parts) < 4 || parts[0] != "module" || !frontpage.IsPageDir(parts[2]) {
			return nil
		}
		if parts[1] == "front" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		meta, err := parsePageMeta(content)
		if err != nil {
			return nil
		}

		routePath := frontpage.TrimPageFileExt(strings.Join(append([]string{parts[1]}, parts[3:]...), "/"))
		routePath = frontpage.NormalizePath(routePath)
		if routePath == "" {
			return nil
		}
		if _, embedded := embeddedPaths[routePath]; embedded {
			return nil
		}

		savePageAuthRecords(recordMap, recordPriorityMap, content, meta, routePath, entry.Name())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描页面权限失败")
	}

	err = fs.WalkDir(frontroot.PageFS, "page", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() || !frontpage.IsPageFileName(entry.Name()) {
			return nil
		}

		content, err := frontroot.PageFS.ReadFile(path)
		if err != nil {
			return err
		}
		meta, err := parsePageMeta(content)
		if err != nil {
			return nil
		}

		relativePath := strings.TrimPrefix(filepath.ToSlash(path), "page/")
		routePath := frontpage.TrimPageFileExt(filepath.ToSlash(filepath.Join("front", relativePath)))
		routePath = frontpage.NormalizePath(routePath)
		if routePath == "" {
			return nil
		}
		if _, embedded := embeddedPaths[routePath]; embedded {
			return nil
		}

		savePageAuthRecords(recordMap, recordPriorityMap, content, meta, routePath, entry.Name())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描页面权限失败")
	}

	records := make([]authRecord, 0, len(recordMap))
	for _, record := range recordMap {
		records = append(records, record)
	}
	return records, nil
}

func FilterAssignableAuthRows(rows []map[string]any) []map[string]any {
	return embedpageservice.FilterRows(rows)
}

func buildPageAuthRecords(meta pageMeta, routePath string) []authRecord {
	if len(meta.Auth) > 0 {
		records := make([]authRecord, 0, len(meta.Auth))
		for _, item := range meta.Auth {
			keyValue := strings.TrimSpace(util.FirstNonEmpty(item.Key, item.ID))
			if keyValue == "" {
				continue
			}
			sortValue := item.Sort
			if sortValue == 0 {
				sortValue = meta.Sort
			}
			records = append(records, authRecord{
				Key:       keyValue,
				Name:      util.FirstNonEmpty(strings.TrimSpace(item.Name), keyValue),
				Icon:      util.FirstNonEmpty(strings.TrimSpace(item.Icon), strings.TrimSpace(meta.Icon)),
				Path:      routePath,
				ParentKey: pageAuthParentKey(item, keyValue, meta, routePath),
				Type:      normalizeAuthType(item.Type, keyValue, routePath),
				Sort:      sortValue,
				Query:     normalizeAuthQuery(item.Query),
			})
		}
		if len(records) > 0 {
			return records
		}
	}

	return []authRecord{
		{
			Key:       routePath,
			Name:      util.FirstNonEmpty(strings.TrimSpace(meta.Name), strings.TrimSpace(meta.Title), routePath),
			Icon:      strings.TrimSpace(meta.Icon),
			Path:      routePath,
			ParentKey: strings.TrimSpace(meta.Parent),
			Type:      normalizeAuthType(meta.Type, routePath, routePath),
			Sort:      meta.Sort,
		},
	}
}

func savePageAuthRecords(
	recordMap map[string]authRecord,
	recordPriorityMap map[string]int,
	content []byte,
	meta pageMeta,
	routePath string,
	fileName string,
) {
	for _, record := range append(
		buildPageAuthRecords(meta, routePath),
		buildPageActionAuthRecords(content, routePath)...,
	) {
		savePageAuthRecord(recordMap, recordPriorityMap, record, fileName)
	}
}

// Page-level create/update auth usually lives in update.json, but it should be
// grouped under the same resource list page as delete/import/export actions.
func pageAuthParentKey(item authSeed, keyValue string, meta pageMeta, routePath string) string {
	if parent := strings.TrimSpace(item.Parent); parent != "" {
		return parent
	}

	for _, candidate := range []string{keyValue, item.Path, routePath} {
		if parent := listParentPath(candidate); parent != "" {
			return parent
		}
	}

	return strings.TrimSpace(meta.Parent)
}

func listParentPath(pathValue string) string {
	parts := pathParts(pathValue)
	if len(parts) < 2 {
		return ""
	}

	last := strings.ToLower(parts[len(parts)-1])
	if !isFormAction(last) {
		return ""
	}

	parts[len(parts)-1] = "list"
	return strings.Join(parts, "/")
}

func pathParts(pathValue string) []string {
	segments := strings.Split(frontpage.NormalizePath(pathValue), "/")
	result := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment != "" {
			result = append(result, segment)
		}
	}
	return result
}

func isFormAction(segment string) bool {
	switch segment {
	case "create", "update", "edit":
		return true
	default:
		return false
	}
}

func buildPageActionAuthRecords(content []byte, routePath string) []authRecord {
	var payload struct {
		Nodes  map[string][]map[string]any `json:"nodes"`
		Action map[string]any              `json:"action"`
	}
	if err := util.UnmarshalJSONC(content, &payload); err != nil {
		return nil
	}

	recordMap := make(map[string]authRecord)
	for _, items := range payload.Nodes {
		for _, item := range items {
			collectItemActionAuthRecords(recordMap, routePath, item, payload.Action)
		}
	}

	records := make([]authRecord, 0, len(recordMap))
	for _, record := range recordMap {
		records = append(records, record)
	}
	return records
}

func collectItemActionAuthRecords(
	recordMap map[string]authRecord,
	routePath string,
	item map[string]any,
	namedActions map[string]any,
) {
	if len(item) == 0 {
		return
	}

	actionMap, _ := item["action"].(map[string]any)
	collectActionAuthValue(recordMap, routePath, actionMap["click"], item, namedActions, "")
	collectActionAuthValue(recordMap, routePath, actionMap["confirm"], item, namedActions, "")

	if children, ok := item["items"].([]any); ok {
		for _, child := range children {
			if childItem, ok := child.(map[string]any); ok {
				collectItemActionAuthRecords(recordMap, routePath, childItem, namedActions)
			}
		}
	}

	meta, _ := item["meta"].(map[string]any)
	if createButton, ok := meta["createButton"].(map[string]any); ok {
		collectItemActionAuthRecords(recordMap, routePath, createButton, namedActions)
	}
	if buttons, ok := meta["buttons"].([]any); ok {
		for _, button := range buttons {
			if buttonItem, ok := button.(map[string]any); ok {
				collectItemActionAuthRecords(recordMap, routePath, buttonItem, namedActions)
			}
		}
	}
}

func collectActionAuthValue(
	recordMap map[string]authRecord,
	routePath string,
	raw any,
	item map[string]any,
	namedActions map[string]any,
	fallbackKey string,
) {
	switch value := raw.(type) {
	case nil:
		return
	case string:
		actionName := strings.TrimSpace(value)
		if actionName == "" {
			return
		}
		if actionConfig, ok := namedActions[actionName]; ok {
			collectActionAuthValue(recordMap, routePath, actionConfig, item, namedActions, actionName)
		}
	case []any:
		for _, itemValue := range value {
			collectActionAuthValue(recordMap, routePath, itemValue, item, namedActions, fallbackKey)
		}
	case map[string]any:
		collectActionAuthConfig(recordMap, routePath, value, item, fallbackKey)
	}
}

func collectActionAuthConfig(
	recordMap map[string]authRecord,
	routePath string,
	config map[string]any,
	item map[string]any,
	fallbackKey string,
) {
	actionKey := normalizeActionPermissionKey(actionAuthKey(config, item, fallbackKey))
	if actionKey == "" {
		return
	}
	fullKey := routePath + "/" + actionKey
	recordMap[fullKey] = authRecord{
		Key:       fullKey,
		Name:      util.FirstNonEmpty(util.ToString(item["name"]), util.ToString(config["name"]), actionKey),
		Path:      routePath,
		ParentKey: routePath,
		Type:      2,
		Sort:      1000,
	}
}

func actionAuthKey(config map[string]any, item map[string]any, fallbackKey string) string {
	switch strings.ToLower(strings.TrimSpace(util.ToString(config["type"]))) {
	case "export":
		return util.FirstNonEmpty(
			util.ToString(config["exportKey"]),
			util.ToString(item["key"]),
			util.ToString(item["id"]),
			fallbackKey,
		)
	case "import":
		return util.FirstNonEmpty(
			util.ToString(config["importKey"]),
			util.ToString(item["key"]),
			util.ToString(item["id"]),
			fallbackKey,
		)
	case "delete":
		return util.FirstNonEmpty(
			util.ToString(config["key"]),
			fallbackKey,
			util.ToString(item["key"]),
			util.ToString(item["id"]),
			normalizeActionStateKey(item),
		)
	default:
		return ""
	}
}

func normalizeActionStateKey(item map[string]any) string {
	meta, _ := item["meta"].(map[string]any)
	stateKey := strings.TrimSpace(util.ToString(meta["stateKey"]))
	if stateKey == "" {
		return ""
	}
	return strings.ReplaceAll(stateKey, ".", "-")
}

func normalizeActionPermissionKey(key string) string {
	key = strings.Trim(strings.TrimSpace(key), "/")
	key = strings.ReplaceAll(key, ".", "-")
	return key
}

func savePageAuthRecord(
	recordMap map[string]authRecord,
	recordPriorityMap map[string]int,
	record authRecord,
	fileName string,
) {
	priority := frontpage.PageFilePriority(fileName)
	if currentPriority, ok := recordPriorityMap[record.Key]; ok && currentPriority <= priority {
		return
	}
	recordMap[record.Key] = record
	recordPriorityMap[record.Key] = priority
}

func parsePageMeta(content []byte) (pageMeta, error) {
	var payload struct {
		Page pageMeta `json:"page"`
	}
	if err := util.UnmarshalJSONC(content, &payload); err != nil {
		return pageMeta{}, err
	}
	return payload.Page, nil
}

func normalizeAuthType(current int, keyValue, pathValue string) int {
	if current == 1 || current == 2 {
		return current
	}
	if strings.TrimSpace(pathValue) == "" {
		return 1
	}
	if strings.HasSuffix(strings.TrimSpace(pathValue), "/list") {
		return 1
	}
	if strings.TrimSpace(keyValue) != "" && !strings.Contains(strings.TrimSpace(keyValue), "/") {
		return 1
	}
	return 2
}

func normalizeAuthQuery(query authQuery) authQuery {
	if len(query) == 0 {
		return nil
	}
	result := make(authQuery, len(query))
	for key, rule := range query {
		keyValue := strings.TrimSpace(key)
		ruleValue := strings.ToLower(strings.TrimSpace(rule))
		if keyValue == "" || ruleValue == "" {
			continue
		}
		result[keyValue] = ruleValue
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func encodeAuthQuery(query authQuery) string {
	query = normalizeAuthQuery(query)
	if len(query) == 0 {
		return ""
	}
	content, err := json.Marshal(query)
	if err != nil {
		return ""
	}
	return string(content)
}

func upsertAuthRecord(
	ctx context.Context,
	authModel frontrecord.Model,
	columnLookup map[string]string,
	record authRecord,
) error {
	current := authModel.FindMap(ctx, map[string]any{"key": record.Key})
	data := map[string]any{
		"key":       record.Key,
		"name":      record.Name,
		"icon":      util.FirstNonEmpty(record.Icon, ""),
		"path":      util.FirstNonEmpty(record.Path, ""),
		"parent_id": 0,
		"type":      record.Type,
		"sort":      record.Sort,
	}
	if _, ok := columnLookup["query"]; ok {
		data["query"] = encodeAuthQuery(record.Query)
	}

	if len(current) == 0 {
		frontrecord.ApplyCreatedAt(data, columnLookup)
		authModel.Insert(ctx, data)
		return nil
	}

	authModel.Update(ctx, map[string]any{"id": current["id"]}, data)
	return nil
}

func loadAuthKeyMap(ctx context.Context, authModel frontrecord.Model) map[string]uint64 {
	rows := authModel.SelectMap(ctx, nil, map[string]any{"field": "main.id, main.key"})
	result := make(map[string]uint64, len(rows))
	for _, row := range rows {
		keyValue := util.ToStringTrimmed(row["key"])
		if keyValue == "" {
			continue
		}
		result[keyValue] = util.ToUint64(row["id"])
	}
	return result
}

func ensureDefaultRole(ctx context.Context) error {
	roleModel := frontrecord.Resolve("front.NewRoleModel")
	if roleModel == nil {
		return fmt.Errorf("角色模型未注册")
	}

	existing := roleModel.FindMap(ctx, map[string]any{"id": defaultRoleID})
	if len(existing) > 0 {
		if util.ToStringTrimmed(existing["name"]) != defaultRoleName {
			roleModel.Update(ctx, map[string]any{"id": defaultRoleID}, map[string]any{"name": defaultRoleName})
		}
		_ = SyncModelPrimarySequence(ctx, "front.NewRoleModel")
		return nil
	}

	columnLookup := frontrecord.ResolveColumnLookup("front.NewRoleModel", roleModel)
	record := map[string]any{
		"id":   defaultRoleID,
		"name": defaultRoleName,
	}
	frontrecord.ApplyCreatedAt(record, columnLookup)
	roleModel.Insert(ctx, record)
	_ = SyncModelPrimarySequence(ctx, "front.NewRoleModel")
	return nil
}

func EnsureDefaultAccount(ctx context.Context, account, password string, hashPassword func(string) string) error {
	accountModel := frontrecord.Resolve("front.NewAccountModel")
	if accountModel == nil {
		return fmt.Errorf("账户模型未注册")
	}

	if accountModel.Count(ctx, nil) > 0 {
		_ = SyncModelPrimarySequence(ctx, "front.NewAccountModel")
		return nil
	}

	account = strings.TrimSpace(account)
	password = strings.TrimSpace(password)
	if account == "" || password == "" {
		return fmt.Errorf("首次登录需要输入账户和密码")
	}

	columnLookup := frontrecord.ResolveColumnLookup("front.NewAccountModel", accountModel)
	record := map[string]any{
		"id":       defaultAccountID,
		"name":     defaultAccountName,
		"account":  account,
		"password": hashPassword(password),
		"role_id":  defaultRoleID,
	}
	frontrecord.ApplyCreatedAt(record, columnLookup)
	accountID := accountModel.Insert(ctx, record)
	ensureAccountRoleLink(ctx, uint64(accountID), defaultRoleID)
	_ = SyncModelPrimarySequence(ctx, "front.NewAccountModel")
	return nil
}

func grantDefaultRoleAllAuths(ctx context.Context) error {
	authModel := frontrecord.Resolve("front.NewAuthModel")
	roleAuthModel := frontrecord.Resolve("front.NewRoleAuthModel")
	if authModel == nil || roleAuthModel == nil {
		return fmt.Errorf("权限关联模型未注册")
	}

	authRows := authModel.SelectMap(ctx, nil, map[string]any{"field": "main.id"})
	if len(authRows) == 0 {
		return nil
	}

	existingRows := roleAuthModel.SelectMap(ctx, map[string]any{"role_id": defaultRoleID}, map[string]any{
		"field": "main.auth_id",
	})
	existing := make(map[uint64]struct{}, len(existingRows))
	for _, row := range existingRows {
		existing[util.ToUint64(row["auth_id"])] = struct{}{}
	}

	columnLookup := frontrecord.ResolveColumnLookup("front.NewRoleAuthModel", roleAuthModel)
	for _, row := range authRows {
		authID := util.ToUint64(row["id"])
		if authID == 0 {
			continue
		}
		if _, ok := existing[authID]; ok {
			continue
		}

		record := map[string]any{
			"role_id": defaultRoleID,
			"auth_id": authID,
		}
		frontrecord.ApplyCreatedAt(record, columnLookup)
		roleAuthModel.Insert(ctx, record)
	}

	return nil
}

func ensureAccountRoleLinks(ctx context.Context) error {
	accountModel := frontrecord.Resolve("front.NewAccountModel")
	accountRoleModel := frontrecord.Resolve("front.NewAccountRoleModel")
	if accountModel == nil || accountRoleModel == nil {
		return nil
	}

	accountRows := accountModel.SelectMap(ctx, nil, map[string]any{"field": "main.id, main.role_id"})
	if len(accountRows) == 0 {
		return nil
	}

	relationRows := accountRoleModel.SelectMap(ctx, nil, map[string]any{"field": "main.account_id, main.role_id"})
	existing := make(map[uint64]map[uint64]struct{}, len(relationRows))
	for _, row := range relationRows {
		accountID := util.ToUint64(row["account_id"])
		roleID := util.ToUint64(row["role_id"])
		if accountID == 0 || roleID == 0 {
			continue
		}
		if existing[accountID] == nil {
			existing[accountID] = map[uint64]struct{}{}
		}
		existing[accountID][roleID] = struct{}{}
	}

	for _, row := range accountRows {
		accountID := util.ToUint64(row["id"])
		if accountID == 0 || len(existing[accountID]) > 0 {
			continue
		}
		roleID := util.ToUint64(row["role_id"])
		if roleID == 0 {
			continue
		}
		ensureAccountRoleLink(ctx, accountID, roleID)
	}

	return nil
}

func ensureAccountRoleLink(ctx context.Context, accountID, roleID uint64) {
	if accountID == 0 || roleID == 0 {
		return
	}

	accountRoleModel := frontrecord.Resolve("front.NewAccountRoleModel")
	if accountRoleModel == nil {
		return
	}

	existing := accountRoleModel.FindMap(ctx, map[string]any{
		"account_id": accountID,
		"role_id":    roleID,
	})
	if len(existing) > 0 {
		return
	}

	columnLookup := frontrecord.ResolveColumnLookup("front.NewAccountRoleModel", accountRoleModel)
	record := map[string]any{
		"account_id": accountID,
		"role_id":    roleID,
	}
	frontrecord.ApplyCreatedAt(record, columnLookup)
	accountRoleModel.Insert(ctx, record)
}

func SyncModelPrimarySequence(ctx context.Context, modelName string) error {
	tableName, primaryColumn, err := loadModelPrimarySequenceInfo(modelName)
	if err != nil {
		return err
	}
	if tableName == "" || primaryColumn == "" {
		return nil
	}

	db, err := orm.Get()
	if err != nil {
		return err
	}
	if normalizeDatabaseDriver(db.DriverName()) != "postgres" {
		return nil
	}

	var sequenceName sql.NullString
	if err := db.QueryRowContext(
		ctx,
		"SELECT pg_get_serial_sequence($1, $2)",
		tableName,
		primaryColumn,
	).Scan(&sequenceName); err != nil {
		return err
	}
	if !sequenceName.Valid || strings.TrimSpace(sequenceName.String) == "" {
		return nil
	}

	statement := fmt.Sprintf(
		"SELECT setval($1::regclass, COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false)",
		quotePostgresIdentifier(primaryColumn),
		quotePostgresTableName(tableName),
	)
	_, err = db.ExecContext(ctx, statement, sequenceName.String)
	return err
}

func loadModelPrimarySequenceInfo(modelName string) (string, string, error) {
	resourceName := frontrecord.ResourceName(modelName)
	if resourceName == "" {
		return "", "", nil
	}

	entries, err := os.ReadDir(filepath.Join("data", "table"))
	if err != nil {
		return "", "", err
	}

	type schemaColumn struct {
		Name          string `json:"name"`
		Primary       bool   `json:"primary"`
		AutoIncrement bool   `json:"autoIncrement"`
	}
	type tableSchemaFile struct {
		Table   string         `json:"table"`
		Columns []schemaColumn `json:"columns"`
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name != resourceName+".json" && !strings.HasSuffix(name, "_"+resourceName+".json") {
			continue
		}

		content, readErr := os.ReadFile(filepath.Join("data", "table", name))
		if readErr != nil {
			continue
		}

		var schema tableSchemaFile
		if jsonErr := json.Unmarshal(content, &schema); jsonErr != nil {
			continue
		}

		primaryColumn := ""
		for _, column := range schema.Columns {
			if column.Primary && column.AutoIncrement {
				primaryColumn = strings.TrimSpace(column.Name)
				break
			}
		}
		if primaryColumn == "" {
			for _, column := range schema.Columns {
				if column.Primary {
					primaryColumn = strings.TrimSpace(column.Name)
					break
				}
			}
		}

		return strings.TrimSpace(schema.Table), primaryColumn, nil
	}

	return "", "", nil
}

func normalizeDatabaseDriver(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "postgres", "postgresql", "pgx":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func quotePostgresIdentifier(name string) string {
	parts := strings.Split(strings.TrimSpace(name), ".")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(part, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, ".")
}

func quotePostgresTableName(name string) string {
	return quotePostgresIdentifier(name)
}

func loadConfigMeta() (configMeta, error) {
	content, actualPath, err := util.ReadJSONCFile(
		filepath.Join("config", "front.jsonc"),
		filepath.Join("config", "front.json"),
	)
	if err != nil {
		if os.IsNotExist(err) {
			return configMeta{}, nil
		}
		return configMeta{}, err
	}

	signature := frontpage.Signature(content)
	if cached, ok := configMetaCache.Load(actualPath); ok {
		entry := cached
		if entry.signature == signature {
			return entry.value, nil
		}
	}

	var payload configMeta
	if err := util.UnmarshalNormalizedJSON(content, &payload); err != nil {
		return configMeta{}, err
	}

	configMetaCache.Store(actualPath, configMetaCacheEntry{
		signature: signature,
		value:     payload,
	})
	return payload, nil
}
