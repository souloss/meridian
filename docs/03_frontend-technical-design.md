# Meridian 前端技术设计

> 状态：可实施基线（2026-09-04）
> 需求来源：[01_requirement-specification.md](./01_requirement-specification.md)
> 机器可读契约入口：[contracts/manifest.yaml](../contracts/manifest.yaml)
> 技术选型依据：[05_technology-stack-decision.md](./05_technology-stack-decision.md)
> 本文只定义信息架构、交互和前端实现策略。路径、DTO、枚举、权限和错误均引用 YAML，不另建副本。

## 1. 契约与范围

前端构建首先从 `contracts/openapi.yaml`（由 `contracts/api/` 下的 TypeSpec 唯一源编译生成）生成 TypeScript client 和 schema types；视图定义来自 `contracts/views.yaml`；角色和状态来自 `contracts/domain.yaml`；验收状态由 `contracts/acceptance.yaml` 决定。手写类型只能用于纯 UI 状态。

若设计稿文案与生成类型不一致，以契约为准并阻断 CI。禁止用 `as unknown as`、本地枚举或前端计算的 publish/approve 谓词掩盖契约漂移。

首发实现 `M0-M3`：控制面、仓库接入、OpenAPI Viewer、层/审批/AI、分支版本、diff/分享/todo、CLI 所需管理面和最小通知。M4-M5 的多 kind、分组全局视图、通用订阅渠道按 manifest 后续启用。

## 2. 技术栈与工程边界

- Nuxt 4 + Vue 3 + TypeScript 5.9，`ssr: false` 的纯 SPA 模式；`nuxt generate` 产出 `.output/public` 并由 Go embed；
- Nuxt file-based router：租户、资源和固定版本路由；`server/`、Nitro API、server middleware 和 SSR-only composable 禁止进入前端；
- Orval 8 从同一 OpenAPI 生成 models、Fetch request、TanStack Vue Query hooks/query keys 和 MSW handlers；不采用已转入维护模式的 `openapi-fetch`；
- TanStack Vue Query 5：所有 API server state、缓存、轮询、重试和失效；Pinia 3：仅客户端壳状态，不保存 API entity 或路由事实；
- Nuxt UI 4 + Tailwind CSS 4：应用壳、表单、弹层、Tree、Splitter、Table 与虚拟列表；不再并用 Naive UI、UnoCSS 或单独封装的 TanStack Table；
- Zod 4：静态 UI 表单；Ajv 8（JSON Schema 2020-12）：View options、仓库配置和 kind 动态 schema；两者不得手写复制 OpenAPI wire DTO；
- CodeMirror 6 + `@codemirror/merge`：YAML/JSON 编辑、只读源码和 side-by-side diff；
- Cytoscape.js 3：依赖关系分析图；直接封装生命周期，不引入 Vue wrapper；
- `@nuxtjs/i18n` 10：`zh-CN`/`en`，`no_prefix` 路由策略，翻译资源按 locale 懒加载；
- `@nuxt/icon` + 本地 `@iconify-json/lucide` 图标集，构建和运行时均不依赖公共 CDN；
- 视图插件：内建 Vue component 或受限 iframe；
- 测试：Vitest/Vue Test Utils、Mock Service Worker、Playwright。

选择 Nuxt 不是为了 SSR。Meridian 的价值页面全部依赖登录态、租户和实时 API 数据，SPA 与单二进制交付更匹配；Nuxt 提供约定式路由、布局、自动导入、代码分割和 Vite HMR，减少 B 端多模块装配成本。CI 必须执行 `nuxt generate`，确认所有深链经 Go fallback 可刷新，并拒绝运行时服务端依赖。

Orval custom fetcher 的职责仅限同源 base URL、`credentials: same-origin`、cookie 会话的 CSRF header、统一 ErrorResponse/requestId 和 401 处理。`If-Match`、`Idempotency-Key`、上传 body 和 operation 参数必须由生成签名显式传入，不能在 wrapper 中猜测。生成目录禁止手改，代码生成后必须 typecheck 且 git diff 为空。

