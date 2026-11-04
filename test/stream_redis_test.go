package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	frontstream "github.com/dever-package/front/service/stream"
	"github.com/redis/go-redis/v9"
)

func TestStreamUsesConfiguredRedisACL(t *testing.T) {
	binary := os.Getenv("REDIS_SERVER_BINARY")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("redis-server")
		if err != nil {
			t.Skip("redis-server is required for the Redis integration test")
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	acl := filepath.Join(dir, "users.acl")
	if err := os.WriteFile(acl, []byte("user default off\nuser streamtest on >stream-test-password ~tenant-test:* +@read +@write +@connection\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	process := exec.Command(binary, "--bind", "127.0.0.1", "--port", port, "--save", "", "--appendonly", "no", "--aclfile", acl, "--dir", dir)
	process.Stdout, process.Stderr = &logs, &logs
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	})
	client := redis.NewClient(&redis.Options{Addr: address, Username: "streamtest", Password: "stream-test-password"})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		if err := client.Ping(ctx).Err(); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("test Redis did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Chdir(dir)
	if err := os.Mkdir("config", 0700); err != nil {
		t.Fatal(err)
	}
	settings, err := json.Marshal(map[string]any{"redis": map[string]any{
		"enable": true, "addr": address, "username": "streamtest", "password": "stream-test-password", "prefix": "tenant-test",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("config/setting.jsonc", settings, 0600); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"TEAM", "STREAM", "ENERGON", ""} {
		for _, suffix := range []string{"REDIS_URL", "REDIS_ADDR"} {
			key := suffix
			if prefix != "" {
				key = prefix + "_" + suffix
			}
			t.Setenv(key, "")
		}
	}
	writer := frontstream.New("team")
	id, err := writer.WritePayload(ctx, "acl-check", map[string]any{"text": "stream output"})
	if err != nil {
		t.Fatalf("write using configured Redis: %v", err)
	}
	reader := frontstream.New("team")
	rows, err := reader.Read(ctx, "acl-check", "0-0", 10, -1)
	if err != nil {
		t.Fatalf("read using configured Redis: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != id || rows[0].Payload["text"] != "stream output" {
		t.Fatalf("stream round trip returned unexpected entries: %+v", rows)
	}
	if ttl := client.TTL(ctx, "tenant-test:team:stream:acl-check").Val(); ttl <= 0 {
		t.Fatalf("stream expiration was not set: %v", ttl)
	}
	if err := client.XLen(ctx, "team:stream:acl-check").Err(); err == nil || !strings.Contains(err.Error(), "NOPERM") {
		t.Fatalf("unprefixed stream must remain forbidden, got %v", err)
	}
	if err := os.Remove("config/setting.jsonc"); err != nil {
		t.Fatal(err)
	}
	_, err = frontstream.New("team").Read(ctx, "acl-check", "0-0", 1, -1)
	if err == nil || !strings.Contains(err.Error(), "读取 Stream Redis 配置失败") {
		t.Fatalf("missing configuration must report its cause, got %v", err)
	}
}
