# Meridian · 前端技术设计（需求 01）

> 需求文档：[01_requirement-specification.md](./01_requirement-specification.md)
> 配套后端设计：[02_backend-technical-design.md](./02_backend-technical-design.md)（下称"后端 §N"）
> 代码基线：**全新项目（greenfield）**，无现状代码；本文档全部为目标设计。
> 文档状态：目标设计（页面尚未实现，不得据此推断已上线）

## 1. 目标与设计原则

### 1.1 业务目标

为资产目录提供多角色门户：目录浏览与检索、资产多视图查看（含层管理/合并预览/DIFF/全局视图）、AI 修订审批、仓库与租户管理。核心体验指标：录入仓库后 5 分钟看到首个文档；新增视图零侵入接入。

### 1.2 范围边界

- 本期范围：Nuxt 4 SPA 前端（打包产物交后端 `embed`）、视图插件框架、iframe 视图桥。
- 外部依赖（不在本期范围）：后端 API（以后端 §7 为契约）、Swagger UI / Redoc dist 资源包（后端托管）、视觉稿（本设计给出布局与组件级定义，视觉细节由 UI 走查补充）。

### 1.3 设计原则

- **视图是插件**：核心壳（Viewer Shell）只依赖 `ViewDef + input descriptor`，任何视图不进壳的代码。
- **URL 即状态**：租户、视图、版本选择、diff 选择、筛选条件全部可从 URL 还原（分享/回退/刷新一致）。
- **服务端权威**：前端不自行判定权限/可发布性，只消费后端返回的 `capabilities` 字段渲染操作项；错误码驱动交互（如 `asset_path_not_found` → 候选修正 UI）。
- **大文档不阻塞**：5MB+ 文档解析/高亮进 Web Worker；列表虚拟滚动。
- **失败默认可见**：所有 mutation 有 pending/错误态，绝不静默失败。

### 1.4 技术栈与关键选型

| 项 | 选型 | 说明 |
| --- | --- | --- |
| 框架 | Nuxt 4（SPA 模式，`ssr: false`） | 后端 embed 托管，无 SSR 运维负担 |
| UI 组件库 | Naive UI | 表格/树/表单完备，暗色主题原生支持 |
| 样式 | UnoCSS + 设计 token（CSS variables） | 亮/暗主题切换，viewer 主题跟随 |
| 状态 | Pinia | stores 见 §7 |
| 请求 | `ofetch` 封装 `useApi()` + TanStack Query (vue-query) | 缓存、重试、失效联动 |
| 图 | X6（dep-graph）+ mermaid 不使用（交互图需自绘） | 依赖图需拖拽/聚焦/展开 |
| 编辑器 | CodeMirror 6 | YAML/JSON 高亮、行级错误标注、只读 diff 模式 |
| Diff 文本视图 | CodeMirror merge view | side-by-side |
| 表格 | Naive UI DataTable + 虚拟滚动 | 条目表/端点表 |
| i18n | `@nuxtjs/i18n`，`zh-CN` 默认 + `en` | 文案全部抽 key |
| 类型 | 后端 OpenAPI 自描述 → `openapi-typescript` 生成 API 类型 | CI 校验漂移 |

## 2. 信息架构与路由表

URL 前缀规则：登录后除平台管理外全部挂在 `/t/{tenantSlug}` 下（后端 F1.5）。

| 路由 | 页面 | 权限 |
| --- | --- | --- |
| `/login` | 登录 | 匿名 |
| `/t/{t}` | 目录首页（Dashboard/Catalog） | viewer |
| `/t/{t}/search?q=` | 全局搜索结果 | viewer |
| `/t/{t}/repos` / `/repos/{id}` | 仓库列表 / 仓库详情（含候选服务、健康、同步历史） | viewer（写操作 maintainer） |
| `/t/{t}/repos/new` | 录入仓库向导 | maintainer |
| `/t/{t}/services/{slug}` | 服务主页 | viewer（受 visibility） |
| `/t/{t}/services/{slug}/assets/{kind}/{name}` | **资产查看器（Viewer Shell，核心页）**；query：`?view=&version=&branch=&opts=` | viewer |
| `/t/{t}/services/{slug}/assets/{kind}/{name}/diff?left=&right=` | diff 视图直达（Shell 的 versions 模式入口） | viewer |
| `/t/{t}/reviews` | 审批中心（AI/第三方修订待审队列） | `layer:approve` |
| `/t/{t}/todos` | 待办（breaking 确认） | 登录 |
| `/t/{t}/groups` / `/groups/{slug}` | 系统分组列表 / 分组详情（scope 视图入口：dep-graph、大盘） | viewer |
| `/t/{t}/jobs` / `/jobs/{id}` | 任务中心 / 任务详情（SSE 日志） | maintainer |
| `/t/{t}/notifications` | 站内信 | 登录 |
| `/t/{t}/settings/{tab}` | 租户设置：members / tokens / credentials / kinds / views / notify / audit | tenant_admin（credentials 对 maintainer 开放） |
| `/admin/{tab}` | 平台管理：tenants / users / jobs / audit | platform_admin |
| `/shared/{token}` | 分享链接落地页（无导航壳，只渲染目标视图） | 匿名（验签） |
| `/me/settings` | 个人偏好（默认视图、语言、主题） | 登录 |

### 2.1 全局布局

