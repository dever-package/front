package action

import (
	"strings"

	actionvalidate "my/package/front/service/action/validate"
)

type FieldError struct {
	failures []actionvalidate.Failure
}

func NewFieldError(field string, message string) error {
	return NewFieldErrors(map[string]string{field: message})
}

func NewFieldErrors(fieldErrors map[string]string) error {
	failures := make([]actionvalidate.Failure, 0, len(fieldErrors))
	for field, message := range fieldErrors {
		field = strings.TrimSpace(field)
		message = strings.TrimSpace(message)
		if field == "" || message == "" {
			continue
		}
		failures = append(failures, actionvalidate.Failure{
			Field:   field,
			Message: message,
		})
	}
	return FieldError{failures: failures}
}

func (e FieldError) Error() string {
	if len(e.failures) == 0 {
		return "表单校验失败"
	}
	if message := strings.TrimSpace(e.failures[0].Message); message != "" {
		return message
	}
	return "表单校验失败"
}

func (e FieldError) FieldFailures() []actionvalidate.Failure {
	return append([]actionvalidate.Failure(nil), e.failures...)
}

func FieldFailures(err error) ([]actionvalidate.Failure, bool) {
	type carrier interface {
		FieldFailures() []actionvalidate.Failure
	}

	if err == nil {
		return nil, false
	}
	current, ok := err.(carrier)
	if !ok {
		return nil, false
	}
	failures := current.FieldFailures()
	return failures, len(failures) > 0
}
