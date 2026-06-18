---
name: dever-front
description: Use when 修改 Dever front 核心组件，包括后台 runtime、page JSON 解析、标准 action、权限、上传、导入导出、站点配置、插件加载、SDK、缓存和升级影响。
version: 0.1.0
---

# Front 核心组件

本组件 skill 必须和 `shemic-dever` 一起使用。先遵守 Dever 框架规则，再按这里的 front runtime 边界修改。

普通业务 page JSON 开发不要以本 skill 为主；先按 `shemic-dever` 的 `references/front-page-quick.md` 和 `references/front-page.md` 写。只有维护 `package/front` runtime 本身时，才使用下面的约束。

## 事实来源

- 组件源码：`backend/package/front`
- 组件声明：`backend/package/front/dever.json`
- 后台页面：`front/page`
- 编译产物：`front/html`
- API：`api`
- Middleware：`middleware`
- Model：`model`
- 标准 page runtime：`service/page`
- 标准 action：`service/action`
- 权限：`service/permission`
- 上传：`service/upload`
- 导入导出：`service/importer`、`service/export`
- 站点配置和运行时：`service/site`、`service/siteconfig`
- Runtime 缓存：`service/runtimecache`
- 插件 SDK：`sdk`

## 硬规则

- `package/front` 只承载通用 runtime 能力，不放业务组件私有逻辑。
- 不手改 `front/html` 及其 `assets`；主前端源码构建后才更新这里。
- 不绕过 page/model/action registry 直连任意表、字段或 SQL。
- 标准 action 必须经过站点、登录态、权限、字段白名单和 model 元数据校验。
- page JSON 自动推导、Options、Relations、partial save、权限上下文不能为单个页面特殊分支而破坏。
- 上传、导入、导出必须保留大小、类型、路径、权限、任务状态和错误脱敏边界。
- 公开 route 只放 `dever.json.front.public` 或站点 `public` 中明确允许的路径。
- runtime/cache 变更必须有统一失效路径；写操作成功后不能只依赖 TTL。
- package/module front 插件加载走 `service/site` 和 Dever CLI 编译器，不在业务组件里复制插件静态服务。
- 站点运行契约属于组件 `dever.json.front.sites`；项目 `config/front.json` 只覆盖展示配置。

## 允许改动的场景

- 新增通用 page node、action 或数据解析能力。
- 修复权限、站点配置、上传、导入导出、缓存、插件加载等 runtime 问题。
- 扩展后台账号、角色、菜单、权限等 front 自有 model 的真实行为。
- 维护插件 SDK 的公开 API，并保持组件插件不直接依赖主 front 源码。

普通业务页面、业务校验、业务状态流转和业务前台交互不要放到 `package/front`。

## 常见检查

- 权限异常：先查 `service/permission`、page parent/auth、站点 access mode 和 action key 推导，不要放开通配权限。
- 保存异常：先查 `service/action` 的字段过滤、`action.submit.data`、`_partial` 和 update 页上下文。
- option 异常：先查 `service/page` model 推导、Options、Relations 和嵌入页上下文。
- 插件未加载：先查 `service/site`、插件 manifest/source 发现、页面 node type 和 dev proxy。
- 站点配置异常：先查 `dever.json.front.sites` 与 `config/front.json` 的职责边界。
