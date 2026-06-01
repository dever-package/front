package permission

import (
	"sync"
	"sync/atomic"

	"my/package/front/service/siteconfig"
)

const (
	defaultRoleID      uint64 = 1
	defaultAccountID   uint64 = 1
	defaultRoleName           = "超级管理员"
	defaultAccountName        = "系统管理员"
)

type pageMeta struct {
	Name   string     `json:"name"`
	Title  string     `json:"title"`
	Icon   string     `json:"icon"`
	Parent string     `json:"parent"`
	Type   int        `json:"type"`
	Sort   int        `json:"sort"`
	Init   []string   `json:"init"`
	Auth   []authSeed `json:"auth"`
}

type configMeta struct {
	Auth  []authSeed `json:"auth"`
	Entry string     `json:"entry"`
}

type authSeed = siteconfig.AuthSeed

type authQuery map[string]string

type authRecord struct {
	Key       string
	Name      string
	Icon      string
	Path      string
	ParentKey string
	Type      int
	Sort      int
	Query     authQuery
}

type authGraph struct {
	rows       []map[string]any
	rowByPath  map[string][]map[string]any
	rowByKey   map[string]map[string]any
	parentByID map[uint64]uint64
	allIDs     map[uint64]struct{}
}

type accessSnapshot struct {
	auth    authGraph
	roleIDs []uint64
	allowed map[uint64]struct{}
}

var (
	bootstrapState struct {
		mu   sync.Mutex
		done atomic.Bool
	}
)
