# Meridian 实施进度

> 最后核对：2026-09-05
> 当前里程碑：M0（foundation）
> 里程碑状态：进行中，尚未放行
> 最新稳定提交：`f3b20fe feat: add nuxt tenant control plane`
> 当前开发切片：M0 Acceptance/Smoke 与 executable spikes

本文只记录实施状态和验证证据，不定义产品行为，也不替代契约。范围、接口、领域规则、存储和验收发生冲突时，依次回到 [`contracts/manifest.yaml`](../contracts/manifest.yaml) 引用的对应契约；里程碑是否完成以 [`contracts/acceptance.yaml`](../contracts/acceptance.yaml) 为准。

## 状态规则

| 状态 | 含义 |
| --- | --- |
| 已完成 | 当前切片已经实现、通过切片门禁并提交；不代表所属里程碑已经放行 |
| 部分完成 | 已有实现和测试，但对应 Acceptance/Smoke 仍有断言未覆盖 |
| 开发中 | 工作区存在未提交实现，完整门禁尚未通过 |
| 未开始 | 只有契约、DDL 或生成的 501 transport stub，没有可用业务实现 |
| 阻塞 | 继续实施需要产品决策、外部权限或人工提供环境信息 |

不使用主观完成百分比。只有对应用户故事、Smoke、负向用例和里程碑门禁全部通过，才把里程碑标记为“已完成”。生成代码和数据库表已经存在，也不能单独视为业务能力完成。

## 里程碑总览

| 里程碑 | 交付范围 | 当前状态 |
| --- | --- | --- |
| M0 地基 | 契约与生成、迁移、身份/租户/RBAC/PAT、凭据、仓库骨架、Blob、Job、Audit、Outbox、控制面基础 | 进行中 |
| M1 资产主链路 | Repository/Service/Source、发现与同步、默认分支 Track、OpenAPI normalize/index、Viewer/public read | 未开始业务实现；只有契约和生成接口，M0 仓库骨架除外 |
| M2 Layer 与 Overlay | LayerHead/Revision、Overlay、人工编辑、Provenance、Rollback、字段级 GitOps | 未开始业务实现 |
| M3 AI、Diff 与门禁 | AI producer/review、生命周期、分支版本、Diff、分享、Todo、CLI push/diff、最小通知 | 未开始业务实现 |
| M4 多 kind 与全局视图 | dbschema/dependency、SystemGroup、依赖图、搜索和全局视图 | 未开始业务实现 |
| M5 协作与开放集成 | 通用订阅、Inbox、Webhook、AsyncAPI、合规和运维加固 | 未开始业务实现 |

`M0-M3` 是 MVP，`M4-M5` 是 v1 扩展；`M6+` 不在 v1 交付范围内。

## M0 当前状态

