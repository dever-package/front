package permission

import (
	"context"
	"fmt"
	"time"

	devercache "github.com/shemic/dever/cache"
	"github.com/shemic/dever/util"

	authctx "my/package/front/service/internal/authctx"
	embedpageservice "my/package/front/service/permission/embedpage"
	"my/package/front/service/runtimecache"
	"my/package/front/service/siteconfig"
)

const (
	configMetaCacheTTL    = 5 * time.Minute
	authRecordsCacheTTL   = 5 * time.Minute
	authGraphCacheTTL     = 5 * time.Minute
	accessSnapshotTTL     = 30 * time.Second
	mainInfoCacheTTL      = 30 * time.Second
	permissionCacheMaxKey = 512
)

var (
	configMetaCache = devercache.New[string, configMeta](
		devercache.WithTTL(configMetaCacheTTL),
		devercache.WithMaxEntries(permissionCacheMaxKey),
	)
	authRecordsCache = devercache.New[string, []authRecord](
		devercache.WithTTL(authRecordsCacheTTL),
		devercache.WithMaxEntries(permissionCacheMaxKey),
	)
	authGraphCache = devercache.New[string, authGraph](
		devercache.WithTTL(authGraphCacheTTL),
		devercache.WithMaxEntries(permissionCacheMaxKey),
	)
	accessSnapshotCache = devercache.New[string, *accessSnapshot](
		devercache.WithTTL(accessSnapshotTTL),
		devercache.WithMaxEntries(permissionCacheMaxKey),
	)
	mainInfoCache = devercache.New[string, map[string]any](
		devercache.WithTTL(mainInfoCacheTTL),
		devercache.WithMaxEntries(permissionCacheMaxKey),
	)
)

func init() {
	runtimecache.Register("front.permission", invalidatePermissionCaches, clearPermissionCaches)
}

func invalidatePermissionCaches() {
	configMetaCache.Invalidate()
	authRecordsCache.Invalidate()
	authGraphCache.Invalidate()
	accessSnapshotCache.Invalidate()
	mainInfoCache.Invalidate()
	embedpageservice.ClearCache()
}

func clearPermissionCaches() {
	configMetaCache.Clear()
	authRecordsCache.Clear()
	authGraphCache.Clear()
	accessSnapshotCache.Clear()
	mainInfoCache.Clear()
	embedpageservice.ClearCache()
}

func permissionSitePageKey(ctx context.Context) string {
	return fmt.Sprintf(
		"%s:%s",
		siteconfig.SiteKeyFromContext(ctx),
		siteconfig.PageFromContext(ctx),
	)
}

func permissionUserKey(ctx context.Context) string {
	return fmt.Sprintf("%s:%d", permissionSitePageKey(ctx), authctx.OptionalUID(ctx))
}

func mainInfoCacheKey(ctx context.Context, includePermissions bool) string {
	return fmt.Sprintf("%s:%t", permissionUserKey(ctx), includePermissions)
}

func cloneAuthRecords(records []authRecord) []authRecord {
	if records == nil {
		return nil
	}
	cloned := make([]authRecord, 0, len(records))
	for _, record := range records {
		next := record
		next.Query = cloneAuthQuery(record.Query)
		cloned = append(cloned, next)
	}
	return cloned
}

func cloneAuthQuery(query authQuery) authQuery {
	if query == nil {
		return nil
	}
	cloned := make(authQuery, len(query))
	for key, value := range query {
		cloned[key] = value
	}
	return cloned
}

func cloneMainInfoPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = cloneCacheValue(value)
	}
	return cloned
}

func cloneCacheValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, item := range current {
			result[key] = cloneCacheValue(item)
		}
		return result
	case []map[string]any:
		result := make([]map[string]any, 0, len(current))
		for _, item := range current {
			if item == nil {
				continue
			}
			if cloned, ok := cloneCacheValue(item).(map[string]any); ok {
				result = append(result, cloned)
			}
		}
		return result
	case []any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			result = append(result, cloneCacheValue(item))
		}
		return result
	case authQuery:
		return cloneAuthQuery(current)
	default:
		return value
	}
}

func prepareAuthRows(rows []map[string]any) []map[string]any {
	if rows == nil {
		return nil
	}
	prepared := util.CloneMapSlice(rows)
	for _, row := range prepared {
		if len(row) > 0 {
			row["query"] = parseAuthQuery(row["query"])
		}
	}
	return prepared
}
