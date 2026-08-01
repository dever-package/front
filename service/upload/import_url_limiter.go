package upload

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultImportURLConcurrency = 64
	maxImportURLConcurrency     = 64
)

var importURLSlots = make(chan struct{}, importURLConcurrency())

func acquireImportURLSlot(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case importURLSlots <- struct{}{}:
		return func() {
			<-importURLSlots
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("等待远程资源导入任务失败: %w", ctx.Err())
	}
}

func importURLConcurrency() int {
	value := strings.TrimSpace(os.Getenv("FRONT_UPLOAD_IMPORT_URL_CONCURRENCY"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("UPLOAD_IMPORT_URL_CONCURRENCY"))
	}
	if value == "" {
		return defaultImportURLConcurrency
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return defaultImportURLConcurrency
	}
	if n > maxImportURLConcurrency {
		return maxImportURLConcurrency
	}
	return n
}