| 实施切片 | 实现状态 | 验收状态 | 证据 |
| --- | --- | --- | --- |
| Go 1.27.1、Node 24 LTS、pnpm 11 与 vfox 工具链 | 已完成 | 切片门禁已通过 | `b29514f`，`.vfox.toml`、Makefile |
| `cmd/meridian` 单入口、Nuxt 静态产物嵌入、健康检查和 API/static 404 边界 | 已完成 | 后端自动化覆盖存在；M0 全量前端 Spike 尚未放行 | `b29514f`，`internal/handler/server_test.go` |
| OpenAPI 驱动的 oapi-codegen/Orval 生成和无漂移检查 | 已完成 | 生成门禁已建立 | `b29514f`、`5e3a45a`、`ddeae8d` |
| 生成 API 导出声明/字段注释与应用 DDL 每列注释 | 已完成 | OpenAPI/生成 Go 导出注释和实际迁移列注释均由契约测试强制 | `5e3a45a`，`internal/contracttest/openapi_documentation_test.go`，`internal/contracttest/storage_documentation_test.go` |
| Goose 应用迁移、River 迁移及 up/down/up 生命周期 | 已完成 | SMK-001 的数据库主路径已有集成覆盖，里程碑 Smoke 尚未统一放行 | `ab1635f`、`7ef39f8`，`internal/database/database_integration_test.go` |
| sqlc + pgx 强类型持久化基础 | 已完成 | 切片门禁已通过 | `ddeae8d` |
| 登录、CSRF、用户、租户、成员、RBAC 和 PAT | 已完成 | US-01、SMK-002/003/004 部分覆盖；全租户 operation 隔离矩阵和控制面 E2E 未完成 | `ac8da08`，`internal/handler/identity_integration_test.go` |
| 凭据加密、服务端指纹和非回显 | 已完成 | SMK-005 部分覆盖，正式独立 fixture 尚未统一放行 | `31e3440`、`d976cde`、`66f473c` |
| 凭据轮换、原子仓库同步任务投递和幂等重放 | 已完成 | SMK-005/031 部分覆盖；平台 Job 查询和 Worker 终态执行已接入 | `394ffe7`、`b270b9d`、`b8d2ef6`、`147b155` |
| 租户/全局凭据连接探测和仓库连接预检 | 已完成 | 成功、分类失败、超时和秘密脱敏已有单元/集成覆盖 | `354af57` |
| 全局凭据管理及强制删除 | 部分完成 | CRUD/轮换/删除、引用仓库解绑健康状态和平台 Job 投影已实现；正式 Smoke fixture 仍待统一放行 | `66f473c`、`394ffe7`、`b270b9d`、`b8d2ef6` |
| Known Host 创建、派生指纹和列表 | 部分完成 | 服务端派生规则已有单测和 handler；SMK-035 独立 API 场景尚未统一放行 | `31e3440`、`66f473c` |
| 仓库 CRUD、凭据绑定、URL 规范化、ETag 和配额 | 已完成 | SMK-029 的原子配额/409 details/计数不变及 SMK-031 的全局凭据解绑健康状态已有 HTTP 集成断言；统一 Smoke 仍随 M0 放行 | `bd954d4`、`internal/handler/identity_integration_test.go` |
| River Worker 与通用 Job 控制面 | 部分完成 | 已接入事务内 River 入队、`river_job_id` 关联、Worker attempt fencing、六阶段状态推进和可重放阶段日志；已完成租户 Job 查询/详情、取消、SSE 重放/心跳/终态关闭、`repo.sync` 手工重试代际和 24 小时幂等；其它 Job 类型的手工重试需等对应 Worker 参数契约 | `147b155`、`ea589ad` |
| Audit 查询与权限边界 | 已完成 | 租户和平台查询、过滤、分页、元数据脱敏、租户隔离及平台 404 边界已有单元和真实 HTTP/PG 集成覆盖 | `db920bc` |
| Outbox 事务与分发基础 | 已完成 | `collect.failed` 与 Job 终态/审计同事务；周期扫描、SKIP LOCKED、六次尝试、退避、租约回收和旧 Worker 栅栏已有单元及真实 PG/River 覆盖；订阅路由与 webhook/in-app/email 适配按契约属于 M5 | `78cbc38` |
| 本地 SHA-256 CAS Blob 驱动 | 已完成 | 流式摘要、排他原子发布、去重、损坏检测、短时内容能力、租户唯一字节配额和真实 PG 覆盖均已通过；内容 HTTP endpoint 按契约在 M1 资产消费者接入 | `6c79863` |
| M0 Nuxt 控制面 | 部分完成 | 登录、租户壳、认证/租户守卫、仓库/凭据/Job 查询、Job 取消/重试/SSE、桌面/移动导航已实现；创建仓库/凭据表单、平台级管理视图和完整 Smoke fixture 仍未闭环 | `f3b20fe` |
| M0 executable spikes | 部分完成 | 单二进制、生成和迁移已有基础；Nuxt 类型检查、静态生成、桌面/移动 Playwright 与 axe 已通过；CodeMirror 大文件、Table/Cytoscape 性能 spike 尚未放行 | `f3b20fe`、[`05_technology-stack-decision.md`](./05_technology-stack-decision.md) 第 8 节 |

## M0 验收矩阵

