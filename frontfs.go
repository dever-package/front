package front

import "embed"

// ManifestFS 内嵌 front 组件声明。
//
//go:embed dever.json
var ManifestFS embed.FS

// PageFS 内嵌 front 模块自身的页面配置，避免运行时依赖 module/front 目录。
//
//go:embed page/*/*.json page/*/*/*.json page/*/*/*/*.json
var PageFS embed.FS

// SiteFS 内嵌后台前端静态产物，发布时由 front 的 build:backend 写入 html 目录。
//
//go:embed html
var SiteFS embed.FS
