package eval

import (
	"context"
	"fmt"
	"strings"
)

func Run(ctx context.Context, req Request) (Result, error) {
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Result{}, err
	}

	switch normalized.Language {
	case LanguageJavaScript:
		return runJavaScript(ctx, normalized)
	default:
		return Result{}, Error{
			Code:    ErrorCodeUnsupported,
			Message: fmt.Sprintf("不支持的 eval 语言: %s", normalized.Language),
		}
	}
}

func normalizeRequest(req Request) (Request, error) {
	req.Language = normalizeLanguage(req.Language)
	req.Entry = strings.TrimSpace(req.Entry)
	if req.Entry == "" {
		req.Entry = DefaultEntry
	}
	if req.Timeout <= 0 {
		req.Timeout = DefaultTimeout
	}
	if req.Timeout > MaxTimeout {
		req.Timeout = MaxTimeout
	}
	if req.MaxScriptBytes <= 0 {
		req.MaxScriptBytes = DefaultMaxScriptBytes
	}
	if req.MaxOutputBytes <= 0 {
		req.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if req.MaxOutputDepth <= 0 {
		req.MaxOutputDepth = DefaultMaxOutputDepth
	}
	if req.MaxOutputArrayLength <= 0 {
		req.MaxOutputArrayLength = DefaultMaxOutputArrayLength
	}

	if strings.TrimSpace(req.Script) == "" {
		return Request{}, Error{Code: ErrorCodeInvalidRequest, Message: "eval script 不能为空"}
	}
	if len([]byte(req.Script)) > req.MaxScriptBytes {
		return Request{}, Error{Code: ErrorCodeInvalidRequest, Message: "eval script 超过大小限制"}
	}
	if req.Entry == "" {
		return Request{}, Error{Code: ErrorCodeInvalidRequest, Message: "eval entry 不能为空"}
	}
	return req, nil
}

func normalizeLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", LanguageJavaScript, LanguageJS:
		return LanguageJavaScript
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
