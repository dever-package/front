package eval

import "time"

const (
	LanguageJavaScript = "javascript"
	LanguageJS         = "js"

	DefaultEntry                = "evaluate"
	DefaultTimeout              = 100 * time.Millisecond
	MaxTimeout                  = time.Second
	DefaultMaxScriptBytes       = 64 * 1024
	DefaultMaxOutputBytes       = 64 * 1024
	DefaultMaxOutputDepth       = 8
	DefaultMaxOutputArrayLength = 2000
)

const (
	ErrorCodeInvalidRequest     = "invalid_request"
	ErrorCodeUnsupported        = "unsupported_language"
	ErrorCodeCompileFailed      = "compile_failed"
	ErrorCodeEntryMissing       = "entry_missing"
	ErrorCodeExecutionFailed    = "execution_failed"
	ErrorCodeTimeout            = "timeout"
	ErrorCodeInvalidOutput      = "invalid_output"
	ErrorCodeExpectedNotMatched = "expected_not_matched"
)

type Request struct {
	Language             string
	Script               string
	Input                any
	Config               any
	Entry                string
	Timeout              time.Duration
	MaxScriptBytes       int
	MaxOutputBytes       int
	MaxOutputDepth       int
	MaxOutputArrayLength int
}

type Result struct {
	Language   string `json:"language"`
	Entry      string `json:"entry"`
	Value      any    `json:"value"`
	DurationMS int64  `json:"duration_ms"`
}

type ValidateRequest struct {
	Request         Request
	Expected        any
	CompareExpected bool
}

type ValidateResult struct {
	Result  Result `json:"result"`
	Matched bool   `json:"matched"`
}

type Error struct {
	Code    string
	Message string
}

func (e Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}
