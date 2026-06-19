package upload

import (
	"context"
	"sync"
	"time"

	dlog "github.com/shemic/dever/log"
	"github.com/shemic/dever/util"

	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

const (
	uploadSessionCleanupInterval = time.Hour
	uploadSessionCleanupBatch    = 100
)

var uploadSessionCleanupRuntime struct {
	once   sync.Once
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func StartSessionCleanup() {
	uploadSessionCleanupRuntime.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		uploadSessionCleanupRuntime.mu.Lock()
		uploadSessionCleanupRuntime.cancel = cancel
		uploadSessionCleanupRuntime.done = done
		uploadSessionCleanupRuntime.mu.Unlock()

		go func() {
			defer close(done)
			runSessionCleanupLoop(ctx)
		}()
	})
}

func StopSessionCleanup(ctx context.Context) error {
	ctx = normalizeSessionCleanupStopContext(ctx)
	uploadSessionCleanupRuntime.mu.Lock()
	cancel := uploadSessionCleanupRuntime.cancel
	done := uploadSessionCleanupRuntime.done
	uploadSessionCleanupRuntime.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return cleanupExpiredSessions(ctx, time.Now())
}

func runSessionCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(uploadSessionCleanupInterval)
	defer ticker.Stop()

	for {
		if _, err := CleanupExpiredSessions(ctx); err != nil {
			dlog.ErrorFields("front.upload.cleanup_expired_sessions_failed", "清理过期上传会话失败", dlog.Fields{
				"error": err.Error(),
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func cleanupExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sessionModel, err := uploadrepo.ResolveSessionModel()
	if err != nil {
		return 0, err
	}

	rows := sessionModel.SelectMap(ctx, map[string]any{
		"expired_at": map[string]any{"lte": now},
	}, map[string]any{
		"field":    "main.id",
		"order":    "main.id asc",
		"page":     1,
		"pageSize": uploadSessionCleanupBatch,
	})

	var deleted int64
	for _, row := range rows {
		sessionID := util.ToUint64(row["id"])
		if sessionID == 0 {
			continue
		}
		if err := cleanupUploadSession(sessionID); err != nil {
			return deleted, err
		}
		deleted += sessionModel.Delete(ctx, map[string]any{"id": sessionID})
	}
	return deleted, nil
}

func normalizeSessionCleanupStopContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
