package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/component"
	"github.com/shemic/dever/server"
	_ "github.com/shemic/dever/server/http"

	frontsite "github.com/dever-package/front/service/site"
)

func TestUnboundAdminPathOnWorkHost(test *testing.T) {
	if os.Getenv("DEVER_SITE_PATH_FIXTURE") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestUnboundAdminPathOnWorkHost$", "-test.v")
		command.Env = append(os.Environ(), "DEVER_SITE_PATH_FIXTURE=1")
		if output, err := command.CombinedOutput(); err != nil {
			test.Fatalf("isolated site routes: %v\n%s", err, output)
		}
		return
	}
	root := test.TempDir()
	test.Chdir(root)
	if err := os.Mkdir(filepath.Join(root, "config"), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "setting.json"), []byte(`{}`), 0o600); err != nil {
		test.Fatal(err)
	}
	fixture := fstest.MapFS{
		"dever.json": {Data: []byte(`{
			"name":"path-test",
			"front":{"sites":{
				"admin":{"api":"front","page":"admin","access":{"mode":"rbac","authProvider":"front"}},
				"body":{"api":"bot/body","page":"body","config":{"url":["https://work.example.test"]},"access":{"mode":"login","authProvider":"user"}},
				"isolated":{"api":"isolated","page":"isolated","config":{"url":["https://isolated.example.test"]}}
			}}
		}`)},
	}
	component.Register(component.Definition{Name: "path-test", ManifestFS: fixture})
	app := fiber.New()
	frontsite.Register(&sitePathServer{app: app})
	cases := []struct {
		path   string
		status int
		site   string
		base   string
	}{
		{path: "/", status: 200, site: "body", base: "/"},
		{path: "/runtime.js", status: 200, site: "body", base: "/"},
		{path: "/admin", status: 200, site: "admin", base: "/admin"},
		{path: "/admin/sign-in", status: 200, site: "admin", base: "/admin"},
		{path: "/admin/runtime.js", status: 200, site: "admin", base: "/admin"},
		{path: "/body", status: 404},
		{path: "/isolated/runtime.js", status: 404},
	}
	for _, item := range cases {
		test.Run(item.path, func(test *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://work.example.test"+item.path, nil)
			response, err := app.Test(request)
			if err != nil {
				test.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != item.status {
				test.Fatalf("HTTP %d, want %d", response.StatusCode, item.status)
			}
			if item.status != http.StatusOK {
				return
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				test.Fatal(err)
			}
			for _, expected := range []string{`"siteKey":"` + item.site + `"`, `"basePath":"` + item.base + `"`} {
				if !strings.Contains(string(body), expected) {
					test.Fatalf("missing runtime field %s", expected)
				}
			}
		})
	}
}

type sitePathServer struct {
	server.Server
	app *fiber.App
}

func (adapter *sitePathServer) Get(path string, handler server.HandlerFunc) {
	adapter.app.Get(path, func(raw *fiber.Ctx) error {
		request := server.GetContext(raw)
		defer server.ReleaseContext(request)
		return handler(request)
	})
}