建议目录：

```text
web/app/pages               Nuxt 路由页
web/app/layouts             admin/tenant/public/share 布局
web/app/middleware          auth/tenant 路由守卫
web/app/api/generated        Orval 生成 models/client/query/MSW，禁止手改
web/app/api/fetcher.ts       唯一手写 transport adapter
web/app/composables         API client/query/csrf/capability
web/app/stores              仅导航、drawer、未提交草稿等 UI 状态
web/app/features/admin      平台控制面
web/app/features/catalog    repository/service/source/group
web/app/features/assets     viewer/layers/revisions/versions
web/app/features/diff       compare/snapshot/share/todo
web/app/features/jobs       drawer/progress/retry
web/app/features/inbox      approval/minimal notifications
web/app/views               ViewHost/component/iframe bridge
web/app/components/ui       无业务语义的基础组件
```

## 3. 路由与访问边界

```text
/login
/admin/users
/admin/tenants
/admin/global-credentials
/admin/producer-profiles
/admin/audit
/t/:tenant/dashboard
/t/:tenant/repos
/t/:tenant/repos/new
/t/:tenant/repos/:repositoryId
/t/:tenant/services/:serviceSlug
/t/:tenant/assets/:assetId
/t/:tenant/assets/:assetId/versions/:versionId
/t/:tenant/reviews
/t/:tenant/todos
/t/:tenant/jobs
/t/:tenant/search
/t/:tenant/groups/:groupId
/share/:token
/public/t/:tenant/services/:serviceSlug/...
```

路由进入租户区域前读取 auth/me，并校验 active tenant membership。资源能力由 API 返回；`CapabilityGuard` 只控制展示，提交失败仍按服务端 404 处理。platform admin 没有租户成员身份时，不渲染租户业务路由。

平台控制面提供 global credential 与 producer profile 管理。secret 只在创建/轮换表单提交且绝不回填；producer profile 使用 executable、args 数组、环境变量白名单、网络模式和资源上限等结构化控件，不提供 shell 文本框。租户侧凭据下拉合并可读的本地凭据和 global 凭据，global 项只读标记；producer 下拉只展示 API 返回的 enabled/available 选项。

公开页和分享页使用独立 layout/client：不加载租户导航，不调用内部列表接口，不把 bearer/cookie 信息写入内容 URL。

## 4. 全局壳与状态

桌面端左侧栏包括目录、仓库、审批、待办、任务；顶栏包括租户切换、全局搜索、任务状态、通知和用户菜单。移动端改为抽屉导航；Viewer 工具栏横向滚动，不压缩固定按钮。

所有 query key 使用 Orval 生成的 key factory，并必须包含 tenant 和稳定资源标识；禁止在组件中手拼另一套 key。以下是语义形态，不是待手写 DTO：

```ts
['asset', tenant, assetId, refType, refName]
['version', tenant, versionId]
['layer-head', tenant, layerId, scopeType, scopeKey]
['job', tenant, jobId]
```

禁止用 Asset 级缓存承载分支 current/latest。切换 branch/tag 后必须换 Track query key。写成功后按响应中 affected resource 精确失效；任务完成后再刷新版本、source health 和 todo。

当前租户以 URL 的 `:tenant` 为唯一事实源，auth/me、membership、用户偏好和 capability 都属于 Query server state。Pinia 只保存移动导航展开、JobDrawer 当前选择、未提交的本地草稿等无需后端持久化的 UI 状态；主题由 Nuxt color mode 管理，locale 由 i18n 与用户偏好同步。刷新页面后必须能只凭 URL 和 API 恢复业务页面，不能依赖 Pinia 中残留的 tenant/entity。

所有错误通过生成的 ErrorResponse 处理：401 跳登录并保留 return URL；404 显示统一不可用状态；409 保留用户输入并提示刷新；412 提示资源已更新；413 标出容量限制；422 映射字段或编辑器 line/column；429 使用 `retryAfterSec` 倒计时且不自动重试写请求；503、网络错误和超时显示可重试状态并保留 requestId。Job SSE 断线按 `Last-Event-ID` 重连，终态关闭后停止轮询；只有用户主动取消才发送取消请求。

