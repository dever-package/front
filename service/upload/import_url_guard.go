package upload

import (
	"net/url"

	"github.com/dever-package/front/service/remoteurl"
)

var importURLHTTPClient = remoteurl.NewHTTPClient(remoteurl.ClientOptions{
	Timeout:      importURLTimeout,
	MaxRedirects: 5,
	ProxyEnvVars: []string{
		"FRONT_UPLOAD_IMPORT_URL_PROXY",
	},
})

func validateImportURL(parsed *url.URL) error {
	return remoteurl.Validate(parsed)
}
