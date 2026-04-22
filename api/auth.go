package api

import (
	"github.com/shemic/dever/server"

	frontservice "github.com/dever-package/front/service"
)

type Auth struct{}

func (Auth) PostLogin(c *server.Context) error {
	return frontservice.Login(c)
}
