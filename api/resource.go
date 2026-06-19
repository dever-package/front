package api

import (
	"github.com/shemic/dever/server"

	uploadservice "github.com/dever-package/front/service/upload"
)

type Resource struct{}

func (Resource) GetList(c *server.Context) error {
	return uploadservice.ListResources(c)
}

func (Resource) GetCategory(c *server.Context) error {
	return uploadservice.ListResourceCategories(c)
}

func (Resource) GetSource(c *server.Context) error {
	return uploadservice.ListResourceSources(c)
}

func (Resource) PostAssignCategory(c *server.Context) error {
	return uploadservice.AssignResourceCategory(c)
}
