# Meridian 技术栈冻结决策

> 状态：Accepted / Frozen
> 决策日期：2026-09-05
> 机器可读版本基线：[contracts/manifest.yaml](../contracts/manifest.yaml)
> 适用范围：M0-M5；任何替换必须更新本文和 manifest，并通过本文件第 8 节门禁。

## 1. 决策目标

技术栈按 Meridian 的真实工作负载选择，而不是按框架热度选择：多租户 B 端控制面、深层动态路由、10k 行表格、1 MiB 可编辑 YAML/JSON、5-10 MiB 只读文档、side-by-side diff、500 节点/5000 边依赖图、OpenAPI 契约驱动、PostgreSQL 事务任务，以及 Go 单二进制私有化交付。

排序后的决策标准是：业务适配和正确性、开发反馈速度、契约类型安全、私有化交付复杂度、可访问性与大数据性能、长期维护面。团队熟悉度可影响上手成本，但不能覆盖前五项。

## 2. 冻结结论

| 层 | 冻结选择 | 责任边界 |
| --- | --- | --- |
| 交付形态 | Go 模块化单体 + 内嵌 SPA + PostgreSQL + 本地 CAS blob | 一个静态二进制和一个 PG 依赖；不引入 Node runtime、Redis、MQ、ES、MongoDB |
| 后端语言 | Go `>=1.27.1,<1.28` | HTTP、领域、worker、CLI；泛型方法边界见第 4 节 |
| HTTP/契约 | Chi v5 + oapi-codegen v2 strict server + nethttp middleware | `net/http` 兼容；OpenAPI 先行并生成 transport DTO/interface |
| 数据访问 | PostgreSQL 16+ + pgx/v5 + sqlc | 纯 SQL、编译期类型检查；不使用 ORM |
| 异步任务 | River 0.33 line | 与业务写入共用 `pgx.Tx`；外部通知仍经 outbox |
| 迁移 | goose v3 + `//go:embed` SQL | 启动只 auto-up；River 和应用 migration 共用 PG advisory lock |
| 资产引擎 | pb33f/libopenapi + evanphx/json-patch/v5 | OpenAPI AST/diff 与 RFC patch；Meridian 自己负责 Overlay 编译、scope、顺序和 provenance |
| 前端框架 | Nuxt 4 + Vue 3 + TypeScript 5.9，`ssr:false` | 纯 SPA、文件路由、布局、自动导入、Vite HMR/代码分割；不运行 Nitro server |
| API 客户端 | Orval 8 `vue-query` + Fetch + MSW | 从 OpenAPI 生成 model/request/query key/hook/mock；一个窄 custom fetcher |
| Server state | TanStack Vue Query 5 | API 缓存、轮询、重试、失效；Pinia 不复制 API entity |
| Client state | Pinia 3 | 仅导航、drawer、本地未提交草稿等 UI 状态 |
| UI/样式 | Nuxt UI 4 + Tailwind CSS 4 | B 端壳、表单、弹层、Tree、Splitter、Table/Virtual、主题与 A11y |
| Schema/Form | Ajv 8 + Zod 4 | Ajv 校验动态 JSON Schema；Zod 校验静态 UI 表单 |
| 编辑器/Diff | CodeMirror 6 + `@codemirror/merge` | YAML/JSON 编辑、只读源码、并排 diff；大文档计算进 Worker |
| 依赖图 | Cytoscape.js 3 canvas renderer | 只读图分析、布局、选择、邻域交互；不用 node-flow 编辑器 |
| 国际化/图标 | Nuxt i18n 10 + Nuxt Icon + 本地 Lucide 集 | `zh-CN`/`en`；运行时不访问公共 CDN |
| 测试 | Go test/testcontainers + Vitest/VTU + MSW + Playwright | 单元、SQL/队列集成、契约、键盘/A11y、视觉和 E2E |

版本范围由 manifest 冻结，落库后的 `go.sum` 和 `pnpm-lock.yaml` 精确锁定直接与传递依赖。不能用 `latest` 构建发布物。

## 3. 为什么保留 Nuxt/Vue

不硬切 React/Vite。Nuxt 4 本身默认使用 Vite，开发期仍获得快速 HMR；同时它提供文件路由、layout/middleware、自动导入和按页代码分割，正好覆盖 Meridian 的 admin/tenant/public/share 四类壳和大量深链。官方支持在 `ssr:false` 下生成经典静态 SPA，`.output/public` 可直接被 Go embed，因此不会为未使用的 SSR 能力引入生产 Node runtime。

候选比较：

