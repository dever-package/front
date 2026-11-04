package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestQiniuUploadRecovery(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(source))
	encoded, err := json.Marshal(map[string]any{"Replace": map[string]string{filepath.Join(root, "service/upload/provider/qiniu_upload_test.go"): filepath.Join(root, "test/qiniu_upload_test.go.fixture")}})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(t.TempDir(), "overlay.json")
	if err := os.WriteFile(overlay, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "-mod=readonly", "-overlay="+overlay, "github.com/dever-package/front/service/upload/provider", "-run", "^TestQiniuUploadRecovery$", "-count=1", "-v")
	command.Dir = filepath.Join(root, "../..")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	t.Log(string(output))
}