```
┌────────────────────────────────────────────────────────┐
│ TopBar: Logo | 租户切换器▾ | 全局搜索(⌘K) | 通知铃 | 主题 | 头像▾ │
├─────────┬──────────────────────────────────────────────┤
│ SideNav │  <NuxtPage>                                  │
│ 目录     │                                              │
│ 仓库     │                                              │
│ 系统分组 │                                              │
│ 审批中心 │  (badge: 待审数)                              │
│ 待办     │  (badge: breaking 未确认数)                   │
│ 任务中心 │                                              │
│ 设置     │                                              │
└─────────┴──────────────────────────────────────────────┘
```

- 租户切换器：下拉当前用户的租户列表（`GET /auth/me`），切换 = 路由跳转 `/t/{newSlug}` + 全部 query 缓存失效。
- ⌘K 全局搜索：弹层，输入即请求 `GET /search`（300ms 防抖），结果分组（条目/资产/服务/仓库），条目命中直接深链到 Viewer Shell 并定位条目。
- 权限渲染：路由中间件 `tenant-guard` 读 `me.tenants` 判断角色；页面内操作按钮由资源响应的 `capabilities: string[]` 控制显隐（后端权威）。

## 3. 设计系统

### 3.1 设计 token

| token 组 | 内容 |
| --- | --- |
| 色板 | `--color-primary`（品牌蓝）、语义色 success/warning/danger/info；**层来源色**：repo=灰蓝、manual=绿、ai_generated=紫、third_party=橙（层标注/徽章/provenance 全局一致） |
| 严重级色 | breaking=红、risky=橙、non-breaking=蓝、informational=灰（diff/徽章/统计卡全局一致） |
| 暗色主题 | CSS variables 双套；`useTheme()` 写 `<html data-theme>`；iframe 视图经 postMessage 下发主题（§6.3） |
| 排版 | 正文 14px；代码 `JetBrains Mono` 13px；页面标题 20px |
| 间距/圆角 | 4px 基线网格；卡片圆角 8px |

### 3.2 通用组件清单

| 组件 | 用途 |
| --- | --- |
| `LifecycleBadge` | draft/published/deprecated/retired 徽章 |
| `OriginBadge` | 层来源徽章（含色点 + 文案） |
| `RevisionStatusTag` | active/pending_review/approved/rejected/archived |
| `KindIcon` | 每 AssetKind 一个图标（registry 提供，缺省通用图标） |
| `VersionPicker` | 版本/分支选择器：下拉含"已发布版本列表 + 分支最新 + 输入版本号"，diff 场景可多选 2 |
| `DocSelectorModal` | diff/collection 的文档选择器：级联 服务→资产→版本/分支，或本地上传 |
| `EmptyState` | 空态 + 主行动按钮（如"资产缺失 → 用 AI 生成"） |
| `CodeBlock` / `YamlEditor` | CM6 只读/可编辑封装，支持行级错误、来源层行高亮 |
| `ConfirmModal` | 破坏性操作二次确认（删除、驳回、retire） |
| `CandidateFixCard` | `asset_path_not_found` 候选文件列表 + 一键改路径 |
| `SseLogViewer` | 任务日志流式滚动 + 阶段分组折叠 |
| `ShareButton` | 生成分享链接弹层（过期时间选择 → 复制 URL） |

## 4. 页面设计（逐页）

### 4.1 目录首页 `/t/{t}`

```
┌ 统计条: 服务数 | 资产数 | 待审 N | breaking 未确认 N | 覆盖率(有 openapi 的服务占比) ┐
├ Tab: 全部 | 我负责的 | 我收藏的 | 最近更新 | 待我处理                             ┤
├ 筛选条: 分组▾ 标签▾ 生命周期▾ 资产类型▾ [卡片/列表切换]                          ┤
│ ServiceCard*N: 名称+徽章 | 资产 kind 图标行 | 质量分 | 最近变更时间 | Star        │
└ 分页                                                                            ┘
```

- 数据：`GET /services`（分面参数透传 URL query）；统计条 `GET /views/resolve`（scope=tenant 的 catalog-dashboard 查询，复用后端聚合）。
- "待我处理" tab = `/breaking-todos?status=open` + `/reviews` 数量合并展示，点击跳对应中心页。

### 4.2 录入仓库向导 `/t/{t}/repos/new`（3 步）

1. **连接**：URL + 凭证选择（下拉已有凭证 + "新建凭证"内联抽屉）+ 默认分支；[测试连接] 按钮调 `POST /credentials/{id}/test`，错误分类文案化（dns/auth/host_key/timeout 各自给修复建议）。
2. **发现**：保存仓库后触发 `POST /repositories/{id}/discover`，轮询 job 完成 → 展示候选服务清单（checkbox 表格：目录、探测依据、语言），支持手工添加行；勾选确认调 `candidates:accept`。若存在 `.asset-platform.yaml` 则展示"从配置文件导入"预览面板（服务/资产源树 + 漂移标记），确认后 `import-config?apply=true`。
3. **资产源**：对每个已确认服务展示推断出的资产源配置表单（kind/path 预填探测结果），保存后触发首次 `sync`，页面跳仓库详情并打开任务日志抽屉。

### 4.3 仓库详情 `/t/{t}/repos/{id}`

