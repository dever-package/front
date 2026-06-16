package upload

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultImportURLConcurrency = 4

var importURLSlots = make(chan struct{}, importURLConcurrency())

func acquireImportURLSlot() (func(), error) {
	select {
	case importURLSlots <- struct{}{}:
		return func() {
			<-importURLSlots
		}, nil
	default:
		return nil, fmt.Errorf("远程资源导入任务繁忙，请稍后再试")
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
	if n > 64 {
		return 64
	}
	return n
}