| 验收项 | 当前状态 | 未闭环内容 |
| --- | --- | --- |
| US-01 | 部分完成 | 平台控制面、完整权限反例和独立 E2E fixture |
| US-11 | 部分完成 | 全 operation 隔离矩阵和统一 Smoke 仍未完成；仓库绑定后的全局凭据删除与平台 Job 查询已有覆盖 |
| SMK-001 | 部分完成 | 数据库迁移集成覆盖已有；仍需按 Smoke 入口统一执行 health/readiness/migration 断言 |
| SMK-002 | 部分完成 | 后端登录/租户切换已有；租户路由及前端缓存隔离未完成 |
| SMK-003 | 部分完成 | 认证错误和部分跨租户规则已有；尚未生成并执行所有租户资源 operation 的隔离矩阵 |
| SMK-004 | 部分完成 | PAT 创建、非回显、撤销和撤销后 401 已有集成覆盖；仍需独立 fixture 放行 |
| SMK-005 | 部分完成 | 加密、指纹、轮换、幂等、连接结果和脱敏已有；仍需完整引用仓库数量及正式连通 fixture |
| SMK-029 | 部分完成 | 原子配额检查、409 details、计数不变断言和 API 集成测试已覆盖；仍需独立 Smoke fixture 放行 |
| SMK-031 | 部分完成 | 全局凭据生命周期、绑定仓库解绑/health 和平台 Job 投影已有；仍需正式 Smoke fixture 放行 |
| SMK-035 | 部分完成 | 派生算法与服务已有；完整 API 正反例 fixture 尚未统一放行 |

## 当前工作区快照

最后稳定基线是 `f3b20fe`。该提交完成时，Go 单测、竞态测试、`go vet`、数据库/HTTP 集成测试、契约校验、DDL/生成注释审计和 `git diff --check` 均已通过；提交后的生成无漂移检查随后通过。

2026-09-05 核对时，仓库、平台 Job、River Worker、审计查询、M0 Outbox 与本地 CAS Blob 基础切片已通过门禁，包含：

- `migrations/queries/repository.sql`；
- sqlc 生成的 repository 查询代码；
- repository service 类型、URL/配置校验和配额错误模型；
- 仓库 API 的三态 PATCH、ETag、凭据绑定和软删除集成断言；
- 实际 M0 DDL 列注释审计测试。
- 平台管理员 Job 列表/详情的脱敏查询、过滤、分页和跨租户拒绝断言；`input`、`result`、`error`、凭据秘密和 River 内部字段不进入响应。
- 凭据轮换事务同时写入 Meridian `jobs`、River `river_job`，并在同一事务回填 `river_job_id`；
- River worker 启动时按 attempt fencing 抢占 durable job，避免旧 attempt 覆盖新 attempt；
- `resolve -> discover -> extract -> merge -> normalize -> index` 六阶段均写入带 advisory-lock 序列的 `job_stage_logs`，失败路径只写入脱敏错误；
- Worker/数据库集成测试验证 terminal failure、阶段日志数量、River kind 和应用状态。
- 租户/平台审计列表使用独立强类型查询；租户端固定 `tenant_id`，平台端要求平台管理员，并只映射契约允许的脱敏元数据。
- HTTP/PG 集成验证 deepObject 租户过滤、平台事实隔离和未授权 404。
- `collect.failed` 完整 AsyncAPI 信封与 terminal Job、阶段日志和 `job.failed` 审计使用同一 PostgreSQL 事务写入；共享 `eventId` 作为接收方去重键。
- River 启动及每分钟触发有界 Outbox 扫描；领取使用 `FOR UPDATE SKIP LOCKED`，失败只保留稳定错误码，五档退避覆盖六次尝试，过期租约可恢复且旧租约回写被栅栏拒绝。
- M0 默认没有通知通道；具体订阅匹配、配置解密和 webhook/in-app/email 投递适配保持在 M5，不以假成功冒充分发。
- 本地 Blob 以 SHA-256 派生唯一安全路径，采用临时文件、文件 `fsync` 和不覆盖目标的 hard-link 原子发布；重复写和并发写会复核目标摘要，磁盘损坏不会被静默接受。
- Blob 元数据与租户引用在同一事务内登记；按租户唯一摘要计费并锁定租户配额行，重复引用不重复占用配额，跨租户只复用全局不可变元数据。
- HMAC-SHA-256 内容能力最长 300 秒，绑定摘要、制品类型、媒体类型、Disposition 和可选分享记录，并支持 active/previous key 平滑轮换；实际内容 endpoint 随 M1 资产读取链路接入。
- 租户 Job 控制面固定以 `tenant_id` 做数据边界：列表/详情加载阶段日志并返回脱敏错误、阶段 attempt、进度和能力；`job:run` 才能取消或重试，PAT 必须同时具备精确租户绑定和 `job:run` scope。
- 取消在同一 PostgreSQL 事务内锁定 Meridian Job、调用 River `JobCancelTx` 并写入 `cancelled`；已终态 Job 返回 `job_not_cancellable`，跨租户资源统一表现为 404。
- 手工重试在同一事务内锁定幂等键、源 Job 和最新 generation，复制不可变 input，创建 `trigger=retry`、`retryOfJobId` 和新 River Job；相同 key/hash 精确重放，不同 hash 返回幂等冲突，存在等价 pending/running generation 返回状态冲突。M0 当前仅允许 `repo.sync`，因为其它 Job 类型尚未有可安全重建的 Worker 参数契约。
- Job SSE 首先发送当前 state，随后按持久化 sequence 重放日志；`Last-Event-ID` 只接受非负十进制序列，15 秒心跳使用注释帧，响应禁止缓存和代理缓冲，终态发送最终 state 后关闭。
- 迁移 `00004_job_stage_attempt.sql` 为阶段日志增加一基 attempt，并由 DDL 注释审计覆盖所有 Goose `Up` 迁移（含 `ADD COLUMN`）。
- Nuxt 控制面已接入生成的 API 客户端和 Vue Query：登录/退出、CSRF 会话缓存、租户路由隔离、仓库/凭据只读目录、Job 列表/详情/筛选/取消/重试/SSE 实时状态，以及响应式桌面侧栏和移动抽屉。
- 前端验证已通过 `vfox exec nodejs@24.20.0 -- pnpm --dir web typecheck`、`pnpm generate`、`pnpm check:generated-docs`；Playwright CI 矩阵为 6 项（5 passed、1 个桌面移动抽屉用例按视口跳过），每个执行项目的 axe 违规均为 0；1440px Dashboard 与 393px Jobs 抽屉截图无横向溢出。
- 当前前端边界是创建仓库/凭据按钮保持禁用，避免在后端对应写入契约和完整表单校验尚未落地时制造假成功；这些能力进入后续 M0 切片。

