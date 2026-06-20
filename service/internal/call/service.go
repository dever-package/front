package call

import (
	"fmt"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/server"
)

func Service(c *server.Context, serviceName string, payload any) (result any, err error) {
	return ServiceWithArgs(serviceName, c, []any{payload})
}

func ServiceWithArgs(serviceName string, args ...any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if recovered, ok := r.(error); ok {
				err = recovered
				return
			}
			err = fmt.Errorf("%v", r)
		}
	}()

	return load.Service(serviceName, args...), nil
}
