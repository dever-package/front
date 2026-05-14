package permission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/shemic/dever/util"

	authctx "my/package/front/service/authctx"
	embedpageservice "my/package/front/service/embedpage"
	frontpage "my/package/front/service/page"
	frontrecord "my/package/front/service/record"
)

type InputLookup func(key string) string

type AccessScope struct {
	snapshot *accessSnapshot
}

var errNoPermission = errors.New("暂无权限")

const (
	inheritInputKey          = "_inherit"
	inheritParentPathKey     = "_parentPath"
	inheritParentInputPrefix = "_parent_"
)

func EnsurePageAccess(ctx context.Context, pathValue string) error {
	return EnsurePageAccessWithInput(ctx, pathValue, nil)
}

func NewAccessScope(ctx context.Context) (*AccessScope, error) {
	snapshot, err := loadAccessSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &AccessScope{snapshot: snapshot}, nil
}

func (scope *AccessScope) EnsurePageAccess(ctx context.Context, pathValue string, lookup InputLookup) error {
	if scope == nil {
		return nil
	}
	return ensurePageAccessWithSnapshot(ctx, scope.snapshot, pathValue, lookup)
}

func EnsurePageAccessWithInput(ctx context.Context, pathValue string, lookup InputLookup) error {
	pathValue = frontpage.NormalizePath(pathValue)
	if pathValue == "" {
		return nil
	}

	snapshot, err := loadAccessSnapshot(ctx)
	if err != nil || snapshot == nil {
		return err
	}
	return ensurePageAccessWithSnapshot(ctx, snapshot, pathValue, lookup)
}

func ensurePageAccessWithSnapshot(ctx context.Context, snapshot *accessSnapshot, pathValue string, lookup InputLookup) error {
	pathValue = frontpage.NormalizePath(pathValue)
	if pathValue == "" {
		return nil
	}
	if snapshot == nil {
		return nil
	}

	authRow, protected := resolveAccessAuthRow(snapshot.auth, pathValue, lookup)
	if embedpageservice.HasPath(pathValue) {
		if canInheritPageAccess(ctx, snapshot, pathValue, lookup) {
			return nil
		}
		return errNoPermission
	}
	if protected && canAccessAuthRow(snapshot, authRow) {
		return nil
	}
	if !protected {
		return nil
	}

	return errNoPermission
}

func PayloadInputLookup(payload any, fallback InputLookup) InputLookup {
	return func(key string) string {
		value := strings.TrimSpace(lookupPayloadInput(payload, key))
		if value != "" {
			return value
		}
		if fallback == nil {
			return ""
		}
		return fallback(key)
	}
}

func resolveAccessAuthRow(
	graph authGraph,
	pathValue string,
	lookup InputLookup,
) (map[string]any, bool) {
	rows := graph.rowByPath[pathValue]
	if len(rows) == 0 {
		return nil, false
	}

	var fallback map[string]any
	for _, row := range rows {
		query := authRowQuery(row)
		if len(query) == 0 {
			if fallback == nil || authRowKey(row) == pathValue {
				fallback = row
			}
			continue
		}
		if matchAuthQuery(query, lookup) {
			return row, true
		}
	}

	if fallback != nil {
		return fallback, true
	}
	return nil, true
}

func authRowQuery(row map[string]any) authQuery {
	if len(row) == 0 {
		return nil
	}
	if query, ok := row["query"].(authQuery); ok {
		return query
	}

	query := parseAuthQuery(row["query"])
	row["query"] = query
	return query
}

func loadAccessSnapshot(ctx context.Context) (*accessSnapshot, error) {
	graph, err := loadAuthGraph(ctx)
	if err != nil {
		return nil, err
	}
	roleIDs := resolveCurrentRoleIDs(ctx)
	if len(graph.rows) == 0 {
		return &accessSnapshot{
			auth:    graph,
			roleIDs: roleIDs,
			allowed: map[uint64]struct{}{},
		}, nil
	}
	return &accessSnapshot{
		auth:    graph,
		roleIDs: roleIDs,
		allowed: resolveAllowedAuthSet(ctx, graph, roleIDs...),
	}, nil
}

