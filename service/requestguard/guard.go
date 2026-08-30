package requestguard

import (
	"strings"
	"sync"

	"github.com/shemic/dever/server"
)

type Guard func(*server.Context) error

type registeredGuard struct {
	name  string
	guard Guard
}

var registry struct {
	sync.RWMutex
	guards []registeredGuard
}

func Register(name string, guard Guard) {
	name = strings.TrimSpace(name)
	if name == "" || guard == nil {
		panic("请求守卫名称和实现不能为空")
	}

	registry.Lock()
	defer registry.Unlock()
	for index := range registry.guards {
		if registry.guards[index].name == name {
			registry.guards[index].guard = guard
			return
		}
	}
	registry.guards = append(registry.guards, registeredGuard{name: name, guard: guard})
}

func Check(context *server.Context) error {
	registry.RLock()
	guards := append([]registeredGuard(nil), registry.guards...)
	registry.RUnlock()

	for _, current := range guards {
		if err := current.guard(context); err != nil {
			return err
		}
	}
	return nil
}
