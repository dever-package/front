package front

import "embed"

// PageFS 内嵌 front 模块自身的页面配置，避免运行时依赖 module/front 目录。
//
//go:embed page
var PageFS embed.FS