func loadAuthGraph(ctx context.Context) (authGraph, error) {
	authModel := frontrecord.Resolve("front.NewAuthModel")
	if authModel == nil {
		return authGraph{}, nil
	}

	rows := authModel.SelectMap(ctx, nil, map[string]any{
		"field": "main.id, main.key, main.name, main.icon, main.path, main.parent_id, main.sort, main.type, main.query",
		"order": "main.sort asc, main.id asc",
	})
	graph := authGraph{
		rows:       rows,
		rowByPath:  make(map[string][]map[string]any, len(rows)),
		rowByKey:   make(map[string]map[string]any, len(rows)),
		parentByID: make(map[uint64]uint64, len(rows)),
		allIDs:     make(map[uint64]struct{}, len(rows)),
	}
	for _, row := range rows {
		id := authRowID(row)
		if id == 0 {
			continue
		}
		graph.allIDs[id] = struct{}{}
		graph.parentByID[id] = util.ToUint64(row["parent_id"])
		if key := authRowKey(row); key != "" {
			graph.rowByKey[key] = row
		}
		if path := authRowPath(row); path != "" {
			graph.rowByPath[path] = append(graph.rowByPath[path], row)
		}
	}
	return graph, nil
}

func EnsureActionAccess(ctx context.Context, pathValue string, actionKey string) error {
	_, err := CheckActionAccess(ctx, pathValue, actionKey)
	return err
}

func CheckActionAccess(ctx context.Context, pathValue string, actionKey string) (bool, error) {
	pathValue = frontpage.NormalizePath(pathValue)
	actionKey = strings.TrimSpace(actionKey)
	if pathValue == "" || actionKey == "" {
		return false, nil
	}

	fullKey := pathValue + "/" + actionKey
	if strings.Contains(actionKey, "/") {
		fullKey = frontpage.NormalizePath(actionKey)
	}
	snapshot, err := loadAccessSnapshot(ctx)
	if err != nil || snapshot == nil {
		return false, err
	}

	authRow, ok := snapshot.auth.rowByKey[fullKey]
	if !ok || len(authRow) == 0 {
		return false, nil
	}

	if canAccessAuthRow(snapshot, authRow) {
		return true, nil
	}
	return true, errNoPermission
}

func canAccessAuthRow(snapshot *accessSnapshot, row map[string]any) bool {
	if snapshot == nil || len(row) == 0 {
		return false
	}
	if hasDefaultRole(snapshot.roleIDs) {
		return true
	}
	_, ok := snapshot.allowed[authRowID(row)]
	return ok
}

func canInheritPageAccess(ctx context.Context, snapshot *accessSnapshot, pathValue string, lookup InputLookup) bool {
	if lookup == nil || !allowsInheritedAccess(lookup(inheritInputKey)) {
		return false
	}

	parentPath := frontpage.NormalizePath(lookup(inheritParentPathKey))
	if parentPath == "" || parentPath == pathValue || !embedpageservice.IsChild(parentPath, pathValue) {
		return false
	}

	return ensurePageAccessWithSnapshot(ctx, snapshot, parentPath, parentAccessLookup(lookup)) == nil
}

func allowsInheritedAccess(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "form":
		return true
	default:
		return false
	}
}

func parentAccessLookup(lookup InputLookup) InputLookup {
	return func(key string) string {
		return lookup(inheritParentInputPrefix + strings.TrimSpace(key))
	}
}

func authRowID(row map[string]any) uint64 {
	return util.ToUint64(row["id"])
}

func authRowKey(row map[string]any) string {
	return util.ToStringTrimmed(row["key"])
}

func authRowPath(row map[string]any) string {
	return util.ToStringTrimmed(row["path"])
}

func matchAuthQuery(query authQuery, lookup InputLookup) bool {
	if len(query) == 0 {
		return true
	}
	for key, rule := range query {
		value := ""
		if lookup != nil {
			value = lookup(key)
		}
		value = strings.TrimSpace(value)
		rule = strings.TrimSpace(rule)
		empty := isEmptyAuthInput(key, value)
		switch strings.ToLower(rule) {
		case "required", "notempty", "not_empty":
			if empty {
				return false
			}
		case "empty":
			if !empty {
				return false
			}
		default:
			if value != rule {
				return false
			}
		}
	}
	return true
}