本阶段提交前已通过 `go1.27.1` 的 `go test ./...`、`go vet`、sqlc vet、数据库/HTTP 集成、契约校验和 DDL/生成注释审计；提交后必须再次执行生成无漂移门禁。

## 下一步顺序

1. 补齐 M0 Nuxt 创建仓库/凭据表单及平台级控制面缺口，并为写入路径增加契约/负向 E2E。
2. 完成 CodeMirror 大文件、Table/Cytoscape 性能等剩余 executable spikes。
3. 逐项运行 M0 Acceptance/Smoke；全部通过后才将 M0 标记为完成并开始 M1。

## 更新流程

每个开发切片都按以下顺序更新本文：

1. 开始实现前，将“当前开发切片”及对应表格状态改为“开发中”。
2. 实现后运行与改动范围匹配的生成、单测、静态检查、数据库/HTTP 集成和契约检查。
3. 门禁失败时，在“当前工作区快照”记录首个有效阻塞，不提前标记完成。
4. 门禁通过后，将切片状态改为“已完成”或“部分完成”，记录实际覆盖与尚未满足的验收断言。
5. 进度更新与代码一起提交，并把“最新稳定提交”更新为该提交 SHA。
6. 提交后重新执行生成无漂移检查，再自动进入下一切片。
7. 只有需要产品决策、外部权限或人工环境输入时才标记“阻塞”并暂停。

默认切片门禁：

```sh
vfox exec golang@1.27.1 -- go test ./...
vfox exec golang@1.27.1 -- go vet ./...
./scripts/test-integration.sh
make contracts-validate
make contracts-generate-then-git-diff-exit-code
git diff --check
```

涉及前端时还必须执行生成、类型检查、Playwright、移动/桌面视口和 axe；涉及迁移时必须额外执行空库 up、显式 down、再次 up 和事务回滚验证。

## 当前阻塞

没有需要人工决策的阻塞。
