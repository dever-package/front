package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shemic/dever/component"
	"github.com/shemic/dever/util"

	frontpagepath "my/package/front/internal/pagepath"
	pagecontent "my/package/front/service/internal/pagecontent"
	embedpageservice "my/package/front/service/permission/embedpage"
	frontrecord "my/package/front/service/record"
	"my/package/front/service/runtimecache"
	"my/package/front/service/siteconfig"
)

func EnsureBootstrap(ctx context.Context) error {
	site, ok := siteconfig.FromContext(ctx)
	if !ok {
		site = siteconfig.MustLoad().Sites[0]
	}
	return EnsureBootstrapForSite(ctx, site)
}

func EnsureBootstrapForSite(ctx context.Context, site siteconfig.Site) error {
	if !shouldBootstrapSite(site) {
		return nil
	}
	if bootstrapState.done.Load() {
		return nil
	}

	bootstrapState.mu.Lock()
	defer bootstrapState.mu.Unlock()

	if bootstrapState.done.Load() {
		return nil
	}

	if err := runBootstrap(ctx); err != nil {
		return err
	}
	bootstrapState.done.Store(true)
	runtimecache.Invalidate()
	return nil
}

func ForceBootstrap(ctx context.Context) error {
	site, ok := siteconfig.FromContext(ctx)
	if !ok {
		site = siteconfig.MustLoad().Sites[0]
	}
	return ForceBootstrapForSite(ctx, site)
}

func ForceBootstrapForSite(ctx context.Context, site siteconfig.Site) error {
	if !shouldBootstrapSite(site) {
		return nil
	}

	bootstrapState.mu.Lock()
	defer bootstrapState.mu.Unlock()

	runtimecache.Invalidate()
	if err := runBootstrap(ctx); err != nil {
		return err
	}
	bootstrapState.done.Store(true)
	runtimecache.Invalidate()
	return nil
}

func WarmupSites(ctx context.Context, sites []siteconfig.Site) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, site := range sites {
		if err := WarmupSite(ctx, site); err != nil {
			return fmt.Errorf("预热 front 站点 %s 失败: %w", site.Key, err)
		}
	}
	return nil
}

func WarmupSite(ctx context.Context, site siteconfig.Site) error {
	if strings.TrimSpace(site.Key) == "" {
		return nil
	}

	siteCtx := siteconfig.WithSite(ctx, site)
	if err := EnsureBootstrapForSite(siteCtx, site); err != nil {
		return err
	}

	if _, err := loadConfigMetaForSite(site.Key); err != nil {
		return err
	}
	if _, err := collectAuthRecords(siteCtx); err != nil {
		return err
	}
	if site.UsesRBAC() {
		if _, err := loadAuthGraph(siteCtx); err != nil {
			return err
		}
		if _, err := loadAccessSnapshot(siteCtx); err != nil {
			return err
		}
	}
	return nil
}

func shouldBootstrapSite(site siteconfig.Site) bool {
	return site.UsesRBAC() && strings.TrimSpace(site.Access.AuthProvider) == siteconfig.DefaultAuthProvider
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

	rows := filterAuthRowsForCurrentSite(ctx, authModel.SelectMap(ctx, nil))
	rows = FilterAssignableAuthRowsForSite(siteconfig.SiteKeyFromContext(ctx), rows)
	return map[string]any{
		"page":     1,
		"pageSize": len(rows),
		"total":    len(rows),
		"tree":     true,
		"list":     rows,
	}
}