## 5. 核心用户流程

### 5.1 平台初始化

`/admin/users` 和 `/admin/tenants` 提供列表、创建、停用和成员绑定。创建租户后在同一流程绑定首位 tenant admin。平台审计页只展示脱敏元数据；任何正文链接都不出现。

空部署 bootstrap 的认证方式由运维配置提供，UI 不承担默认密码生成。首次完成后页面提示停用 bootstrap credential。

### 5.2 仓库接入向导

`/repos/new` 为三步同页向导：

1. 连接：URL、可空 credential、默认分支，调用 connection test；按 dns/auth/host_key/timeout 给出可操作错误；
2. 发现：保存后启动 discover job，展示稳定排序的候选；支持勾选批量接受和手工创建服务；若存在配置文件，先 preview 再 apply；
3. 资产源：为确认的服务填写 SourceSpec，mode 改变时切换 path/profile 等字段；保存并触发首轮 sync。

JobDrawer 展示阶段、attempt、结构化错误和 retry。接口返回 deduplicated 时继续观察原 jobId，不创建第二个假任务。

### 5.3 服务主页与缺失资产

服务页头显示 lifecycle、visibility、owners、标签、仓库/ref 与 capability 操作。正文包含：

- 资产列表：按 kind 分组，明确 current published、latest draft、ref、health 和待审数；
- 缺失 kind：展示“配置资产源”和“用 AI 生成”，后者调用服务级冷启动 operation；
- 资产源：SourceSpec 与展开 binding 分层展示，避免把 glob 误当单文件；
- 变更历史、成员、备注；后两项按里程碑和 capability 显示。

health 文案固定：stale 表示仍有历史有效版本但最新采集失败；invalid 表示从未成功；历史查看入口始终保留。

生命周期控件只显示服务端允许的下一状态并始终携带当前 ETag；deprecated 显示全局告警徽章，retired 页面切为历史只读并隐藏内容写入。public URL 只在 visibility=public 且 lifecycle 为 published/deprecated 时展示；生命周期冲突统一按 409 `invalid_state` 保留页面数据并刷新能力。

### 5.4 Viewer Shell

Viewer 路由固定 assetId/versionId；只在用户明确选择“跟随分支最新”时解析 Track head，并立即把解析后的 versionId 写入 URL。这样分享、刷新和 deep link 不随 latest 漂移。

Viewer 顶部包含 ref/version picker、lifecycle、分享和下载；View tabs 来自 views contract。切换时构造 InputDescriptor，调用 resolve，成功后才写 URL 和偏好。422 时保留旧视图并展示 descriptor 错误。

`source`、`operations`、`items-table`、`layers`、`diff` 是内建 Vue 组件；Swagger/Redoc 等第三方 renderer 在不含 `allow-same-origin` 的 opaque-origin iframe 中运行。父页先取签名 artifact，再以 document text 通过一次性 nonce 建立的 transferred MessagePort 传入；之后忽略 window message，frame 自身 `connect-src 'none'`。Try-it-out 默认关闭；启用时 iframe 通过该 port 请求父页面执行 tenant allowlist 内、`credentials: omit` 的浏览器 fetch，不经过平台后端代理，也不向 iframe 传 session。表格 columns 使用 ViewDef 独立字段，不能塞进 optionsSchema。

### 5.5 层与人工 Overlay

Layers view 左侧展示 Layer，右侧展示 scoped timeline 和 head：latest、candidate、effective 必须分别标注，不能以 `active` 文案代替“当前”。ref selector 默认仓库 default branch，global 继承要显式显示。

支持：

- 启停 Layer；
- 原子排序全部 overlay，提交 expected revision；
- 创建 manual SourceSpec（响应返回 `initialLayerId`）并提交 manual revision；
- 查看 revision 原文、元数据、review context 和 provenance；
- rollback 到允许的历史修订；
- merge preview。