- 头部：URL、分支、凭证指纹、健康状态（lastSync/failStreak/duration）、[立即同步] [发现服务] 按钮。
- Tab：**服务列表**（含漂移提示行内 banner：`配置漂移 → [对比] [采纳] [忽略]`）/ **同步历史**（collection_jobs 表格，行点击开日志抽屉）/ **设置**（分支策略、拉取策略、webhook 地址与 secret 展示、sync_cron）。

### 4.4 服务主页 `/t/{t}/services/{slug}`

```
┌ 头部: 名称+LifecycleBadge | owners | 标签 | Star | 订阅▾ | [设置]              ┐
├ 概览卡: 仓库/分支/根目录 | 质量分 | 最近版本 | 健康                             ┤
├ 资产列表(按 kind 分组):                                                        │
│   openapi ▸ admin-api   [v1.4.0 published] [层: R M A] [查看] [diff]           │
│   dbschema ▸ main       [资产缺失? EmptyState → 用 AI 生成 / 配置资产源]        │
├ Tab: 变更历史(版本时间线,breaking 红点) | 资产源配置 | 成员 | 备注              ┤
```

- 层指示器 `[R M A]`：以来源色点表示该资产有 repo/manual/ai 层，hover 显示层数与待审数。
- **资产源配置 tab**（maintainer）：表格 = 一行一个 source（kind/name/role/origin/mode/path/enabled/最后错误）；`last_error.code=asset_path_not_found` 的行内嵌 `CandidateFixCard`；新增源用抽屉表单，**mode 联动校验**（builtin⇒path 必填；command/ai⇒command 必填），role=base 且已存在 base 时禁用并提示。

### 4.5 资产查看器 Viewer Shell（核心页）

```
┌ Crumb: 服务 / kind:name        VersionPicker▾   [发布状态Badge] [ShareButton] ┐
├ ViewTab 条: Swagger UI | Redoc | 源码 | 层 | 端点 | Diff | …(按 ViewDef.order) ┤
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │                     <ViewHost>  (component / iframe)                     │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
└ 底部状态条: 版本 v1.4.0 · commit abc123 · 引擎 v1 · 质量分 87 · 下载▾          ┘
```

- 流程：路由 query（`view/version/branch/opts`）→ 组装 input descriptor → `POST /views/resolve` → 结果注入目标视图。descriptor 校验失败（422）时回退默认视图并 toast。
- ViewTab 条来源 `GET /views` 过滤（kind 匹配 + enabled），顺序按 `order`；用户切换视图写回 URL 并记忆偏好（`/me/settings`）。
- 下载菜单：merged / bundled / 各层原始 / provenance JSON（后端签名 URL 直下）。

#### 4.5.1 `layers` 视图（自研 component，P0 重点）

```
┌ 左栏(层列表, 可拖拽排序 overlay):                    ┬ 右栏:                    ┐
│ ▣ base   repo    docs/openapi.yaml   [rev abc·active]│ 选中层的修订时间线:      │
│ ▣ ovl#1  manual  补充描述           [rev def·active] │  rev def 2026-09-01 由张三│
│ ▢ ovl#2  ai      AI补全            [rev ghi·待审🟣] │  [查看内容][设为当前(回滚)]│
│ [＋новый层/源]                                       │  [与上一修订对比]         │
├ 底部: 合并预览开关 → 并排显示 合并前(base)/合并后 diff，高亮各 overlay 命中处    ┤
```

- 操作与 API 对应：启停 checkbox=`PATCH /layers/{id}`；拖拽排序=`PATCH ord`（乐观更新+失败回滚）；回滚=`POST /layers/{id}:rollback`（ConfirmModal 说明"将产生新版本"）。
- **manual 层编辑**：`[编辑]` 打开全屏 YamlEditor 双栏——左编辑 overlay 内容，右实时合并预览（`POST /assets/preview-merge`，800ms 防抖，1MB 内起用）；保存 = `POST /layers/{id}/revisions`，`overlay_invalid` 行级错误直接标注在编辑器行号上。
- 待审修订（🟣）行内 `[审批]` 快捷入口 → 审批抽屉（见 4.7，复用同一组件）。

#### 4.5.2 `source` 视图

- CM6 只读 + 折叠；顶部开关 **[层标注]**：开启后按 provenance 给行背景着层来源色，hover 行显示 `来自: manual 层 · rev def`（provenance ptr → 行号映射在 Worker 中用 YAML source map 计算）。

#### 4.5.3 `operations` / `items-table` 视图

- DataTable 虚拟滚动；operations 列：method(色标)/path/summary/tag/废弃标记；items-table 列由 `display` JSONB 的 kind 声明列配置驱动（`GET /views` 返回的 `optionsSchema` 内含列定义），两视图共用一个表格组件不同列配置。
- 行点击开右侧 Detail 抽屉（该条目 JSON + provenance 徽章）。

#### 4.5.4 `diff` 视图

```
┌ 选择条: [左: v1.3.0 ▾]  ⇄  [右: release/2.0 最新 ▾]   规则集▾  [保存快照]     ┐
├ 统计卡: +12 新增 | -3 删除 | 9 修改 | 2 breaking(红)                          ┤
├ 呈现切换: ◉树  ○并排  ○清单     级别筛选: ☑breaking ☑risky ☐info             ┤
│ tree: 层级变更树(Path→Operation→字段)，节点带级别色点，点击展开 before/after    │
│ side-by-side: CM6 merge view                                                 │
│ list: 条目级表格(itemKey/type/level/byLayer)                                  │
└ [导出 Markdown] [导出 JSON] [分享]                                            ┘
```

