# Meridian 实施进度

> 最后核对：2026-09-06
> 当前里程碑：M0（foundation）
> 里程碑状态：进行中，尚未放行
> 最新稳定提交：`ee9fcd9 feat(M0-AGENT-001): implement tenant control-plane update`
> 当前开发切片：M0-AGENT-002 凭据、Known Host 与 Smoke（门禁修复后待重试）

本文只记录实施状态和验证证据，不定义产品行为，也不替代契约。可领取的原子工作项、依赖和阶段人工放行记录 `milestoneGates` 以 [`contracts/work-items.yaml`](../contracts/work-items.yaml) 为准。范围、接口、领域规则、存储和验收发生冲突时，依次回到 [`contracts/manifest.yaml`](../contracts/manifest.yaml) 引用的对应契约；里程碑是否完成以 [`contracts/acceptance.yaml`](../contracts/acceptance.yaml)、工作项门禁和用户明确验收为准。

## 状态规则

| 状态 | 含义 |
| --- | --- |
| 已完成 | 当前切片已经实现、通过切片门禁并提交；不代表所属里程碑已经放行 |
| 部分完成 | 已有实现和测试，但对应 Acceptance/Smoke 仍有断言未覆盖 |
| 开发中 | 工作区存在未提交实现，完整门禁尚未通过 |
| 未开始 | 只有契约、DDL 或生成的 501 transport stub，没有可用业务实现 |
| 失败/待重试 | 门禁或测试失败，尚未达到重试上限 |
| 技术阻塞 | 同一技术根因连续失败，等待修复或人工介入 |
| 外部阻塞 | 需要产品决策、外部权限或人工提供环境信息 |
| 待验收 | 自动门禁通过，等待用户验收 |

这些中文状态是人工可读投影，不是工作项队列枚举。技术失败未耗尽尝试时为 `needs_retry`，耗尽后为 `failed` 并填写 `failureKind`；历史 `blocked_technical` 标签对应这个技术失败投影。外部依赖使用 `blocked`，历史 `blocked_external` 只是投影标签。`待验收` 对应 `milestoneGates.<M#>.status=needs_human_acceptance`，只有自动门禁全部通过的阶段才能进入；技术失败不能当作成品待验收。`stale` 是证据有效性，不是工作项状态；契约或相关代码变化后，旧证据不得继续作为当前通过依据。

不使用主观完成百分比。只有对应用户故事、Smoke、负向用例和里程碑门禁全部通过，并且 `milestoneGates.<M#>.status=accepted`，才把里程碑标记为“已完成”。生成代码和数据库表已经存在，也不能单独视为业务能力完成。同阶段 `passed` 的后继项可以继续开发；跨阶段必须等待用户明确放行，未回复不等于通过。

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

目前 M0-M5 的 `milestoneGates` 均为 `pending`，没有任何阶段被人工放行。每阶段所有工作项为 `passed` 后，Agent 必须提交独立阶段报告，包含桌面入口、可演示用户主路径、自动门禁和断言证据、延期事项，再转为 `needs_human_acceptance` 等待用户；反馈 `changes_requested` 时先新增本阶段修复项，不能直接跳到下一阶段。

M0-M3 采用 desktop-first：先实现完整桌面功能，移动视觉、动画和细间距可延期到具名后续工作项。功能、权限、错误状态、键盘可用性、A11y 和契约已声明的移动功能不能后置，延期不能减弱验收。

## M0 当前状态

| 实施切片 | 实现状态 | 验收状态 | 证据 |
| --- | --- | --- | --- |
| Go 1.27.1、Node 24 LTS、pnpm 11 与 vfox 工具链 | 已完成 | 切片门禁已通过 | `b29514f`，`.vfox.toml`、Makefile |
| `cmd/meridian` 单入口、Nuxt 静态产物嵌入、健康检查和 API/static 404 边界 | 已完成 | 后端自动化覆盖存在；M0 全量前端 Spike 尚未放行 | `b29514f`，`internal/handler/server_test.go` |
| OpenAPI canonical bundle、oapi-codegen/Orval 生成和无漂移检查 | 已完成 | 拆分源与 canonical 语义等价；12 个领域 server 包、统一路由装配、公共 models/spec 与前端生成门禁均已接入 | `scripts/bundle-openapi.sh`、`generate.go`、`internal/handler/domain_adapters.gen.go` |
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
| M0 Nuxt 控制面 | 部分完成 | 登录、租户壳、认证/租户守卫、仓库/凭据查询、创建、编辑、删除、Job 查询/取消/重试/SSE、桌面/移动导航和表单负向校验已实现；平台运维脱敏目录、管理员守卫和响应式壳已接入，租户更新 API 已用 ETag 集成验证；平台创建/成员编排 UI 与正式 Smoke fixture 仍待后续工作项 | `ee9fcd9`、`43ffa5c` |
| M0 executable spikes | 部分完成 | 单二进制、生成和迁移已有基础；Nuxt 类型检查、静态生成、桌面/移动 Playwright 与 axe 已通过；CodeMirror 大文件、Table/Cytoscape 性能 spike 尚未放行 | `c1e6b4e`、[`05_technology-stack-decision.md`](./05_technology-stack-decision.md) 第 8 节 |

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