编辑器左侧编辑，右侧 800ms 防抖 preview；保存携带 `expectedEffectiveRevisionId` 和 Idempotency-Key。422 的 1-based line/column 映射 CodeMirror。关闭、重开或 rollback 后出现新版本是正常行为；内容 hash 可以复用，versionId 不得被前端当成 hash。

交互编辑严格受 `manualOrPushRevisionBytes=1 MiB` 约束。超过 1 MiB 的版本只读展示；达到 5 MiB 时关闭全量语法树、折叠和自动格式化，仅渲染 viewport，YAML/JSON parse、搜索索引和格式化放入 Web Worker。大文档 diff 直接消费后端 DiffResult/hunks，不在主线程对两份全文做 diff。CodeMirror 及语言包只在 Viewer/source/diff 路由懒加载。

### 5.6 AI 冷启动与审批

缺失资产没有 assetId，因此从 Service 的 missingKinds 入口调用 service-level AI generate，选择平台允许的 profile/kind/name/ref，不展示 shell 输入。

任务完成后跳到审批中心。ReviewDrawer 展示 revision 内容、before/after、结构化 diff、影响项、producer 摘要和意见：

- approve/reject 只对当前 candidate 开放；
- reject 必填意见；
- approve 响应中的 mergeJobId 直接交给 JobDrawer；
- 409 candidate 已变化时关闭按钮、刷新上下文；
- 无有效 base 时 before 是契约定义的预览骨架，但审批前绝不宣称已有 AssetVersion。

信任模式 UI 明确写“免审批，不自动发布”。相同内容如果等于 current candidate/effective 可显示 unchanged；曾被 reject 的同内容再次生成仍是新的 review attempt。

### 5.7 Diff、快照与分享

Diff 输入选择器支持固定版本、branch/tag 和临时 upload。提交后响应中的 resolved version 和 baseline 写入 URL/页面，不保留漂移的 `latest` 别名。

展示 tree、side-by-side、list 三种模式，共用同一 DiffResult；等级枚举固定 `breaking|risky|non_breaking|informational`。Diff 保存快照后才能分享或导出。普通 Viewer 分享则提交 viewId、input descriptor、scope 和 options；服务端在创建时将 ref 与 scope 成员解析成固定 versionId 并冻结 artifact allowlist。分享页只能读取冻结 descriptor 绑定的结果，过期、撤销或替换资源都返回 404。

非默认分支首版的自动 diff 基线由后端解析为默认分支 current/latest。前端只展示 `baselineVersionId`，不自行猜测。breaking todo 在版本 index 后出现，用户只可 ack 自己的 todo。

### 5.8 GitOps 漂移

Repository 配置导入采用 preview/apply 两阶段。Preview 页面按字段展示 DB value、file value、source 和差异；apply 使用 previewId/configDigest。漂移解决的动作固定为 take_file、keep_db、ignore：

- take_file 显式接受 file 值；
- keep_db 令字段变为 db_manual；
- ignore 只忽略当前 file digest，下一次仓库变化重新提示。

不得在 Service 或 Source 顶层用单一 `configSource` badge 代替字段级来源。

### 5.9 搜索、分组、订阅与通知

搜索结果包括固定 versionId/itemKey 的 deep link、结构化高亮和 facets。前端不执行 HTML 高亮。SystemGroup 依赖图只呈现返回范围内的服务，覆盖率分母使用未删除成员服务。

依赖图固定使用 Cytoscape.js canvas renderer：`hierarchical` 映射内建 `breadthfirst`，`force` 映射内建 `cose`，首版不增加布局插件。默认请求最多 500 节点/2000 边；超过默认值必须先请求服务端 neighborhood，5000 边是单次硬上限而不是默认全量。边标签默认不渲染，交互时按需显示；1000 边以上启用 `hideEdgesOnViewport`、关闭动画并将 pixel ratio 固定为 1。实例、事件和 resize observer 必须在组件卸载时销毁。不得把 React Flow/Vue Flow 用作图分析引擎；它们面向可编辑 node-flow，和当前只读依赖分析不匹配。

