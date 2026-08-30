package pagefilter

import (
	"strings"
	"sync"
)

type Filter func(componentName, pageName, routePath string) bool

type registeredFilter struct {
	name   string
	filter Filter
}

var registry struct {
	sync.RWMutex
	filters []registeredFilter
}

func Register(name string, filter Filter) {
	name = strings.TrimSpace(name)
	if name == "" || filter == nil {
		panic("页面过滤器名称和实现不能为空")
	}

	registry.Lock()
	defer registry.Unlock()
	for index := range registry.filters {
		if registry.filters[index].name == name {
			registry.filters[index].filter = filter
			return
		}
	}
	registry.filters = append(registry.filters, registeredFilter{name: name, filter: filter})
}

func Enabled(componentName, pageName, routePath string) bool {
	registry.RLock()
	filters := append([]registeredFilter(nil), registry.filters...)
	registry.RUnlock()

	for _, current := range filters {
		if !current.filter(componentName, pageName, routePath) {
			return false
		}
	}
	return true
}