| 候选 | 对 Meridian 的收益 | 未选择原因 |
| --- | --- | --- |
| Nuxt 4 SPA | 路由/布局约定、Vue DX、Vite、静态 fallback、Nuxt UI 一体化 | 冻结采用 |
| Vue + Vite + Vue Router | 最小、透明 | router/layout/module conventions、错误壳、i18n 和构建约束都要自行组装，M0 交付速度更慢 |
| Next.js 15 static export | React 生态和 App Router | SSR/RSC/server actions 在本产品不可用；任意租户深链的纯静态交付需要额外约束，框架收益与运行方式错位 |
| React + Vite | 生态广、自由度高 | 能实现但需要重写已冻结的 Vue view contract，并重新组合 dashboard、表格、表单和路由设施，没有可量化业务收益 |

生产约束是 `nuxt generate`，而不是 `nuxt build` 后启动 Nitro。仓库禁止添加 Nuxt server route、server middleware 和依赖请求时服务端上下文的实现。Go 静态服务器必须分别验证首页、任意租户深链、public/share 深链、静态 404 和 API 404。

## 4. Go 1.27 使用原则

采用当前稳定补丁 Go 1.27.1。Go 1.27 的泛型方法用于把已有的 package-level 通用变换收拢到具体辅助类型，例如 typed result、pipeline stage 和 collection adapter，从而改善类型推断与链式组合。它不改变 Meridian 的领域建模方式：interface method 不能声明类型参数，泛型方法也不能实现 interface method，因此 transport、domain、repository 和 provider 接口继续使用明确的领域操作。

`iter.Seq`/`iter.Seq2` 适合 libopenapi AST、items 和 provenance 的惰性遍历，但是否采用由 benchmark 决定。数据库 list、HTTP page 和跨 goroutine stream 不使用 iterator 冒充分页或 channel。禁止建立通用 repository、通用 CRUD handler 或反射式 mapper；这类抽象会隐藏 tenant 条件、事务和错误语义。

## 5. 前端关键取舍

### 5.1 UI 与状态

Nuxt UI 4 已覆盖 Dashboard、Form、Slideover、Splitter、Tree、Table 和 Virtual，并建立在 Tailwind CSS 4 与 Reka UI 上。它替换 Naive UI + UnoCSS + 单独 TanStack Table 三套并行设施，减少 theme token、表格 adapter 和无障碍行为的重复维护。业务组件可以组合 Nuxt UI primitives，但不复制整个 shadcn-vue 组件树。多租户品牌只通过受控 token/AppConfig 映射，不能从租户数据注入任意 class。

TanStack Query 是唯一 API server-state owner；Pinia 不保存 tenant、membership、Asset、Track、Job 或 capability 副本。当前 tenant 来自 route param，locale/theme 分别由 i18n/color-mode 管理。页面刷新必须能从 URL + API 完整恢复。

### 5.2 OpenAPI 客户端

不采用 `openapi-fetch`。其维护者已在 2026 路线图宣布转入维护模式，并明确指出复杂认证、序列化和 runtime 行为会迫使项目堆叠 wrapper。Meridian 恰好包含 Bearer token 静默续期、ETag、Idempotency-Key、multipart、统一错误、任务和多种 response，因此改用仍活跃的 Orval 8：生成 Fetch request、Vue Query hooks/query keys 与 MSW mock，减少人工胶水。

custom fetcher 只能处理同源 base URL、Bearer access token + `/auth/refresh` 续期、ErrorResponse/requestId 和全局 401；业务 header/body 必须在 OpenAPI operation 上出现。生成代码不能手改，也不能在 composable 中再包一层通用 CRUD SDK。

### 5.3 编辑器

CodeMirror 6 满足 JSON/YAML、高亮、lint、只读 source、MergeView 和扩展需求，且可按路由拆包。Monaco 的 diff、worker 与语言服务更完整，但当前没有补全、跳转、重构或 VS Code extension 兼容需求，其模型/worker 生命周期和包体成本不会转化为业务价值。

1 MiB 以内允许完整编辑/lint/preview；更大内容只读。5 MiB 以上禁用全量高亮、折叠、格式化和浏览器全文 diff，parse/search/format 放 Web Worker，Diff 使用后端 hunks。若未来验收明确要求本地 OpenAPI IntelliSense、跨文件引用跳转或 LSP，再以相同 10 MiB fixture 对 Monaco 做替换 spike。

### 5.4 依赖图

依赖图是分析/浏览，不是流程编排。Cytoscape.js 提供稳定的 canvas renderer、graph selector/algorithm、事件和内建 layout，适合 500 节点/5000 边上限。React Flow/Vue Flow 的 DOM/SVG node editor 强项与需求不一致；Sigma.js 的 WebGL 对数万节点更有优势，但当前规模下会增加布局和业务交互装配成本。

