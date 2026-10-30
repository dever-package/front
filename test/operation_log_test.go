package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dever-package/front/service/operationlog"
	"github.com/dever-package/front/service/siteconfig"
	"github.com/shemic/dever/server"
)

func TestOperationLogIdentityDomain(t *testing.T) {
	ctx := siteconfig.WithSite(context.Background(), siteconfig.Site{
		Key: "external", Access: siteconfig.Access{Mode: "login", AuthProvider: "external"},
	})
	t.Run("custom recorder precedes platform account resolution", func(t *testing.T) {
		var received []operationlog.Entry
		c := &server.Context{}
		c.SetContext(operationlog.WithRecorder(ctx, func(_ context.Context, entry operationlog.Entry) {
			received = append(received, entry)
		}))
		operationlog.Record(c, operationlog.Entry{Action: "upload", TargetID: "7"})
		operationlog.RecordForAccount(c, map[string]any{"id": 1}, operationlog.Entry{Action: "login"})
		if len(received) != 2 || received[0].Action != "upload" || received[0].TargetID != "7" || received[1].Action != "login" {
			t.Fatalf("custom recorder received %#v", received)
		}
	})
	t.Run("recorder failure propagates", func(t *testing.T) {
		failure := errors.New("audit persistence failed")
		c := &server.Context{}
		c.SetContext(operationlog.WithRecorder(ctx, func(context.Context, operationlog.Entry) { panic(failure) }))
		defer func() {
			err, ok := recover().(error)
			if !ok || !errors.Is(err, failure) {
				t.Fatalf("recorder failure was swallowed: %v", err)
			}
		}()
		operationlog.Record(c, operationlog.Entry{Action: "update"})
	})
}
