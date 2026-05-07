package upload

import (
	"github.com/shemic/dever/server"

	uploadresource "my/package/front/service/upload/resource"
)

func ListResources(c *server.Context) error {
	return uploadresource.HandleList(c)
}

func ListResourceCategories(c *server.Context) error {
	return uploadresource.HandleListCategories(c)
}

func ListResourceSources(c *server.Context) error {
	return uploadresource.HandleListSources(c)
}

func AssignResourceCategory(c *server.Context) error {
	return uploadresource.HandleAssignCategory(c)
}