- 左右选择器 = `VersionPicker`（多态：版本/分支/本地上传→`POST /uploads`）；变更即改 URL query 并重发 `POST /diff`。
- `byLayer` 存在时（层感知 diff，P1）清单模式提供"按层分组"切换。

#### 4.5.5 iframe 视图（swagger-ui / redoc / rapidoc）

- `ViewHost` 渲染 `<iframe src="/viewer-assets/{viewId}/index.html">`（后端托管 dist），通过 postMessage 桥（§6.3）传 `{docUrl, theme, options}`；Try it out 请求由 iframe 直接发起（同源，携带会话）。

### 4.6 系统分组与全局视图 `/t/{t}/groups/{slug}`

- 头部：分组树面包屑（域→系统）+ 成员服务 chips（可增删，`systemgroup:manage`）。
- Tab：
  - **依赖图（dep-graph）**：X6 画布；节点=服务（按分组着色，双击进服务主页），边=dependency 条目（hover 显示协议/中间件）；工具条：布局切换（力导/层次）、聚焦模式（只显选中节点 1 度邻居）、导出 PNG。数据 = `POST /views/resolve`（scope descriptor）分页拉全量边后前端建图；>2000 边时提示切换聚焦模式。
  - **资产大盘（catalog-dashboard）**：统计卡（服务/资产/条目数、质量分分布直方图、覆盖率环图：无 openapi 的服务列表可下钻）。
  - **成员服务列表**。

### 4.7 审批中心 `/t/{t}/reviews`

```
┌ 筛选: 状态(待审/已审) | 来源(ai/third_party) | 服务▾                          ┐
│ 列表行: [🟣ai] order-service / openapi:admin-api  rev ghi  2026-09-04  [审批] │
├ 审批抽屉(点击行展开, 占屏 80%):                                               │
│  上: 元信息(模型/promptDigest/token 用量/任务链接)                             │
│  中: Tab[修订内容 | 合并前后对比(preview-merge diff) | 影响条目]               │
│  下: [✓通过]  [✗驳回(意见必填)]  [重新生成]                                   │
```

- 通过=`:approve`（成功后 toast"已触发合并"并列表移除）；驳回=`:reject`（comment 必填校验）；重新生成=`POST /assets/{id}/ai-generate`（确认弹层提示将产生新修订）。
- 侧栏 badge 数量 = 待审列表 total，60s 轮询 + 站内信事件即时刷新。

### 4.8 待办中心 `/t/{t}/todos`

- breaking 确认列表：行=资产版本 + breaking 摘要（红色统计）+ [查看 diff]（深链 diff 视图）+ [确认知悉]（`:ack`，可填备注）。已确认历史 tab。

### 4.9 任务中心 `/t/{t}/jobs`

- 表格：类型/仓库/触发方式/阶段进度条（六段 stage 点亮）/状态/耗时；筛选类型与状态。
- 详情 `/jobs/{id}`：阶段时间线 + `SseLogViewer`（`GET /jobs/{id}/logs` SSE，断线自动降级 3s 轮询）；[取消] 按钮（running 时）。

### 4.10 设置页 `/t/{t}/settings/{tab}`

| tab | 内容要点 |
| --- | --- |
| members | 成员表格 + 邀请（用户名搜索）+ 角色下拉；移除最后 admin 时按后端 `last_admin` 错误提示 |
| tokens | PAT 列表（掩码+最后使用）；创建弹层选 scopes/过期 → **明文仅展示一次**（复制按钮 + 关闭确认） |
| credentials | 凭证卡片（指纹/类型/共享范围）；创建抽屉按 kind 联动表单；删除遇 `credential_in_use` 展示引用仓库并提供强制解绑确认 |
| kinds | AssetKind 开关列表（停用二次确认："存量数据只读不删"） |
| views | ViewDef 表格：启停/拖拽排序/默认参数（optionsSchema 动态渲染表单） |
| notify | 渠道配置：站内(默认开)/webhook(URL+secret+测试发送)/邮件 provider |
| audit | 审计表格：时间/操作者/action/target + 过滤器 |

### 4.11 分享落地页 `/shared/{token}`

- 无侧栏/顶栏壳，仅 logo 水印 + 目标视图全屏渲染；`GET /shared/{token}` 404 时展示"链接已失效"空态。只读：隐藏一切操作按钮（编辑/审批/下载可配）。

## 5. 视图插件框架（前端侧）

### 5.1 注册机制

```ts
// app/views/registry.ts
export interface FrontViewPlugin {
  id: string                              // 与后端 ViewDef.id 一致
  component?: Component                   // mount=component 时
  // mount=iframe 时无需前端代码，Shell 直接 iframe 化
}
// 新增 component 视图 = 在 views/ 下加一个目录并在 registry 注册一行
```

- Shell 渲染逻辑：`ViewDef.mount==='component'` → 查 registry 取组件动态加载（`defineAsyncComponent`，视图代码分包）；`'iframe'` → 通用 IframeHost；`'external'` → 新窗口打开。后端有 ViewDef 而前端无注册 ⇒ tab 隐藏并 console.warn（向前兼容）。

