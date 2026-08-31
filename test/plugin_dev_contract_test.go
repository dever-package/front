package integration_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/dever-package/front/service/siteconfig"
)

const (
	pluginDevNamesEnv   = "DEVER_FRONT_PLUGIN_DEV_NAMES"
	pluginDevVersionEnv = "DEVER_FRONT_PLUGIN_DEV_VERSION"
)

func TestPluginDevSourceNamesUseExplicitRunSelection(t *testing.T) {
	t.Setenv(pluginDevNamesEnv, " crm,bot,crm, ")

	names, explicit := siteconfig.PluginDevSourceNames()
	if !explicit {
		t.Fatal("plugin source selection should be explicit when dever run exports names")
	}
	if expected := []string{"bot", "crm"}; !reflect.DeepEqual(names, expected) {
		t.Fatalf("plugin source names = %v, want %v", names, expected)
	}
	if !siteconfig.PluginDevSourceAllowed("crm") {
		t.Fatal("selected source plugin should be allowed")
	}
	if siteconfig.PluginDevSourceAllowed("bot-remote") {
		t.Fatal("unselected source plugin should not be allowed")
	}
}

func TestPluginDevSourceNamesKeepManualModeFallback(t *testing.T) {
	unsetEnv(t, pluginDevNamesEnv)

	names, explicit := siteconfig.PluginDevSourceNames()
	if explicit || len(names) != 0 {
		t.Fatalf("manual plugin dev mode should not have an explicit source selection: names=%v explicit=%v", names, explicit)
	}
	if !siteconfig.PluginDevSourceAllowed("crm") {
		t.Fatal("manual plugin dev mode should retain source discovery behavior")
	}
}

func TestPluginDevVersionUsesRunSessionAndHasStableFallback(t *testing.T) {
	t.Setenv(pluginDevVersionEnv, "vite-session-1")
	if got := siteconfig.PluginDevVersion(); got != "vite-session-1" {
		t.Fatalf("plugin dev version = %q, want %q", got, "vite-session-1")
	}

	unsetEnv(t, pluginDevVersionEnv)
	first := siteconfig.PluginDevVersion()
	second := siteconfig.PluginDevVersion()
	if first == "" || first != second {
		t.Fatalf("plugin dev fallback version must be non-empty and process-stable: first=%q second=%q", first, second)
	}
}

func TestPluginAssetsConsumeRunSourceContract(t *testing.T) {
	source := readModuleFile(t, filepath.Join("service", "site", "plugin_assets.go"))
	required := []string{
		"siteconfig.PluginDevSourceNames()",
		"siteconfig.PluginDevSourceAllowed(pluginName)",
		"siteconfig.PluginDevVersion()",
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("plugin asset flow is missing %q", fragment)
		}
	}
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	value, exists := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(name, value)
			return
		}
		_ = os.Unsetenv(name)
	})
}

func readModuleFile(t *testing.T, relativePath string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Dir(filepath.Dir(currentFile))
	content, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}