最后稳定基线是 `ee9fcd9`。该提交完成时，平台租户 PATCH 已实现显式字段更新、平台管理员权限、If-Match 乐观并发控制和 412 陈旧版本响应；对应 PostgreSQL/HTTP 集成断言已通过。提交后的生成无漂移检查、go vet 和 `git diff --check` 随后通过。

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
- 平台运维视图已接入生成 API 客户端和 Vue Query：用户/租户脱敏目录、全局凭据/平台 Job/审计投影、平台管理员路由守卫，以及无租户平台管理员的租户业务 404 边界；用户密码哈希、租户 settings、凭据 secret、Job input/result/error 均不进入列表投影。
- 租户控制面 PATCH 已按契约支持 `displayName`、`status`、完整 `quota` 三态字段更新；仓储 SQL 使用显式 set flag 保留 omitted 语义，服务端校验平台权限和 tenant ETag，陈旧版本统一返回 412。
- 前端验证已通过 `vfox exec nodejs@24.20.0 -- pnpm --dir web typecheck`、`pnpm generate`、`pnpm check:generated-docs`；Playwright CI 矩阵为 18 项（17 passed、1 个移动导航用例按视口跳过），每个执行项目的 axe 违规均为 0；创建、编辑、删除路径均验证了 Query 刷新、`If-Match`，凭据 secret 不进入更新请求或列表 DOM。
- 当前前端边界是平台创建用户/租户、成员编排和完整 Smoke fixture 尚未接入；租户仓库/凭据控制面已具备创建、编辑、删除和并发更新保护，不以禁用按钮或假成功占位。
- Nuxt 创建表单已按 OpenAPI 严格构造 `RepositoryCreateRequest` 与 SSH/HTTP `CredentialCreateRequest` union；默认分支、公开仓库哨兵值、共享范围/团队 UUID、secret 最小长度和备注长度在客户端先校验，服务端 4xx 只展示脱敏错误。
- 创建成功后只失效当前租户对应的 Query key；E2E 验证幂等键请求、列表刷新、私钥不进入列表 DOM，以及弹窗在桌面/移动视口和 axe 下可用。

本阶段提交前已通过 `go1.27.1` 的 `go test ./...`、`go vet`、sqlc vet、数据库/HTTP 集成、契约校验和 DDL/生成注释审计；提交后已再次通过 `make contracts-generate-then-git-diff-exit-code`。

## 下一步顺序

下一项不再由本节文字推断，按 `contracts/work-items.yaml` 的选择规则领取。当前队列为：`M0-AGENT-001`（passed）→ `M0-AGENT-002`（needs_retry，本次用户授权恢复到 attempt=0）→ `M0-AGENT-003`。M0 的八项 Smoke 与全部自动门禁通过后先向用户报告，只有 M0 gate 为 `accepted` 才能进入 M1；M2 首项依赖 M1 最后一项 `M1-AGENT-003`，M3 首项依赖 `M2-AGENT-002`。每个工作项的命令、断言和报告路径以队列为准，SMK-033 属于 M3，SMK-030 体验镜像属于 M5。

## 更新流程

每个开发切片都按以下顺序更新本文：

1. 开始实现前，将“当前开发切片”及对应表格状态改为“开发中”。
2. 实现后运行与改动范围匹配的生成、单测、静态检查、数据库/HTTP 集成和契约检查。
3. 门禁失败时，在“当前工作区快照”记录首个有效阻塞，不提前标记完成。
4. 门禁通过后，将切片状态改为“已完成”或“部分完成”，记录实际覆盖与尚未满足的验收断言。
5. 进度更新与代码一起提交，并把“最新稳定提交”更新为该提交 SHA。
6. 提交后重新执行生成无漂移检查；同阶段有可领取项则继续，全阶段自动门禁通过则提交阶段报告并等待用户验收，禁止自动跨阶段。
7. 产品决策、外部权限或人工环境输入使用 `blocked`；技术失败耗尽使用 `failed` 和 `failureKind` 并请求技术介入。仓库内缺 target/fixture 是当前项的 `tooling_gap`，须实际修复，禁止无变化重复或自动清零计数。

默认切片门禁：

```sh
vfox exec golang@1.27.1 -- go test ./...
vfox exec golang@1.27.1 -- go vet ./...
./scripts/test-integration.sh
make contracts-validate
make contracts-generate-then-git-diff-exit-code
git diff --check
```

每次门禁必须生成工作项证据 JSON；命令失败不得标记“已完成”。前端、迁移、性能和安全附加门禁必须在工作项 `verify` 中以 Make target 名称数组声明，并依次执行 `make <target> ITEM=<工作项 ID>`。报告统一使用 `workItem`、`status`、`commands[]` 等队列 `evidence` 字段，具体格式见运行手册第 7 节；不得将旧报告字段或普通集成测试结果视为缺失 Smoke 断言已通过。

涉及前端时还必须执行生成、类型检查、桌面 Playwright 和 axe，移动端执行契约声明的功能/权限断言，视觉精修可以按上述规则延期；涉及迁移时必须额外执行空库 up、显式 down、再次 up 和事务回滚验证。

## 当前阻塞

`M0-AGENT-002` 的声明门禁曾经缺失，属于仓库内 `tooling_gap`，不是外部依赖。本次用户授权补齐可执行入口和真实断言，队列恢复为 `needs_retry/attempt=0`；一次性恢复原因及原 attempt=3 保存在 `recovery`。这不代表工作项已经通过：下一次 Agent 必须重新领取、检查 `make smoke-m0-credentials ITEM=M0-AGENT-002` 和全部声明门禁，补齐任何仍缺少的断言后再形成新证据。历史失败报告保留在 `artifacts/agent/M0-AGENT-002/`，不得改写或复用为通过证据。

后续阶段尚未实现的 target/fixture 同样属于对应工作项交付物，不能因此反复要求用户提供命令。若实际修复后仍达到重试上限，Agent 应给出明确技术失败报告；只有阶段自动门禁全绿时，才提交成品阶段验收报告。