### 5.2 视图组件统一 Props 契约

```ts
interface ViewProps {
  docs?: DocRef[]            // single/versions/collection 解析结果
  itemQuery?: ItemQueryPage  // scope 解析结果（含 fetchMore 回调）
  descriptor: InputDescriptor
  options: Record<string, unknown>   // 经 optionsSchema 校验的参数
  theme: 'light' | 'dark'
}
// 视图对外事件: emit('update:options'), emit('navigate', deepLink)
```

`options` 变化由 Shell 序列化进 URL `?opts=`（base64url JSON）并纳入分享链接。

### 5.3 iframe 桥协议

| 方向 | message | payload |
| --- | --- | --- |
| Shell→iframe | `init` | `{docUrl, theme, options}` |
| Shell→iframe | `theme` | `{theme}`（主题切换实时下发） |
| iframe→Shell | `ready` / `resize` | `—` / `{height}` |

iframe 沙箱：`sandbox="allow-scripts allow-same-origin allow-forms"`；仅接受同源 dist。

## 6. 状态管理与 API 层

### 6.1 Pinia stores

| store | 职责 |
| --- | --- |
| `useAuthStore` | me、当前租户、角色；租户切换动作（跳转+失效全部 query） |
| `useViewPrefStore` | 用户级/服务级默认视图（localStorage + `/me/settings` 同步） |
| `useNotifyStore` | 站内信未读数、审批/待办 badge（事件驱动刷新） |
| `useThemeStore` | 主题 + iframe 广播 |

其余数据一律走 TanStack Query（key 规范：`[tenant, resource, id, params]`；租户切换时 `queryClient.clear()`）。

### 6.2 API 封装与错误处理

- `useApi()`：ofetch 实例，自动拼 `/api/v1/t/{tenant}` 前缀、CSRF 头；401 → 跳登录；404 统一"不存在或无权限"文案（不区分，配合后端防泄漏口径）。
- 错误码→交互映射表（集中维护 `errorMap.ts`）：`asset_path_not_found`→CandidateFixCard、`overlay_invalid`→编辑器行标注、`version_not_publishable`→阻塞修订列表弹层、`base_layer_exists`→表单项禁用提示、`quota_exceeded`→配额说明弹层、`branch_not_indexed`→"先同步该分支"引导。

## 7. 工程结构

```
web/
  app/
    layouts/ (default.vue, blank.vue)
    pages/ (按 §2 路由)
    views/ (registry.ts + layers/ source/ operations/ items-table/ diff/
            dep-graph/ dashboard/ collection-table/ changelog/)
    components/ (§3.2 通用组件)
    stores/  composables/ (useApi, useTheme, useSse, useWorkerYaml)
    workers/ (yaml-sourcemap.worker.ts, highlight.worker.ts)
    locales/ (zh-CN.json, en.json)
  types/api.d.ts (openapi-typescript 生成)
```

### 7.1 实施顺序（对齐后端里程碑）

| 阶段 | 交付 |
| --- | --- |
| M0 | 布局壳、登录、租户切换、设置页（members/tokens/credentials）、路由守卫、API 层与类型生成流水线 |
| M1 | 录入仓库向导、仓库详情、服务主页、Viewer Shell + swagger-ui/redoc(iframe)/source/operations/items-table、目录首页 |
| M2 | layers 视图（含 manual 编辑 + 实时合并预览）、source 层标注、资产源配置 tab |
| M3 | diff 视图三形态、审批中心、AI 生成入口与空态、任务中心 SSE |
| M4 | 系统分组、dep-graph、catalog-dashboard、全局搜索 ⌘K、collection-table |
| M5 | 订阅/通知/待办中心、分享落地页、设置页剩余 tab（kinds/views/notify/audit） |

## 8. 测试与验收

| 场景 | 条件 | 预期 |
| --- | --- | --- |
| 租户切换 | 从 A 切到 B | URL 前缀变化、全部列表刷新、无 A 租户残留数据（query 全失效） |
| URL 还原 | 刷新 diff 页（含 left/right/opts） | 完整还原选择与视图状态 |
| 大文档 | 5MB openapi 打开 source/operations | 主线程无 >200ms 长任务（Worker 化验证） |
| 合并预览 | 编辑 overlay 连续输入 | 防抖后仅尾部请求；`overlay_invalid` 标注正确行 |
| 审批流 | 通过/驳回/驳回缺 comment | 列表联动、badge 更新、缺 comment 阻断提交 |
| 明文 token | 创建 PAT 后关闭弹层再打开 | 不再可见明文 |
| iframe 主题 | 切暗色 | swagger-ui/redoc 实时跟随 |
| 分享链接 | 匿名打开/过期打开 | 只读渲染无操作按钮 / 失效空态 |
| i18n | 切 en | 无中文残留（CI key 覆盖检查） |
| 无障碍基线 | 键盘遍历主流程 | 焦点可达、modal 焦点圈闭 |

## 9. 发布与待确认项

- 构建：`nuxt generate`（SPA）产物进后端 `embed`，与后端同版本号发布；无独立回滚（随后端镜像回退）。
- 浏览器基线:最近两个大版本 Chrome/Edge/Safari/Firefox；不支持 IE。

