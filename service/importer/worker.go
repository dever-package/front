package importer

import (
	"context"
	"sync"
	"time"
)

var importWorker struct {
	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

func Start() {
	importWorker.mu.Lock()
	defer importWorker.mu.Unlock()

	if importWorker.stop != nil {
		return
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	importWorker.stop = stop
	importWorker.done = done

	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			if err := processPendingImportTask(context.Background()); err != nil {
				time.Sleep(2 * time.Second)
			}
			select {
			case <-ticker.C:
			case <-stop:
				return
			}
		}
	}()
}

func Stop(ctx context.Context) error {
	importWorker.mu.Lock()
	stop := importWorker.stop
	done := importWorker.done
	if stop == nil {
		importWorker.mu.Unlock()
		return nil
	}
	importWorker.stop = nil
	importWorker.done = nil
	close(stop)
	importWorker.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func processPendingImportTask(ctx context.Context) error {
	task, ok, err := claimPendingTask(ctx)
	if err != nil || !ok {
		return err
	}

	if err := runImportTask(ctx, task); err != nil {
		finishTaskFailed(ctx, task.ID, err)
		return err
	}
	return nil
}

func runImportTask(ctx context.Context, task taskSnapshot) error {
	updateTaskProgress(ctx, task.ID, 3, "解析导入配置")
	config, err := resolveImportConfig(task.PagePath, task.ImportKey)
	if err != nil {
		return err
	}

	updateTaskProgress(ctx, task.ID, 5, "读取导入文件")
	summary, err := importWorkbookRows(ctx, task, config)
	if err != nil {
		return err
	}

	finishTaskSuccess(ctx, task.ID, summary)
	return nil
}
