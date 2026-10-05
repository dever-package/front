package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/component"
	"github.com/shemic/dever/config"
	"github.com/shemic/dever/server"

	frontpage "github.com/dever-package/front/service/page"
	frontsite "github.com/dever-package/front/service/site"
	"github.com/dever-package/front/service/siteconfig"
)

const hostPagePath = "/bot/body/statistics/list"

var registerHostPageFixtureOnce sync.Once

func TestHostPageNavigationServesMatchingAdminPage(t *testing.T) {
	registerHostPageFixture(t)

	response, apiCalled := requestHostPage(t, hostPageRequest{
		method: http.MethodGet,
		host:   "admin.example.test",
		accept: "text/html,application/xhtml+xml",
	})
	defer response.Body.Close()

	if apiCalled {
		t.Fatal("HTML page navigation reached the conflicting API handler")
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want HTML", contentType)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, "window.appRuntime") || !strings.Contains(content, `"siteKey":"admin"`) {
		t.Fatal("response is missing the admin SPA runtime")
	}
}

func TestHostPageNavigationPreservesAPIBoundaries(t *testing.T) {
	registerHostPageFixture(t)

	tests := []struct {
		name          string
		request       hostPageRequest
		wantAPICalled bool
	}{
		{
			name: "JSON request on admin host",
			request: hostPageRequest{
				method: http.MethodGet,
				host:   "admin.example.test",
				accept: "application/json",
			},
			wantAPICalled: true,
		},
		{
			name: "site-scoped request on admin host",
			request: hostPageRequest{
				method:     http.MethodGet,
				host:       "admin.example.test",
				accept:     "text/html",
				siteHeader: "admin",
			},
			wantAPICalled: true,
		},
		{
			name: "same path on body host",
			request: hostPageRequest{
				method: http.MethodGet,
				host:   "bot.example.test",
				accept: "text/html",
			},
			wantAPICalled: true,
		},
		{
			name: "unknown host",
			request: hostPageRequest{
				method: http.MethodGet,
				host:   "unknown.example.test",
				accept: "text/html",
			},
			wantAPICalled: true,
		},
		{
			name: "non-navigation method",
			request: hostPageRequest{
				method: http.MethodPost,
				host:   "admin.example.test",
				accept: "text/html",
			},
			wantAPICalled: true,
		},
		{
			name: "disabled static site",
			request: hostPageRequest{
				method:        http.MethodGet,
				host:          "admin.example.test",
				accept:        "text/html",
				disableStatic: true,
			},
			wantAPICalled: true,
		},
		{
			name: "forwarded admin host",
			request: hostPageRequest{
				method:        http.MethodGet,
				host:          "proxy.example.test",
				forwardedHost: "admin.example.test",
				accept:        "text/html",
			},
			wantAPICalled: false,
		},
	}

	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			response, apiCalled := requestHostPage(t, current.request)
			defer response.Body.Close()
			if apiCalled != current.wantAPICalled {
				t.Fatalf("API called = %v, want %v", apiCalled, current.wantAPICalled)
			}
		})
	}
}

func TestHostPageNavigationMiddlewarePrecedesRequestGuards(t *testing.T) {
	source := readModuleFile(t, filepath.Join("middleware", "init.go"))
	bootstrapIndex := strings.Index(source, "coremiddleware.UseGlobalFunc(frontBootstrap(settings))")
	navigationIndex := strings.Index(source, "coremiddleware.UseGlobal(hostPageNavigation(settings))")
	guardIndex := strings.Index(source, "coremiddleware.UseGlobalFunc(componentRequestGuards(settings))")
	if bootstrapIndex < 0 || navigationIndex < 0 || guardIndex < 0 {
		t.Fatal("host page navigation middleware chain is incomplete")
	}
	if bootstrapIndex > navigationIndex || navigationIndex > guardIndex {
		t.Fatal("host page navigation must run after bootstrap and before request guards")
	}
}

type hostPageRequest struct {
	method        string
	host          string
	forwardedHost string
	accept        string
	siteHeader    string
	disableStatic bool
}

func requestHostPage(t *testing.T, request hostPageRequest) (*http.Response, bool) {
	t.Helper()
	frontConfig := hostPageConfig()
	staticConfig := config.FrontSite{Dir: filepath.Join(t.TempDir(), "missing")}
	if request.disableStatic {
		enabled := false
		staticConfig.Enabled = &enabled
	}
	apiCalled := false
	app := fiber.New()
	app.All(hostPagePath, func(raw *fiber.Ctx) error {
		ctx := server.GetContext(raw)
		defer server.ReleaseContext(ctx)
		served, err := frontsite.TryOpenHostPageNavigation(ctx, frontConfig, staticConfig)
		if err != nil {
			return err
		}
		if served {
			return nil
		}
		apiCalled = true
		raw.Set("Content-Type", "application/json")
		return raw.SendString(`{"api":true}`)
	})

	httpRequest := httptest.NewRequest(request.method, "http://"+request.host+hostPagePath, nil)
	httpRequest.Host = request.host
	httpRequest.Header.Set("Accept", request.accept)
	if request.forwardedHost != "" {
		httpRequest.Header.Set("X-Forwarded-Host", request.forwardedHost)
	}
	if request.siteHeader != "" {
		httpRequest.Header.Set(siteconfig.RequestSiteHeader, request.siteHeader)
	}
	response, err := app.Test(httpRequest)
	if err != nil {
		t.Fatalf("request host page: %v", err)
	}
	return response, apiCalled
}

func registerHostPageFixture(t *testing.T) {
	t.Helper()
	registerHostPageFixtureOnce.Do(func() {
		fixture := fstest.MapFS{
			"dever.json": {
				Data: []byte(`{"name":"bot"}`),
			},
			"front/page/admin/body/statistics/list.json": {
				Data: []byte(`{"page":{"name":"Business data"}}`),
			},
		}
		component.Register(component.Definition{
			Name:         "bot",
			ManifestFS:   fixture,
			ManifestPath: "dever.json",
			PageFS:       fixture,
			PagePrefix:   "front/page",
		})
		frontpage.ClearContentCache()
	})
	if _, err := frontpage.ReadContentForPage("admin", "bot/body/statistics/list"); err != nil {
		t.Fatalf("load host page fixture: %v", err)
	}
}

func hostPageConfig() siteconfig.Config {
	return siteconfig.Config{Sites: []siteconfig.Site{
		{
			Key:  "admin",
			Page: "admin",
			API:  "front",
			Config: siteconfig.SiteConfig{
				URLs: []string{"https://admin.example.test"},
			},
		},
		{
			Key:   "body",
			Owner: "bot",
			Page:  "body",
			API:   "bot/body",
			Config: siteconfig.SiteConfig{
				URLs: []string{"https://bot.example.test"},
			},
		},
	}}
}