| 编号 | 级别 | 待确认项 | 未确认影响 |
| --- | --- | --- | --- |
| F-C1 | P0 | items-table 列配置的下发格式（随 `GET /views` 的 optionsSchema，需与后端 kind 插件对齐字段） | 阻塞 M1 items-table 通用化 |
| F-C2 | P1 | dep-graph 超大图（>2000 边）交互策略（聚焦模式默认 or 服务端子图查询） | 影响 M4 大盘性能 |
| F-C3 | P1 | provenance ptr→YAML 行号映射精度（多行标量/锚点场景） | 层标注可能降级为块级高亮 |
| F-C4 | P2 | Try it out 的跨域目标服务器代理策略 | v1 仅同源/公网直连，失败给出 CORS 提示 |

## 10. 专业名词表

| 统一名称 | 英文/代码标识 | 一句话口径 |
| --- | --- | --- |
| 查看器壳 | Viewer Shell | 资产页核心容器：解析 descriptor → resolve → 挂载视图插件 |
| 视图插件 | FrontViewPlugin | 前端注册表条目，与后端 ViewDef 按 id 配对 |
| iframe 桥 | iframe bridge | Shell 与第三方视图 dist 的 postMessage 协议（init/theme/ready/resize） |
| 层标注 | layer annotation | source 视图按 provenance 给行着层来源色的开关功能 |
| 候选修正卡 | CandidateFixCard | `asset_path_not_found` 错误的候选文件一键修正组件 |
| 输入描述符 | input descriptor | 与后端一致；前端负责从 URL 组装并序列化进分享链接 |

---

## 附录 A · 类型契约

- **单一来源**：后端 OpenAPI 自描述 → `openapi-typescript` 生成 `types/api.d.ts`；**权威 DTO 定义见后端设计附录 C**，本表只列前端专有类型。CI 步骤 `pnpm gen:api` + git diff 非空即失败（防契约漂移）。
- 前端专有类型：

```ts
// 视图插件运行时（§5.2 的完整版）
interface ViewProps {
  docs?: DocRefDTO[]
  itemQuery?: { rows: AssetItemDTO[]; total: number; fetchMore: () => Promise<void> }
  descriptor: InputDescriptor
  options: Record<string, unknown>
  theme: 'light' | 'dark'
}
type ViewEmits = {
  'update:options': [Record<string, unknown>]
  'navigate': [deepLink: string]          // 站内路由字符串，Shell 负责 router.push
}
// iframe 桥消息（判别联合）
type BridgeMsg =
  | { type:'init'; docUrl:string; theme:string; options:object }
  | { type:'theme'; theme:string }
  | { type:'ready' } | { type:'resize'; height:number }
// URL 中的文档选择器字符串形态（见附录 B）
type DocSelStr = `v:${string}` | `b:${string}` | `u:${string}`
```

## 附录 B · URL 序列化算法（可直接编码）

### B.1 Viewer Shell 路由 query 规范

路径：`/t/{t}/services/{slug}/assets/{kind}/{name}`

| query 参数 | 取值 | 缺省 |
| --- | --- | --- |
| `view` | ViewDef.id | 用户偏好 → 服务默认 → kind 首个 enabled 视图 |
| `doc` | DocSelStr：`v:<versionId>` / `b:<branch>`（branch 值 `encodeURIComponent`） | `v:<currentVersionId>`，无 current 则 `v:<latestVersionId>` |
| `left` / `right` | DocSelStr（含 `u:<uploadRef>`），仅 versions 类视图 | left=current，right 空时视图内提示选择 |
| `opts` | `base64url( JSON.stringify(options, sortedKeys) )` | 无（视图默认值） |
| `item` | 条目定位 `encodeURIComponent(itemKey)`（搜索深链用） | 无 |

### B.2 descriptor 组装算法（伪代码，实现于 `composables/useDescriptor.ts`）

```
function buildDescriptor(route, viewDef): InputDescriptor
  switch viewDef.input.mode:
    'single'     -> { mode:'single', assetId, doc: parseSel(route.query.doc ?? defaultDoc) }
    'versions'   -> { mode:'versions', assetId,
                      docs: [parseSel(left ?? defaultDoc), ...(right ? [parseSel(right)] : [])] }
    'collection' -> 从 DocSelectorModal 状态取（URL 形态: docs=<assetId>.<DocSelStr>,<...> 逗号分隔）
    'scope'      -> 由页面上下文注入（分组页: {mode:'scope',scope:'system_group',systemGroupId,kinds}）
  parseSel('v:x') = {versionId:x}; parseSel('b:x') = {assetId, branch:decodeURIComponent(x)}
  parseSel('u:x') = {uploadRef:x}
```

规则：
1. **序列化确定性**：`opts` 与 descriptor JSON 一律 key 排序后 stringify（分享链接与 query cache key 稳定的前提）。
2. **URL 是唯一状态源**：视图内改 options → `emit('update:options')` → Shell `router.replace` 更新 `opts` → watch 触发（同值跳过，防循环）。
3. **分享链接**：`POST /share-links` 的 body = 当前 `{descriptor, viewId, options}` 原样；不含任何本地状态。
4. 非法 query（解析失败/`input_spec_mismatch`）：回退该视图缺省 descriptor，`router.replace` 清洗 URL，toast 提示一次。

## 附录 C · 错误码 → UI 行为映射全表

