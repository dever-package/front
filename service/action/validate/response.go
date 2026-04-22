package validate

import (
	"strings"

	"github.com/shemic/dever/server"
)

func RespondError(c *server.Context, failures []Failure) error {
	fieldErrors := map[string]string{}
	for _, failure := range failures {
		if strings.TrimSpace(failure.Field) == "" || strings.TrimSpace(failure.Message) == "" {
			continue
		}
		if _, exists := fieldErrors[failure.Field]; !exists {
			fieldErrors[failure.Field] = failure.Message
		}
	}

	message := "表单校验失败"
	field := ""
	if len(failures) > 0 {
		if strings.TrimSpace(failures[0].Message) != "" {
			message = failures[0].Message
		}
		field = failures[0].Field
	}

	data := map[string]any{}
	if field != "" {
		data["field"] = field
	}
	if len(fieldErrors) > 0 {
		data["fieldErrors"] = fieldErrors
	}

	return c.JSONPayload(400, map[string]any{
		"code":   400,
		"status": 2,
		"msg":    message,
		"data":   data,
	})
}