默认最多 500 节点/2000 边，超出走服务端 neighborhood；5000 是单次硬上限。性能配置和 layout 映射以 `03_frontend-technical-design.md` 为准。若硬上限提升到 20k 元素、需要连续动态布局，或真实 fixture 无法达到 2 秒门禁，再用 Sigma stable 与 Cytoscape 做同数据 benchmark，而不是预先双引擎兼容。

## 6. 后端关键取舍

Chi 保持 `net/http` 类型贯穿 middleware 与生成 handler；Gin/Echo 不提供本项目需要的额外价值。sqlc + pgx 保留 PostgreSQL SQL 能力并在生成期检查类型，避免 ORM 隐藏 tenant predicate、递归 CTE、JSONB 和批量索引行为。

River 让业务变更和任务插入共用 PG 事务，省去数据库与 broker 双写；通知/webhook 使用业务 outbox，避免把 queue job 当成外部事件事实。只有当任务必须跨数据库/跨区域独立扩缩，且 PostgreSQL 队列压测先成为瓶颈时，才评估独立 broker。

goose v3 支持内嵌 SQL 和程序化 provider，替换原文档中的 golang-migrate。River 自身 schema 迁移到应用记录的精确 target，再迁移应用 schema；二者持有同一个 session advisory lock。启动只 up，down 必须显式执行并先演练备份恢复。

PostgreSQL 16 是最低版本。关系约束、JSONB、递归 CTE、`tsvector` 与 trigram 足以满足 v1 的一致性和十万级检索目标；只有真实 production-like fixture 连续不能满足 P95，且 EXPLAIN/索引/分区优化已经穷尽，才评估 Elasticsearch 或图数据库。

## 7. 明确排除

v1 不引入 Next.js/React、独立 Vite SPA、Naive UI、UnoCSS、shadcn-vue 基线、Redux/Zustand、openapi-fetch、Axios、Monaco、React Flow/Vue Flow、Sigma、GORM、Redis、RabbitMQ/Kafka、Elasticsearch、MongoDB 或图数据库。这里的“排除”不是技术优劣判断，而是防止在当前需求范围内形成两套可选实现。

## 8. 冻结与变更门禁

M0 必须先完成以下 executable spike，全部通过才允许业务模块铺开：

1. Go 1.27.1 编译单二进制，embed Nuxt SPA；任意动态深链刷新成功，API 404 不回退 HTML；
2. canonical OpenAPI bundle 同时驱动各领域的 oapi-codegen server/models/spec 与 Orval Vue Query client/MSW，重新生成无 diff，fixture 请求通过；
3. pgx 事务内写业务行、outbox 和 River job，故障回滚后三者均不可见；空库 goose/River up、显式 down、再次 up 通过；
4. CodeMirror fixture 覆盖 1 MiB 编辑/preview 和 5/10 MiB 只读，交互期间没有超过 50 ms 的重复 long task；
5. Nuxt UI Table 覆盖 10k 虚拟行、键盘焦点和明确 action；Cytoscape fixture 覆盖 500 节点/5000 边并达到首屏 2 秒；
6. Playwright 在桌面和移动端验证登录壳、Viewer/Splitter、表格、图和深链无溢出，并执行 axe 主路径。

每项 spike 必须有固定 `make` target、fixture 路径、机器可解析 JSON 报告和硬阈值；报告纳入 `artifacts/spikes/<id>.json`。未通过或没有报告时，M0 只能保持进行中。M0-M3 先验证移动端功能可用和无溢出，视觉精修与截图基准放到 M4/M5。

依赖 patch 可在上述门禁全绿后更新；minor/major、同职责替换或引入排除项必须提交新 ADR，给出 Meridian fixture 的 build time、bundle、交互、A11y、实现代码量和维护面数据，并同步修改 manifest。不得以“生态更流行”作为变更依据。

## 9. 调研依据

- [Go 1.27 release notes](https://go.dev/doc/go1.27) 与 [release history](https://go.dev/doc/devel/release)
- [Nuxt 4 deployment / static hosting](https://nuxt.com/docs/4.x/getting-started/deployment) 与 [Nuxt introduction](https://nuxt.com/docs/4.x/getting-started/introduction)
- [Nuxt UI components](https://ui.nuxt.com/docs/components/) 与 [Nuxt UI releases](https://github.com/nuxt/ui/releases)
- [Node.js release policy](https://nodejs.org/en/about/previous-releases)
- [Orval documentation](https://orval.dev/docs/) 与 [Orval releases](https://github.com/orval-labs/orval/releases)
- [openapi-typescript 2026 roadmap](https://github.com/openapi-ts/openapi-typescript/discussions/2559)
- [CodeMirror MergeView reference](https://codemirror.net/docs/ref/#merge.MergeView)
- [Cytoscape.js performance guidance](https://js.cytoscape.org/#performance)
- [goose releases](https://github.com/pressly/goose/releases) 与 [River migration guidance](https://riverqueue.com/docs/migrations)
