package cron

import (
	"context"
	"sync"

	dlog "github.com/shemic/dever/log"
)

type BootstrapFunc func(context.Context) error

var bootstrapRegistry struct {
	sync.RWMutex
	items []BootstrapFunc
}

func RegisterBootstrap(fn BootstrapFunc) {
	if fn == nil {
		return
	}
	bootstrapRegistry.Lock()
	defer bootstrapRegistry.Unlock()
	bootstrapRegistry.items = append(bootstrapRegistry.items, fn)
}

func runBootstraps(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	bootstrapRegistry.RLock()
	items := append([]BootstrapFunc(nil), bootstrapRegistry.items...)
	bootstrapRegistry.RUnlock()

	for _, fn := range items {
		if err := fn(ctx); err != nil {
			dlog.ErrorFields("front.cron.bootstrap_failed", "计划任务启动初始化失败", dlog.Fields{
				"error": err.Error(),
			})
		}
	}
}