func syncAuthRecords(ctx context.Context) error {
	records, err := collectBootstrapAuthRecords(ctx)
	if err != nil {
		return err
	}

	authModel := frontrecord.Resolve("front.NewAuthModel")
	if authModel == nil {
		if len(records) == 0 {
			return nil
		}
		return fmt.Errorf("权限模型未注册")
	}

	columnLookup := frontrecord.ResolveColumnLookup("front.NewAuthModel", authModel)
	for _, record := range records {
		if err := upsertAuthRecord(ctx, authModel, columnLookup, record); err != nil {
			return err
		}
	}
	if err := removeStaleManagedAuthRecords(ctx, authModel, columnLookup, records); err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
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

func collectBootstrapAuthRecords(ctx context.Context) ([]authRecord, error) {
	sites, err := bootstrapSites(ctx)
	if err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, nil
	}

	merged := make(map[string]authRecord)
	for _, site := range sites {
		siteCtx := siteconfig.WithSite(ctx, site)
		records, err := collectAuthRecords(siteCtx)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			key := strings.TrimSpace(record.Key)
			if key == "" {
				continue
			}
			merged[key] = record
		}
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

func bootstrapSites(ctx context.Context) ([]siteconfig.Site, error) {
	cfg, err := siteconfig.Load(ctx)
	if err != nil {
		return nil, err
	}

	sites := make([]siteconfig.Site, 0, len(cfg.Sites))
	for _, site := range cfg.Sites {
		if shouldBootstrapSite(site) {
			sites = append(sites, site)
		}
	}
	return sites, nil
}

func removeStaleManagedAuthRecords(ctx context.Context, authModel frontrecord.Model, columnLookup map[string]string, records []authRecord) error {
	if _, ok := columnLookup["managed"]; !ok {
		return nil
	}
	if _, ok := columnLookup["source_type"]; !ok {
		return nil
	}
	if _, ok := columnLookup["source_name"]; !ok {
		return nil
	}

	activeKeys := make(map[string]struct{}, len(records))
	for _, record := range records {
		if key := strings.TrimSpace(record.Key); key != "" {
			activeKeys[key] = struct{}{}
		}
	}

	rows := authModel.SelectMap(ctx, map[string]any{"managed": 1}, map[string]any{
		"field": "main.id, main.key",
	})
	if len(rows) == 0 {
		return nil
	}

	roleAuthModel := frontrecord.Resolve("front.NewRoleAuthModel")
	for _, row := range rows {
		key := util.ToStringTrimmed(row["key"])
		if _, ok := activeKeys[key]; ok {
			continue
		}
		authID := util.ToUint64(row["id"])
		if authID == 0 {
			continue
		}
		if roleAuthModel != nil && authID > 0 {
			roleAuthModel.Delete(ctx, map[string]any{"auth_id": authID})
		}
		authModel.Delete(ctx, map[string]any{"id": authID})
	}
	return nil
}

func collectAuthRecords(ctx context.Context) ([]authRecord, error) {
	records, err := authRecordsCache.GetOrSet(permissionSitePageKey(ctx), func() ([]authRecord, error) {
		return collectAuthRecordsUncached(ctx)
	})
	if err != nil {
		return nil, err
	}
	return cloneAuthRecords(records), nil
}

func collectAuthRecordsUncached(ctx context.Context) ([]authRecord, error) {
	componentAuths, err := loadComponentAuthRecords(ctx)
	if err != nil {
		return nil, err
	}
	pageAuths, err := loadPageAuthRecords(ctx)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]authRecord)
	for _, record := range append(componentAuths, pageAuths...) {
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

func loadComponentAuthRecords(ctx context.Context) ([]authRecord, error) {
	cfg, err := siteconfig.Load(context.Background())
	if err != nil {
		return nil, err
	}
	siteKey := siteconfig.SiteKeyFromContext(ctx)
	if _, ok := cfg.FindBySiteKey(siteKey); !ok {
		siteKey = siteconfig.DefaultSiteKey
	}

	records := make([]authRecord, 0)
	for _, current := range component.Active() {
		site, ok := current.Manifest.Front.Sites[siteKey]
		if !ok {
			continue
		}
		source := recordSource{Type: current.Source, Name: current.Name}
		var walk func(parent string, items []component.AuthSeed)
		walk = func(parent string, items []component.AuthSeed) {
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
					ParentKey: componentAuthParentKey(parent, item.Parent),
					Type:      normalizeAuthType(item.Type, keyValue, pathValue),
					Sort:      item.Sort,
					Query:     normalizeAuthQuery(item.Query),
					Source:    source,
				}
				records = append(records, record)
				walk(keyValue, item.Children)
			}
		}
		walk("", site.Auth)
	}
	return records, nil
}

func componentAuthParentKey(parent string, explicitParent string) string {
	if current := strings.TrimSpace(explicitParent); current != "" {
		return current
	}
	return strings.TrimSpace(parent)
}

