# Meridian 实施进度

> 最后核对：2026-09-05
> 当前里程碑：M0（foundation）
> 里程碑状态：进行中，尚未放行
> 最新稳定提交：`b8d2ef6 feat: add redacted platform job queries`
> 当前开发切片：River Worker 基础与 Job 阶段日志（待提交）

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
| 凭据轮换、原子仓库同步任务投递和幂等重放 | 已完成 | SMK-005/031 部分覆盖；平台 Job 查询已接入，Worker 执行仍待闭环 | `394ffe7`、`b270b9d`、`b8d2ef6` |
| 租户/全局凭据连接探测和仓库连接预检 | 已完成 | 成功、分类失败、超时和秘密脱敏已有单元/集成覆盖 | `354af57` |
| 全局凭据管理及强制删除 | 部分完成 | CRUD/轮换/删除、引用仓库解绑健康状态和平台 Job 投影已实现；正式 Smoke fixture 仍待统一放行 | `66f473c`、`394ffe7`、`b270b9d`、`b8d2ef6` |
| Known Host 创建、派生指纹和列表 | 部分完成 | 服务端派生规则已有单测和 handler；SMK-035 独立 API 场景尚未统一放行 | `31e3440`、`66f473c` |
| 仓库 CRUD、凭据绑定、URL 规范化、ETag 和配额 | 已完成 | SMK-029 的原子配额/409 details/计数不变及 SMK-031 的全局凭据解绑健康状态已有 HTTP 集成断言；统一 Smoke 仍随 M0 放行 | 本阶段提交，`internal/handler/identity_integration_test.go` |
| River Worker 与通用 Job 控制面 | 开发中 | 已接入事务内 River 入队、`river_job_id` 关联、Worker attempt fencing、六阶段状态推进和可重放阶段日志；本切片完整门禁尚未提交 | `ab1635f`、`394ffe7`、`b8d2ef6` |
| Audit 与 Outbox 事务闭环 | 未开始 | 只有 M0 DDL，尚无业务写入、分发和读取闭环 | `ab1635f` |
| 本地 SHA-256 CAS Blob 驱动 | 未开始 | 只有 M0 DDL，尚无存储驱动和签名读取闭环 | `ab1635f` |
| M0 Nuxt 控制面 | 未开始 | 当前只有静态应用壳、生成客户端和 Query 插件；登录、租户壳、凭据/仓库页面及 E2E 未完成 | `b29514f` |
| M0 executable spikes | 部分完成 | 单二进制、生成和迁移已有基础；CodeMirror 大文件、Table/Cytoscape 性能、桌面/移动端 Playwright 与 axe 尚未放行 | [`05_technology-stack-decision.md`](./05_technology-stack-decision.md) 第 8 节 |

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

最后稳定基线是 `b8d2ef6`。该提交完成时，Go 单测、`go vet`、数据库/HTTP 集成测试、契约校验、DDL/生成注释审计和 `git diff --check` 均已通过；提交后的生成无漂移检查随后通过。

2026-09-05 核对时，仓库与平台 Job 投影切片已通过门禁，包含：

- `migrations/queries/repository.sql`；
- sqlc 生成的 repository 查询代码；
- repository service 类型、URL/配置校验和配额错误模型；
- 仓库 API 的三态 PATCH、ETag、凭据绑定和软删除集成断言；
- 实际 M0 DDL 列注释审计测试。
- 平台管理员 Job 列表/详情的脱敏查询、过滤、分页和跨租户拒绝断言；`input`、`result`、`error`、凭据秘密和 River 内部字段不进入响应。

当前未提交的 Worker 切片已通过：

- 凭据轮换事务同时写入 Meridian `jobs`、River `river_job`，并在同一事务回填 `river_job_id`；
- River worker 启动时按 attempt fencing 抢占 durable job，避免旧 attempt 覆盖新 attempt；
- `resolve -> discover -> extract -> merge -> normalize -> index` 六阶段均写入带 advisory-lock 序列的 `job_stage_logs`，失败路径只写入脱敏错误；
- Worker/数据库集成测试验证 terminal failure、阶段日志数量、River kind 和应用状态。

本阶段提交前已通过 `vfox exec golang@1.27.1 -- go test ./...`、`go vet`、sqlc vet、数据库/HTTP 集成、契约校验和 DDL/生成注释审计；提交后必须再次执行生成无漂移门禁。

## 下一步顺序

1. 用实际仓库绑定闭环全局凭据强制删除后的轮换 Job 数量断言。
2. 提交并保持 River Worker 基础、事务内入队和 Job 阶段日志切片。
3. 完成 Audit/Outbox 的同事务写入与分发闭环。
4. 完成本地 SHA-256 CAS Blob 驱动。
5. 完成 M0 Nuxt 控制面和桌面/移动端 E2E、axe 及剩余 executable spikes。
6. 逐项运行 M0 Acceptance/Smoke；全部通过后才将 M0 标记为完成并开始 M1。

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

没有需要人工决策的阻塞。当前编译错误是普通开发问题，应由正在进行的仓库切片自行修复。
