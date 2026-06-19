package operationlog

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	frontpagepath "github.com/dever-package/front/internal/pagepath"
	frontauthctx "github.com/dever-package/front/service/internal/authctx"
	frontmeta "github.com/dever-package/front/service/meta"
	frontrecord "github.com/dever-package/front/service/record"
)

const (
	accountLogModelName = "front.NewAccountLogModel"
	maxTextLength       = 255
	maxPayloadLength    = 4000
)

type Entry struct {
	AccountID   uint64
	AccountName string
	Account     string
	Action      string
	Method      string
	Path        string
	PagePath    string
	TargetModel string
	TargetID    string
	Message     string
	Payload     any
}

func Record(c *server.Context, entry Entry) {
	record(c, entry, nil)
}

func RecordForAccount(c *server.Context, account map[string]any, entry Entry) {
	record(c, entry, account)
}

func RecordAction(
	c *server.Context,
	requestPath string,
	pathValue string,
	modelName string,
	actionType string,
	primaryKey string,
	payload any,
	result map[string]any,
) {
	action := normalizeActionType(actionType, result)
	targetID := firstNonEmptyText(
		ExtractID(result, primaryKey),
		ExtractID(payload, primaryKey),
	)

	Record(c, Entry{
		Action:      action,
		PagePath:    firstNonEmptyText(pathValue, requestPath),
		TargetModel: modelName,
		TargetID:    targetID,
		Message:     buildActionMessage(action, modelName, targetID),
		Payload:     payload,
	})
}

func ExtractID(value any, primaryKey string) string {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		primaryKey = "id"
	}

	switch current := value.(type) {
	case nil:
		return ""
	case map[string]any:
		for _, key := range []string{primaryKey, "id"} {
			if raw, ok := frontrecord.ReadValue(current, key); ok && frontrecord.HasValue(raw) {
				return valueText(raw)
			}
		}
	case []any:
		values := make([]string, 0, len(current))
		for _, item := range current {
			if text := ExtractID(item, primaryKey); text != "" {
				values = append(values, text)
			}
		}
		return strings.Join(values, ",")
	default:
		if frontrecord.HasValue(current) {
			return valueText(current)
		}
	}

	return ""
}

func record(c *server.Context, entry Entry, account map[string]any) {
	defer func() {
		_ = recover()
	}()

	if c == nil {
		return
	}

	entry = withAccount(c, entry, account)
	if entry.AccountID == 0 {
		return
	}

	model := frontrecord.Resolve(accountLogModelName)
	if model == nil {
		return
	}

	model.Insert(c.Context(), map[string]any{
		"account_id":   entry.AccountID,
		"account_name": limitText(entry.AccountName, 64),
		"account":      limitText(entry.Account, 64),
		"action":       limitText(firstNonEmptyText(entry.Action, "request"), 32),
		"method":       limitText(firstNonEmptyText(entry.Method, c.Method()), 16),
		"path":         limitText(firstNonEmptyText(entry.Path, c.Path()), maxTextLength),
		"page_path":    limitText(frontpagepath.NormalizePath(entry.PagePath), maxTextLength),
		"target_model": limitText(entry.TargetModel, 128),
		"target_id":    limitText(entry.TargetID, 128),
		"message":      limitText(entry.Message, maxTextLength),
		"ip":           limitText(requestIP(c), 64),
		"user_agent":   limitText(c.Header("User-Agent"), maxTextLength),
		"payload":      encodePayload(entry.Payload),
		"created_at":   time.Now(),
	})
}

func withAccount(c *server.Context, entry Entry, account map[string]any) Entry {
	if len(account) == 0 && entry.AccountID == 0 {
		if uid := frontauthctx.OptionalUID(c.Context()); uid > 0 {
			accountModel := frontrecord.Resolve("front.NewAccountModel")
			if accountModel != nil {
				account = accountModel.FindMap(c.Context(), map[string]any{"id": uid})
			}
		}
	}

	if entry.AccountID == 0 {
		entry.AccountID = util.ToUint64(account["id"])
	}
	if entry.AccountName == "" {
		entry.AccountName = util.ToStringTrimmed(account["name"])
	}
	if entry.Account == "" {
		entry.Account = util.ToStringTrimmed(account["account"])
	}
	return entry
}

func normalizeActionType(actionType string, result map[string]any) string {
	actionType = strings.ToLower(strings.TrimSpace(actionType))
	if actionType == "save" {
		if util.ToBool(result["created"]) {
			return "create"
		}
		if util.ToBool(result["updated"]) {
			return "update"
		}
	}
	if actionType != "" {
		return actionType
	}
	return "request"
}

func buildActionMessage(action string, modelName string, targetID string) string {
	modelLabel := frontmeta.ResolveModelName(modelName)
	if modelLabel == "" {
		modelLabel = frontrecord.ResourceName(modelName)
	}
	if modelLabel == "" {
		modelLabel = modelName
	}

	actionLabel := map[string]string{
		"create": "新增",
		"update": "编辑",
		"delete": "删除",
		"sync":   "同步",
		"export": "导出",
		"import": "导入",
		"upload": "上传",
		"login":  "登录",
	}[action]
	if actionLabel == "" {
		actionLabel = "操作"
	}

	message := strings.TrimSpace(actionLabel + " " + modelLabel)
	if targetID != "" {
		message += " #" + targetID
	}
	return message
}

func requestIP(c *server.Context) string {
	if c == nil {
		return ""
	}
	for _, key := range []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"} {
		if ip := firstRequestIP(c.Header(key)); ip != "" {
			return ip
		}
	}

	if raw := c.Raw; raw != nil {
		if getter, ok := raw.(interface{ IP() string }); ok {
			if ip := normalizeRequestIP(getter.IP()); ip != "" {
				return ip
			}
		}
		if getter, ok := raw.(interface{ Request() *http.Request }); ok {
			if req := getter.Request(); req != nil {
				if ip := normalizeRequestIP(req.RemoteAddr); ip != "" {
					return ip
				}
			}
		}
	}

	return ""
}

func firstRequestIP(value string) string {
	for _, part := range strings.Split(value, ",") {
		if ip := normalizeRequestIP(part); ip != "" {
			return ip
		}
	}
	return ""
}

func normalizeRequestIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "unknown") {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(value, "[]")
	if parsed := net.ParseIP(value); parsed != nil {
		return parsed.String()
	}
	return value
}

func encodePayload(payload any) string {
	if payload == nil {
		return ""
	}

	normalized := sanitizePayload(payload)
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return limitText(fmt.Sprint(normalized), maxPayloadLength)
	}
	return limitText(string(encoded), maxPayloadLength)
}

func sanitizePayload(value any) any {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, item := range current {
			if isSensitiveKey(key) {
				result[key] = "***"
				continue
			}
			result[key] = sanitizePayload(item)
		}
		return result
	case []any:
		result := make([]any, 0, len(current))
		for _, item := range current {
			result = append(result, sanitizePayload(item))
		}
		return result
	default:
		return current
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(normalized)
	for _, keyword := range []string{"password", "token", "secret", "authorization", "accesskey", "secretkey", "apikey", "api_key", "credential"} {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}

func firstNonEmptyText(values ...any) string {
	for _, value := range values {
		text := valueText(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func valueText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func limitText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return value
	}

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