func loadPageAuthRecords(ctx context.Context) ([]authRecord, error) {
	recordMap := make(map[string]authRecord)
	recordPriorityMap := make(map[string]int)
	pageName := siteconfig.PageFromContext(ctx)
	embeddedPaths := embedpageservice.PathsForPage(pageName)

	if err := pagecontent.WalkComponentPages(pageName, func(page pagecontent.ComponentPage) error {
		meta, err := parsePageMeta(page.Content)
		if err != nil {
			return nil
		}
		if _, embedded := embeddedPaths[page.Path]; embedded {
			return nil
		}
		savePageAuthRecords(recordMap, recordPriorityMap, page.Content, meta, page.Path, page.FileName, recordSource{
			Type: page.Component.Source,
			Name: page.Component.Name,
		})
		return nil
	}); err != nil {
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

func FilterAssignableAuthRowsForSite(siteKey string, rows []map[string]any) []map[string]any {
	cfg, err := siteconfig.Load(context.Background())
	if err != nil {
		return rows
	}
	site, ok := cfg.FindBySiteKey(siteKey)
	if !ok {
		return rows
	}
	return embedpageservice.FilterRowsForPage(site.Page, rows)
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
	source recordSource,
) {
	for _, record := range append(
		buildPageAuthRecords(meta, routePath),
		buildPageActionAuthRecords(content, routePath)...,
	) {
		record.Source = source
		savePageAuthRecord(recordMap, recordPriorityMap, record, fileName)
	}
}

// Page-level create/update auth usually lives in update.json. Explicit parent
// config wins; otherwise it is grouped under the matching list page.
func pageAuthParentKey(item authSeed, keyValue string, meta pageMeta, routePath string) string {
	if parent := strings.TrimSpace(item.Parent); parent != "" {
		return parent
	}
	if parent := strings.TrimSpace(meta.Parent); parent != "" {
		return parent
	}

	for _, candidate := range []string{keyValue, item.Path, routePath} {
		if parent := listParentPath(candidate); parent != "" {
			return parent
		}
	}
	return ""
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
	segments := strings.Split(frontpagepath.NormalizePath(pathValue), "/")
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
			fallbackKey,
			util.ToString(item["key"]),
			util.ToString(item["id"]),
		)
	case "import":
		return util.FirstNonEmpty(
			util.ToString(config["importKey"]),
			fallbackKey,
			util.ToString(item["key"]),
			util.ToString(item["id"]),
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
	priority := frontpagepath.PageFilePriority(fileName)
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

func normalizeAuthQuery(query map[string]string) authQuery {
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
	if _, ok := columnLookup["source_type"]; ok {
		data["source_type"] = strings.TrimSpace(record.Source.Type)
	}
	if _, ok := columnLookup["source_name"]; ok {
		data["source_name"] = strings.TrimSpace(record.Source.Name)
	}
	if _, ok := columnLookup["managed"]; ok {
		data["managed"] = managedAuthRecord(record)
	}

	if len(current) == 0 {
		frontrecord.ApplyCreatedAt(data, columnLookup)
		authModel.Insert(ctx, data)
		return nil
	}

	authModel.Update(ctx, map[string]any{"id": current["id"]}, data)
	return nil
}

func managedAuthRecord(record authRecord) int {
	if strings.TrimSpace(record.Source.Type) == "" || strings.TrimSpace(record.Source.Name) == "" {
		return 0
	}
	return 1
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
	return frontrecord.SyncModelPrimarySequence(ctx, modelName)
}

func loadConfigMetaForSite(siteKey string) (configMeta, error) {
	return configMetaCache.GetOrSet(siteKey, func() (configMeta, error) {
		cfg, err := siteconfig.Load(context.Background())
		if err != nil {
			return configMeta{}, err
		}
		site, ok := cfg.FindBySiteKey(siteKey)
		if !ok {
			site, _ = cfg.FindBySiteKey(siteconfig.DefaultSiteKey)
		}
		if site.Key == "" {
			return configMeta{}, nil
		}
		return configMeta{
			Entry: site.Entry,
		}, nil
	})
}
