package access

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/shemic/dever/server"

	frontpagepath "my/package/front/internal/pagepath"
	authctx "my/package/front/service/internal/authctx"
	"my/package/front/service/permission"
	"my/package/front/service/siteconfig"
	uploadrepo "my/package/front/service/upload/repository"
)

type Operation string

const (
	OperationRead   Operation = "read"
	OperationSign   Operation = "sign"
	OperationList   Operation = "list"
	OperationManage Operation = "manage"
)

type Decision int

const (
	Abstain Decision = iota
	Allow
	Deny
)

type Request struct {
	Operation      Operation
	File           *uploadrepo.UploadFile
	BizKey         string
	Kind           string
	CategoryID     string
	ActorID        int64
	Site           siteconfig.Site
	ClientPagePath string
}

type PolicyFunc func(context.Context, Request) (Decision, error)

type accessError struct {
	status  int
	message string
}

type registeredPolicy struct {
	key    string
	prefix bool
	fn     PolicyFunc
}

var (
	policyMu sync.RWMutex
	policies []registeredPolicy
)

func (e accessError) Error() string {
	return e.message
}

func RegisterBizPolicy(bizKey string, policy PolicyFunc) {
	registerPolicy(bizKey, false, policy)
}

func RegisterBizPrefixPolicy(prefix string, policy PolicyFunc) {
	registerPolicy(prefix, true, policy)
}

func EnsureFile(c *server.Context, operation Operation, file uploadrepo.UploadFile) error {
	if file.ID == 0 {
		return forbidden("上传文件不存在")
	}
	request := requestFromContext(c, Request{
		Operation: operation,
		File:      &file,
		BizKey:    file.BizKey,
		Kind:      file.Kind,
	})
	return ensure(c, request)
}

func EnsureResourceRequest(c *server.Context, request Request) error {
	request = requestFromContext(c, request)
	if request.Operation == "" {
		request.Operation = OperationList
	}
	return ensure(c, request)
}

func Status(err error) int {
	var target accessError
	if errors.As(err, &target) && target.status > 0 {
		return target.status
	}
	return http.StatusForbidden
}

func ClientPageHasPrefix(request Request, prefixes ...string) bool {
	pagePath := frontpagepath.NormalizePath(request.ClientPagePath)
	if pagePath == "" {
		return false
	}
	for _, prefix := range prefixes {
		normalized := frontpagepath.NormalizePath(prefix)
		if normalized == "" {
			continue
		}
		if pagePath == normalized || strings.HasPrefix(pagePath, normalized+"/") {
			return true
		}
	}
	return false
}

func EnsureClientPageAccess(ctx context.Context, request Request) error {
	pagePath := frontpagepath.NormalizePath(request.ClientPagePath)
	if pagePath == "" {
		return forbidden("缺少页面访问上下文")
	}
	return permission.EnsurePageAccess(ctx, pagePath)
}

func registerPolicy(key string, prefix bool, policy PolicyFunc) {
	normalized := normalizePolicyKey(key)
	if normalized == "" || policy == nil {
		return
	}
	policyMu.Lock()
	defer policyMu.Unlock()
	policies = append(policies, registeredPolicy{
		key:    normalized,
		prefix: prefix,
		fn:     policy,
	})
}

func ensure(c *server.Context, request Request) error {
	if c == nil {
		return unauthorized("请先登录")
	}
	ctx := c.Context()
	if request.ActorID <= 0 {
		return unauthorized("请先登录")
	}

	decision, err := policyDecision(ctx, request)
	if err != nil {
		return err
	}
	switch decision {
	case Allow:
		return nil
	case Deny:
		return forbidden("暂无资源访问权限")
	}

	if canAccessResourceCenter(ctx, request) {
		return nil
	}
	if canAccessClientPageForBiz(ctx, request) {
		return nil
	}
	return forbidden("暂无资源访问权限")
}

func requestFromContext(c *server.Context, request Request) Request {
	if c == nil {
		return request
	}
	request.ActorID = authctx.OptionalUID(c.Context())
	if site, ok := siteconfig.FromContext(c.Context()); ok {
		request.Site = site
	}
	request.ClientPagePath = clientPagePath(c)
	if request.File != nil && request.BizKey == "" {
		request.BizKey = request.File.BizKey
	}
	request.BizKey = uploadrepo.NormalizeBizKey(request.BizKey)
	request.Kind = strings.TrimSpace(request.Kind)
	request.CategoryID = strings.TrimSpace(request.CategoryID)
	return request
}

func policyDecision(ctx context.Context, request Request) (Decision, error) {
	policyMu.RLock()
	items := append([]registeredPolicy(nil), policies...)
	policyMu.RUnlock()

	bizKey := normalizePolicyKey(request.BizKey)
	for _, item := range items {
		if !policyMatches(item, bizKey) {
			continue
		}
		decision, err := item.fn(ctx, request)
		if err != nil {
			return Deny, err
		}
		if decision != Abstain {
			return decision, nil
		}
	}
	return Abstain, nil
}

func policyMatches(policy registeredPolicy, bizKey string) bool {
	if bizKey == "" {
		return false
	}
	if policy.prefix {
		return strings.HasPrefix(bizKey, policy.key)
	}
	return bizKey == policy.key
}

func canAccessResourceCenter(ctx context.Context, request Request) bool {
	if !request.Site.UsesRBAC() {
		return false
	}
	return permission.EnsurePageAccess(ctx, "front/resource/list") == nil
}

func canAccessClientPageForBiz(ctx context.Context, request Request) bool {
	prefix := bizPagePrefix(request.BizKey)
	if prefix == "" || !ClientPageHasPrefix(request, prefix) {
		return false
	}
	return EnsureClientPageAccess(ctx, request) == nil
}

func bizPagePrefix(bizKey string) string {
	bizKey = uploadrepo.NormalizeBizKey(bizKey)
	if bizKey == "" {
		return ""
	}
	first := bizKey
	for _, sep := range []string{".", "_", "-"} {
		if index := strings.Index(first, sep); index > 0 {
			first = first[:index]
		}
	}
	switch first {
	case "resource":
		return "front"
	case "energon":
		return "bot"
	default:
		return first
	}
}

func clientPagePath(c *server.Context) string {
	raw := strings.TrimSpace(c.Header("X-Client-Page"))
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		raw = parsed.Path
	}
	return frontpagepath.NormalizePath(raw)
}

func normalizePolicyKey(value string) string {
	return strings.ToLower(uploadrepo.NormalizeBizKey(value))
}

func unauthorized(message string) error {
	return accessError{status: http.StatusUnauthorized, message: message}
}

func forbidden(message string) error {
	return accessError{status: http.StatusForbidden, message: message}
}
