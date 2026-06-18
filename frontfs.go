package front

import "embed"

// ManifestFS 内嵌 front 组件声明。
//
//go:embed dever.json
var ManifestFS embed.FS

// PageFS 内嵌 front 模块自身的页面、模板和模板站静态资源，避免运行时依赖 module/front 目录。
//
//go:embed front/page/*/*.json front/page/*/*/*.json front/page/*/*/*/*.json front/template front/assets
var PageFS embed.FS

// SiteFS 内嵌后台前端静态产物，发布时由 front 的 build:backend 写入 front/html 目录。
//
//go:embed front/html
var SiteFS embed.FS
