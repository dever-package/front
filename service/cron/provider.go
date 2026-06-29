package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type ProviderHandler func(context.Context, map[string]any) (any, error)

var providerRegistry struct {
	sync.RWMutex
	items map[string]ProviderHandler
}

func RegisterProvider(name string, handler ProviderHandler) {
	name = strings.TrimSpace(name)
	if name == "" || handler == nil {
		return
	}

	providerRegistry.Lock()
	defer providerRegistry.Unlock()
	if providerRegistry.items == nil {
		providerRegistry.items = map[string]ProviderHandler{}
	}
	providerRegistry.items[name] = handler
}

func callRegisteredProvider(ctx context.Context, name string, payload map[string]any) (any, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, nil
	}

	providerRegistry.RLock()
	handler := providerRegistry.items[name]
	providerRegistry.RUnlock()
	if handler == nil {
		return nil, false, nil
	}

	result, err := handler(ctx, payload)
	if err != nil {
		return nil, true, err
	}
	if result == nil {
		return map[string]any{}, true, nil
	}
	return result, true, nil
}

func providerPanicError(recovered any) error {
	if recovered == nil {
		return nil
	}
	if err, ok := recovered.(error); ok {
		return err
	}
	return fmt.Errorf("%v", recovered)
}