func isEmptyAuthInput(key string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}

	if isAuthIDInput(key) {
		number, ok := util.ParseInt64(value)
		return ok && number <= 0
	}

	return false
}

func isAuthIDInput(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return key == "id" || strings.HasSuffix(key, ".id") || strings.HasSuffix(key, "_id")
}

func parseAuthQuery(value any) authQuery {
	switch current := value.(type) {
	case nil:
		return nil
	case authQuery:
		return normalizeAuthQuery(current)
	case map[string]string:
		return normalizeAuthQuery(authQuery(current))
	case map[string]any:
		result := make(authQuery, len(current))
		for key, item := range current {
			result[key] = util.ToStringTrimmed(item)
		}
		return normalizeAuthQuery(result)
	case []byte:
		return parseAuthQueryString(string(current))
	case string:
		return parseAuthQueryString(current)
	default:
		return parseAuthQueryString(util.ToString(current))
	}
}

func parseAuthQueryString(raw string) authQuery {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var query authQuery
	if err := json.Unmarshal([]byte(raw), &query); err != nil {
		return nil
	}
	return normalizeAuthQuery(query)
}

func lookupPayloadInput(payload any, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}

	current := payload
	for _, part := range strings.Split(key, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return ""
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case map[string]string:
			current = typed[part]
		default:
			return ""
		}
	}

	return normalizePayloadInputValue(key, current)
}

func normalizePayloadInputValue(key string, value any) string {
	if !frontrecord.HasValue(value) {
		return ""
	}

	text := util.ToStringTrimmed(value)
	if isEmptyAuthInput(key, text) {
		return ""
	}
	return text
}

func resolveAllowedAuthSet(
	ctx context.Context,
	graph authGraph,
	roleIDs ...uint64,
) map[uint64]struct{} {
	roleIDs = normalizeRoleIDs(roleIDs)
	if hasDefaultRole(roleIDs) {
		return graph.allIDs
	}

	roleAuthModel := frontrecord.Resolve("front.NewRoleAuthModel")
	if roleAuthModel == nil {
		return nil
	}

	rows := roleAuthModel.SelectMap(ctx, map[string]any{"role_id": roleIDs}, map[string]any{
		"field": "main.auth_id",
	})
	allowed := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		current := util.ToUint64(row["auth_id"])
		for current != 0 {
			if _, exists := allowed[current]; exists {
				break
			}
			allowed[current] = struct{}{}
			current = graph.parentByID[current]
		}
	}
	return allowed
}

func resolveCurrentRoleIDs(ctx context.Context) []uint64 {
	uid := authctx.OptionalUID(ctx)
	if uid <= 0 {
		return []uint64{defaultRoleID}
	}
	return ResolveAccountRoleIDs(ctx, uint64(uid))
}

func ResolveAccountRoleIDs(ctx context.Context, accountID uint64) []uint64 {
	if accountID == 0 {
		return []uint64{defaultRoleID}
	}

	accountRoleModel := frontrecord.Resolve("front.NewAccountRoleModel")
	if accountRoleModel != nil {
		rows := accountRoleModel.SelectMap(ctx, map[string]any{"account_id": accountID}, map[string]any{
			"field": "main.role_id",
			"order": "main.id asc",
		})
		roleIDs := make([]uint64, 0, len(rows))
		for _, row := range rows {
			roleIDs = append(roleIDs, util.ToUint64(row["role_id"]))
		}
		roleIDs = util.UniqueUint64s(roleIDs)
		if len(roleIDs) > 0 {
			return roleIDs
		}
	}

	accountModel := frontrecord.Resolve("front.NewAccountModel")
	if accountModel == nil {
		return []uint64{defaultRoleID}
	}

	row := accountModel.FindMap(ctx, map[string]any{"id": accountID})
	return normalizeRoleIDs([]uint64{util.ToUint64(row["role_id"])})
}

func normalizeRoleIDs(roleIDs []uint64) []uint64 {
	normalized := util.UniqueUint64s(roleIDs)
	if len(normalized) == 0 {
		return []uint64{defaultRoleID}
	}
	return normalized
}

func hasDefaultRole(roleIDs []uint64) bool {
	for _, roleID := range roleIDs {
		if roleID == defaultRoleID {
			return true
		}
	}
	return false
}