> 与后端附录 B.2 注册表一一对应；集中实现于 `utils/errorMap.ts`，未映射 code 走兜底。`文案 key` 指 i18n key（zh/en 双语必备）。

| code | UI 行为 | 文案 key |
| --- | --- | --- |
| `unauthorized` | 清 session store → 跳 `/login?redirect=<当前>` | `err.unauthorized` |
| `not_found` | 页面级：404 空态页；操作级：toast"不存在或无权限访问" | `err.notFound` |
| `validation_error` | 表单：details.fields 按 path 定位到字段下方红字；非表单 toast | `err.validation` |
| `asset_path_not_found` | 资产源行内嵌 `CandidateFixCard`（details.candidates）| `err.pathNotFound` |
| `overlay_invalid` | YamlEditor 按 details.errors[].line/col 打行级标注 + gutter 图标 | `err.overlayInvalid` |
| `input_spec_mismatch` | Shell 回退默认视图 + toast（附录 B.2 规则 4） | `err.inputSpec` |
| `branch_not_indexed` | VersionPicker 该分支项置灰 + 内联按钮 [先同步该分支]（调 sync） | `err.branchNotIndexed` |
| `nesting_too_deep` | 分组表单 parent 字段错误提示 | `err.nestingTooDeep` |
| `ai_command_missing` | 弹层引导：跳转租户设置 ai 命令配置项 | `err.aiCommandMissing` |
| `version_not_publishable` | 弹层列出 details.blockingRevisions，每行 [去审批] 深链审批抽屉 | `err.notPublishable` |
| `base_layer_exists` | 新增源表单 role 字段禁用 base 选项 + 提示既有层链接 | `err.baseExists` |
| `quota_exceeded` | 弹层展示 details.{used,limit} + "联系租户管理员" | `err.quota` |
| `last_admin` | toast，阻断操作 | `err.lastAdmin` |
| `credential_in_use` | 弹层列 details.repositories + [强制解绑并删除]（二次确认后 `?force=true`） | `err.credInUse` |
| `conflict` | toast"数据已被他人修改，请刷新重试" + 该资源 query invalidate | `err.conflict` |
| `rate_limited` | toast 带 details.retryAfterSec 倒计时；期间禁用触发按钮 | `err.rateLimited` |
| `payload_too_large` | 编辑器/上传处提示 limitBytes 换算的可读大小 | `err.tooLarge` |
| `internal_error` | toast + traceId 可复制（"反馈时请附带"） | `err.internal` |
| 网络层失败 | 自动重试 GET ×2（指数退避）；mutation 不自动重试，给 [重试] 按钮 | `err.network` |

## 附录 D · 页面数据契约总表

> 每页一行：TanStack Query key（首元素恒为 tenantSlug，表内省略）→ 端点 → DTO。`失效联动` = 该页 mutation 成功后需 invalidate 的 key 前缀。

| 页面 | query key | 端点 → DTO | 失效联动 |
| --- | --- | --- | --- |
| 目录首页 | `['services', params]`、`['dashboard','tenant']` | `GET /services` → Page\<ServiceDTO>；`POST /views/resolve`(scope) | star/订阅 → `['services']` |
| 搜索 ⌘K | `['search', q, facets]` | `GET /search` → SearchResultDTO[] | —（只读，300ms 防抖 + 上次结果 keepPreviousData） |
| 仓库列表/详情 | `['repos']`、`['repo', id]`、`['repo', id, 'jobs']` | `GET /repositories(/{id})`、`GET /jobs?repositoryId=` | sync/discover/patch → `['repo', id]` 全前缀 |
| 录入向导 | `['credentials']`、`['repo', id, 'candidates']` | 对应 GET | accept/dismiss → candidates；import-config → `['repo',id]`+`['services']` |
| 服务主页 | `['service', slug]`、`['service', slug, 'sources']`、`['asset', id, 'versions']` | `GET /services/{slug}`、`GET .../sources`、`GET /assets/{id}/versions` | 源 CRUD → sources+service；publish → versions+service |
| Viewer Shell | `['views']`、`['resolve', viewId, descriptorHash]`、`['asset', id]` | `GET /views`、`POST /views/resolve`（descriptor 排序序列化后 hash 作 key） | 层操作/审批 → `['asset',id]` 与 `['resolve']` 全前缀 |
| layers 视图 | `['layer', id, 'revisions']` + Shell 的 asset key | `GET /layers/{id}/revisions` | 修订提交/回滚/启停 → asset+resolve+revisions |
| 编辑器预览 | 不进 query cache | `POST /assets/preview-merge`（800ms 防抖，AbortController 取消前序） | — |
| diff 视图 | `['diff', leftSel, rightSel, ruleSetId]` | `POST /diff` → DiffResponse | 保存快照 → 无（快照独立 key） |
| 审批中心 | `['reviews', filters]`、badge: `['reviews','count']` | `GET /layers/.../revisions?status=pending_review`（聚合端点或列表参数，随后端定）| approve/reject → reviews+badge+对应 asset |
| 待办中心 | `['todos', status]` | `GET /breaking-todos` | ack → todos |
| 分组详情 | `['group', slug]`、`['resolve','dep-graph',groupId]` | `GET /system-groups/{id}`、`POST /views/resolve`(scope) | 成员增删 → 两者 |
| 任务中心 | `['jobs', filters]`；详情日志走 `useSse(jobId)` 不进 cache | `GET /jobs`、SSE `/jobs/{id}/logs` | cancel → `['jobs']` |
| 通知 | `['notifications', unread]`、badge 60s refetchInterval | `GET /notifications` | read → 两者 |
| 设置各 tab | `['members']`/`['tokens']`/`['credentials']`/`['settings']`/`['viewdefs']`/`['audit',filters]` | 对应 GET | 各自 CRUD → 自身 key |
| 分享落地 | `['shared', token]` | `GET /shared/{token}` | — |

