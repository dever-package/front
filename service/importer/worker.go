package importer

import (
	"context"
	"sync/atomic"
	"time"
)

var importWorkerStarted atomic.Bool

func init() {
	startImportWorker()
}

func startImportWorker() {
	if !importWorkerStarted.CompareAndSwap(false, true) {
		return
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			if err := processPendingImportTask(context.Background()); err != nil {
				time.Sleep(2 * time.Second)
			}
			<-ticker.C
		}
	}()
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
