package page

import "github.com/shemic/dever/server"

type OptionRowsResolver func(c *server.Context, getInput func(string) string) ([]map[string]any, error)

var optionRowsResolver OptionRowsResolver

func RegisterOptionRowsResolver(resolver OptionRowsResolver) {
	optionRowsResolver = resolver
}

func resolveOptionRows(c *server.Context, getInput func(string) string) ([]map[string]any, error) {
	if optionRowsResolver == nil {
		return nil, nil
	}
	return optionRowsResolver(c, getInput)
}
