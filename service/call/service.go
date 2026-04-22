package call

import (
	"fmt"

	"github.com/shemic/dever/load"
	"github.com/shemic/dever/server"
)

func Service(c *server.Context, serviceName string, payload any) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	return load.Service(serviceName, c, []any{payload}), nil
}