## 附录 E · 表单校验规则明细

> 前端校验仅为体验（即时反馈），**后端为权威**；规则与后端 422 的 details.fields 对齐（path 一致才能行内定位）。实现：Naive UI form rules + zod schema 复用。

### E.1 仓库表单

| 字段 | 规则 |
| --- | --- |
| url | 必填；正则二选一：`^git@[\w.-]+:[\w./-]+\.git$` 或 `^https?://[\w.-]+/[\w./-]+(\.git)?$`；不匹配提示"支持 SSH 或 HTTPS 格式" |
| credentialId | url 为 `git@`（SSH）时必选 ssh_key 类凭证；https 可选 http_token 或 none（联动过滤下拉项） |
| defaultBranch | 必填；`^[\w./-]{1,255}$`，禁 `..`、首尾 `/` |
| syncCron | 可空；5 段 cron 语法校验（cron-parser），非法即时红字 |
| fetchConfig.depth | shallow=true 时必填，1–1000 整数 |
| branchPolicy.track | 每项非空，glob 字符集 `[\w./*-]` |

### E.2 凭证表单（kind 联动）

| 字段 | 规则 |
| --- | --- |
| name | 必填 1–64 字符；租户内唯一（失焦异步校验，409 conflict 映射） |
| kind | 必填三选一；切换时清空另一分支字段 |
| sshKey.privateKeyPem | kind=ssh_key 必填；须含 `BEGIN ... PRIVATE KEY` 头，否则"不是有效的 PEM 私钥" |
| sshKey.passphrase | 可空；填写后二次输入确认 |
| httpToken.username / token | kind=http_token 均必填；token ≥ 8 字符 |
| sharedScope | 默认 private；改 tenant 时提示"租户内所有 maintainer 可用" |

### E.3 资产源表单（mode/role 联动，核心）

| 字段 | 规则 |
| --- | --- |
| assetKind | 必填；下拉 = 平台 kinds − 租户 disabledKinds |
| assetName | 可空（提示"留空将从路径推导"）；填写时 `^[a-z0-9][a-z0-9-]{0,63}$` |
| layerRole | 必填；该资产已有 base 时 base 选项禁用（`base_layer_exists` 预防） |
| layerOrigin | role=base 且 mode=builtin 时锁定 repo；mode=ai 锁定 ai_generated；mode=push 默认 third_party |
| mode | 必填；**联动必填**：builtin⇒path；command/ai⇒command；manual/push⇒path 与 command 均隐藏 |
| path | builtin 必填；glob 语法预检（括号/花括号配对）；提交后若 `asset_path_not_found` 走候选修正卡 |
| command | command/ai 必填；≤ 512 字符；提示"将在服务根目录快照内执行" |
| ord | overlay 显示，0–99 整数；同资产重复值提交时按 conflict 映射提示 |
| timeoutSec | 10–3600；mode=ai 默认 600，其它默认 120 |

### E.4 PAT 表单

| 字段 | 规则 |
| --- | --- |
| name | 必填 1–64 |
| scopes | 至少选 1 项（checkbox 组） |
| expiresAt | 单选：30/90/180 天/自定义日期（≤ 2 年）/永不（需二次确认"安全风险"提示） |

### E.5 overlay 编辑器（保存前本地预检）

| 检查 | 规则 |
| --- | --- |
| 大小 | ≤ 1MB（与 `REVISION_MAX_BYTES` 一致），超限禁用保存按钮并提示 |
| YAML 语法 | js-yaml 解析（Worker 中），语法错误行内标注、禁用保存 |
| 方言头 | 须含 `overlay: "1.0.0"` 或 `overlay: platform/v1`；platform/v1 须含 `target_kind` 且等于当前资产 kind（本地即校验，减少一轮 422） |
| 服务端回执 | `overlay_invalid` 的 details.errors 逐条映射到编辑器行（附录 C） |

### E.6 其它表单速览

| 表单 | 关键规则 |
| --- | --- |
| 成员邀请 | userId 必填（搜索选择）；role 必填；重复邀请按 conflict 提示 |
| 系统分组 | slug `^[a-z0-9-]{1,64}$`；parentId 选择时过滤掉已有 parent 的分组（前端预防 nesting_too_deep） |
| 分享链接 | expiresIn 单选：1h/24h/7d/30d；生成后 URL 只读框 + 复制按钮 |
| 驳回意见 | comment 必填 1–1000 字符（阻断提交） |
| breaking 确认 | comment 可空 ≤ 1000 |
| 服务设置 | slug 只读（创建后不可改）；visibility 改 public 时二次确认"匿名可见" |
| 租户设置 | aiTrustMode 开启时二次确认"AI/第三方修订将跳过审批"；webhook URL 必须 `https?://` |