订阅写操作对 viewer 合法；同样合法的还有通知已读和自己的 todo ack。因此只按 capability/operation 判定写权限，不建立“viewer 禁止全部 POST/PATCH”的规则。

breaking publish 展示两条独立通知：`version.published` 与 `version.breaking`。Webhook 配置只在创建/轮换时显示一次 secret，之后只显示指纹；轮换使用独立 rotate operation 和 ETag/If-Match，不能通过普通资料编辑静默替换。

## 6. ViewHost 与安全

`views.yaml` 决定每个 ViewDef 的 kind、输入 schema、renderer、order、columns 和 options。组件视图只接收解析后的 descriptor，不自行抓任意 URL。

iframe 使用独立静态入口、严格 CSP 和 sandbox。父页创建 MessageChannel，以 `postMessage('*', [port])` 仅传一次性 nonce 和 port；opaque-origin frame 必须在该 port 回显 nonce，父页随后只经 port 传 `documentText/mediaType/theme/options`，并忽略后续 window message。不得加 `allow-same-origin`，不得传文档 URL 或会话 token，也不提供平台代理。Try it out 默认关闭；用户开启后仅由父页浏览器直连租户 allowlist 内的 HTTPS origin（本地开发可 HTTP），每次新 origin 明确确认，credentials 固定 omit，目标必须自行允许 CORS。

下载由后端返回短时签名 URL；前端不把正文长期写入 localStorage、日志、analytics 或 error report。

## 7. 并发、乐观更新与可访问性

表单写操作使用 ETag/If-Match；Layer revision 使用 expected head。409/412 时显示 server/current 对比，不静默覆盖。排序可以先乐观移动，但必须保持原快照，失败立即回滚并可重试。

固定尺寸用于 toolbar、图标按钮、表格行和状态 badge，避免 job 文案或长 ref 引起布局跳动。图标按钮必须有 accessible name 和 tooltip；表格、树、dialog、drawer 支持键盘；颜色之外再用文字/图标区分状态。长 slug、branch、itemKey 可换行或省略并提供完整 tooltip。

Nuxt UI Table 不允许把“点击整行”作为唯一操作入口；查看、编辑和更多操作必须有可聚焦的 link/button，并通过键盘触发。虚拟列表仍需保留表头语义、焦点恢复和屏幕阅读器可理解的总数/当前位置。

## 8. 测试策略

- 生成 client 的 compile test：OpenAPI 更新后无手写调用漂移；
- 组件测试：CapabilityGuard、ErrorBoundary、VersionPicker、Layers heads、ReviewDrawer、Diff modes；
- MSW contract test：请求和响应均通过生成 schema，覆盖 401/404/409/412/413/422；
- Playwright：逐个执行 `acceptance.yaml` 的 US 和 Smoke，每场景独立 seed；
- 安全回归：public/share 页面不请求内部 API，PAT 撤销为 401，无权与不存在除 requestId 外一致；
- 响应式截图：桌面与移动端检查 Viewer、向导、审批、Diff，无溢出和重叠；
- 大数据：10k item 虚拟滚动、500 节点/5000 边依赖图首屏、1 MiB 交互编辑上限、5 MiB/10 MiB 只读文档主线程 long-task 门禁；
- 可访问性：axe + 键盘主路径，关键状态不只依赖颜色。

## 9. 实施顺序

| 里程碑 | 前端交付 |
| --- | --- |
| M0 | generated client、登录/CSRF、租户壳、控制面、错误与 capability 基础设施 |
| M1 | 仓库向导、服务页、source、job drawer、OpenAPI Viewer/public read |
| M2 | Layers、编辑/preview、revision timeline、rollback、字段级 GitOps |
| M3 | AI 冷启动/审批、Track/version picker、lifecycle、diff/snapshot/share、todo、最小 inbox |
| M4 | GitOps 完整漂移、search/group/dashboard、dbschema/dependency views |
| M5 | 通用订阅、通知渠道、asyncapi view、运维合规界面 |

完成定义不是页面存在，而是对应 `acceptance.yaml` 的用户故事、错误路径和权限反例通过，且生成代码无漂移。
