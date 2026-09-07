# Meridian 实施进度

> 最后核对：2026-09-08
> 当前里程碑：M1（asset-mainline）
> 里程碑状态：M0 自主 checkpoint 已完成；M1 开始继续推进
> 最新稳定提交：`0869beec21827bfa3984698828b536d3f8aa5398 feat(M0-CONTRACT-002): automate milestone checkpoints`
> 当前开发切片：M1-AGENT-001 claimed（attempt 2，租约至 2026-09-07T21:40:31Z）

本文只记录实施状态和验证证据，不定义产品行为，也不替代契约。可领取的原子工作项、依赖和阶段完成记录 `milestoneCheckpoints` 以 [`contracts/work-items.yaml`](../contracts/work-items.yaml) 为准。范围、接口、领域规则、存储和验收发生冲突时，依次回到 [`contracts/manifest.yaml`](../contracts/manifest.yaml) 引用的对应契约；里程碑是否完成以 [`contracts/acceptance.yaml`](../contracts/acceptance.yaml)、工作项门禁和 Agent 证据 checkpoint 为准。

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
| 待 checkpoint | 自动门禁通过，等待 Agent 固化里程碑证据 |

这些中文状态是人工可读投影，不是工作项队列枚举。技术失败未耗尽尝试时为 `needs_retry`，耗尽后为 `failed` 并填写 `failureKind`；历史 `blocked_technical` 标签对应这个技术失败投影。外部依赖使用 `blocked`，历史 `blocked_external` 只是投影标签。`待 checkpoint` 对应 `milestoneCheckpoints.<M#>.status=pending` 且该阶段工作项全部通过；Agent 固化完整证据后直接置为 `completed` 并继续。`stale` 是证据有效性，不是工作项状态；契约或相关代码变化后，旧证据不得继续作为当前通过依据。

不使用主观完成百分比。只有对应用户故事、Smoke、负向用例和里程碑门禁全部通过，并且 `milestoneCheckpoints.<M#>.status=completed`，才把里程碑标记为“已完成”。生成代码和数据库表已经存在，也不能单独视为业务能力完成。同阶段 `passed` 的后继项可以继续开发；跨阶段必须先由 Agent 完成所有较早里程碑 checkpoint。

## 里程碑总览

| 里程碑 | 交付范围 | 当前状态 |
| --- | --- | --- |
| M0 地基 | 契约与生成、迁移、身份/租户/RBAC/PAT、凭据、仓库骨架、Blob、Job、Audit、Outbox、控制面基础 | 已完成（Agent checkpoint） |
| M1 资产主链路 | Repository/Service/Source、发现与同步、默认分支 Track、OpenAPI normalize/index、Viewer/public read | 未开始业务实现；只有契约和生成接口，M0 仓库骨架除外 |
| M2 Layer 与 Overlay | LayerHead/Revision、Overlay、人工编辑、Provenance、Rollback、字段级 GitOps | 未开始业务实现 |
| M3 AI、Diff 与门禁 | AI producer/review、生命周期、分支版本、Diff、分享、Todo、CLI push/diff、最小通知 | 未开始业务实现 |
| M4 多 kind 与全局视图 | dbschema/dependency、SystemGroup、依赖图、搜索和全局视图 | 未开始业务实现 |
| M5 协作与开放集成 | 通用订阅、Inbox、Webhook、AsyncAPI、合规和运维加固 | 未开始业务实现 |

`M0-M3` 是 MVP，`M4-M5` 是 v1 扩展；`M6+` 不在 v1 交付范围内。

M0 曾由 souloss 于 `2026-09-07T23:40:01+08:00` 验收，历史报告保留在 `artifacts/agent/milestones/M0/20260906T182703Z/report.json`。M0-CONTRACT-002 已把后续里程碑切换为 Agent 自主、证据驱动的 checkpoint；完整 M0 quality gate 在源码提交 `0869beec21827bfa3984698828b536d3f8aa5398` 上重跑通过，完成报告为 `artifacts/agent/milestones/M0/20260907T205730Z/report.json`。M1-M5 仍为 `pending`。

M0-M3 采用 desktop-first：先实现完整桌面功能，移动视觉、动画和细间距可延期到具名后续工作项。功能、权限、错误状态、键盘可用性、A11y 和契约已声明的移动功能不能后置，延期不能减弱验收。

## M0 当前状态

| 实施切片 | 实现状态 | 验收状态 | 证据 |
| --- | --- | --- | --- |
| Go 1.27.1、Node 24 LTS、pnpm 11 与 vfox 工具链 | 已完成 | 切片门禁已通过 | `b29514f`，`.vfox.toml`、Makefile |
| `cmd/meridian` 单入口、Nuxt 静态产物嵌入、健康检查和 API/static 404 边界 | 已完成 | SMK-001 与 M0 聚合门禁通过 | `b29514f`、`036c9bc`，`internal/handler/m0_smoke_integration_test.go` |
| OpenAPI canonical bundle、oapi-codegen/Orval 生成和无漂移检查 | 已完成 | 拆分源与 canonical 语义等价；12 个领域 server 包、统一路由装配、公共 models/spec 与前端生成门禁均已接入 | `scripts/bundle-openapi.sh`、`generate.go`、`internal/handler/domain_adapters.gen.go` |
| 生成 API 导出声明/字段注释与应用 DDL 每列注释 | 已完成 | OpenAPI/生成 Go 导出注释和实际迁移列注释均由契约测试强制 | `5e3a45a`，`internal/contracttest/openapi_documentation_test.go`，`internal/contracttest/storage_documentation_test.go` |
| Goose 应用迁移、River 迁移及 up/down/up 生命周期 | 已完成 | SMK-001 独立数据库 Smoke 已通过 up/down/up 与 schema 等价断言 | `ab1635f`、`7ef39f8`、`036c9bc` |
| sqlc + pgx 强类型持久化基础 | 已完成 | 切片门禁已通过 | `ddeae8d` |
| 登录、CSRF、用户、租户、成员、RBAC 和 PAT | 已完成 | US-01 与 SMK-002/003/004 已通过；M0 验证 repository 资源族，完整 operation 矩阵由 M5 SMK-039 承接 | `ac8da08`、`036c9bc` |
| 凭据加密、服务端指纹和非回显 | 已完成 | SMK-005、SMK-031、SMK-035 与完整后端门禁通过；对象过滤按 deepObject 契约生成 | `31e3440`、`66f473c`、`5718bdf`、`036c9bc` |
| 凭据轮换、原子仓库同步任务投递和幂等重放 | 已完成 | SMK-005/031 轮换、幂等和同步任务断言及聚合门禁通过 | `394ffe7`、`b270b9d`、`b8d2ef6`、`147b155`、`036c9bc` |
| 租户/全局凭据连接探测和仓库连接预检 | 已完成 | SMK-005 真实 Git 探测与聚合门禁通过 | `354af57`、`036c9bc` |
| 全局凭据管理及强制删除 | 已完成 | SMK-031 生命周期、解绑和平台 Job 投影与聚合门禁通过 | `66f473c`、`394ffe7`、`036c9bc` |
| Known Host 创建、派生指纹和列表 | 已完成 | SMK-035 API 正反例与聚合门禁通过 | `31e3440`、`66f473c`、`036c9bc` |
| 仓库 CRUD、凭据绑定、URL 规范化、ETag 和配额 | 已完成 | SMK-003/029/031 独立 Smoke 通过；不存在/跨租户为 404，存在但 ETag 过期为 412 | `bd954d4`、`036c9bc` |
| River Worker 与通用 Job 控制面 | 部分完成 | 已接入事务内 River 入队、`river_job_id` 关联、Worker attempt fencing、六阶段状态推进和可重放阶段日志；已完成租户 Job 查询/详情、取消、SSE 重放/心跳/终态关闭、`repo.sync` 手工重试代际和 24 小时幂等；其它 Job 类型的手工重试需等对应 Worker 参数契约 | `147b155`、`ea589ad` |
| Audit 查询与权限边界 | 已完成 | 租户和平台查询、过滤、分页、元数据脱敏、租户隔离及平台 404 边界已有单元和真实 HTTP/PG 集成覆盖 | `db920bc` |
| Outbox 事务与分发基础 | 已完成 | `collect.failed` 与 Job 终态/审计同事务；周期扫描、SKIP LOCKED、六次尝试、退避、租约回收和旧 Worker 栅栏已有单元及真实 PG/River 覆盖；订阅路由与 webhook/in-app/email 适配按契约属于 M5 | `78cbc38` |
| 本地 SHA-256 CAS Blob 驱动 | 已完成 | 流式摘要、排他原子发布、去重、损坏检测、短时内容能力、租户唯一字节配额和真实 PG 覆盖均已通过；内容 HTTP endpoint 按契约在 M1 资产消费者接入 | `6c79863` |
| M0 Nuxt 控制面 | 已完成 | 登录、租户壳、控制面与响应式主路径已通过 M0-AGENT-001；本轮 production generate 和双视口 a11y 再次通过 | `ee9fcd9`、`43ffa5c`、`036c9bc` |
| M0 executable spikes | 已完成 | 10k UTable、500 节点/5000 边 Cytoscape、1/5/10 MiB CodeMirror 策略与桌面/移动 axe 门禁全部通过 | `036c9bc`、`artifacts/spikes/` |

## M0 验收矩阵

| 验收项 | 当前状态 | 未闭环内容 |
| --- | --- | --- |
| US-01 | 自动门禁通过 | M0-AGENT-001 控制面证据与本轮 SMK-002/004 独立 fixture 均通过 |
| US-11 | 自动门禁通过 | M0 repository 代表性隔离与凭据边界通过；完整 operation 矩阵按契约由 M5 SMK-039 执行 |
| SMK-001 | 通过 | 独立数据库 fixture 验证 health/readiness、up/down/up 和 schema 等价 |
| SMK-002 | 通过 | 登录、租户/成员、禁用租户排除和无 membership 平台管理员 404 通过 |
| SMK-003 | 通过 | repository GET/PATCH/DELETE 的匿名、跨租户、缺失资源、viewer mutation 与错误体等价通过 |
| SMK-004 | 通过 | PAT 创建时单次回显、列表不回显、撤销与撤销后 401 通过 |
| SMK-005 | 通过 | 加密、指纹、轮换、幂等、连接结果和脱敏 fixture 通过 |
| SMK-029 | 通过 | 原子配额、409 details 与拒绝后计数不变通过 |
| SMK-031 | 通过 | 全局凭据生命周期、仓库解绑/health 与平台 Job 投影通过 |
| SMK-035 | 通过 | Known Host 派生与 API 正反例 fixture 通过 |

## 当前工作区快照

最后稳定源码 checkpoint 是 `036c9bcaea05dd6e94e0b16b5accc94427f481d9`。该提交补齐逐 case 独立数据库 Smoke runner、M0-AGENT-003 的五个 fixture、前端 capability harness 和三类 executable spike；正式报告位于 `artifacts/agent/M0-AGENT-003/20260906T182703Z/report.json`。

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

M0 自动门禁和 Agent checkpoint 已完成。M1-AGENT-001 attempt 2 在实现前契约审计中发现 SMK-032/SMK-040 所需的五项行为没有确定定义，已保留为 `contract_failure` 并登记 M1-CONTRACT-002；M5 SMK-039 继续承担全部租户资源 ID operation 的完整隔离矩阵。

## 更新流程

每个开发切片都按以下顺序更新本文：

1. 开始实现前，将“当前开发切片”及对应表格状态改为“开发中”。
2. 实现后运行与改动范围匹配的生成、单测、静态检查、数据库/HTTP 集成和契约检查。
3. 门禁失败时，在“当前工作区快照”记录首个有效阻塞，不提前标记完成。
4. 门禁通过后，将切片状态改为“已完成”或“部分完成”，记录实际覆盖与尚未满足的验收断言。
5. 进度更新与代码一起提交，并把“最新稳定提交”更新为该提交 SHA。
6. 提交后重新执行生成无漂移检查；同阶段有可领取项则继续，全阶段自动门禁通过则提交 Agent checkpoint 并立即继续下一阶段。
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

M1-CONTRACT-001 已按用户决定拆分 operation/Smoke 归属并采用 `sourceExpansion` 子结构。attempt 1 的干净 worktree 生成前置缺口已保留在 `artifacts/agent/M1-CONTRACT-001/20260907T165358Z/report.json`；attempt 2 补齐 Nuxt prepare 和生成投影后通过，最终证据见 `artifacts/agent/M1-CONTRACT-001/20260907T170329Z/report.json`。M1-AGENT-001 attempt 2 的只读契约审计确认：Profile 列表缺少 requested kind 输入、候选接受缺少 Service visibility 默认值、候选 commit 身份规则互相冲突、discover 去重键在 resolve 前无法构造、Profile 创建时 dependencyStatus 的判定方式未定义。M1-CONTRACT-002 因这些产品口径进入 `blocked`；证据见 `artifacts/agent/M1-AGENT-001/20260907T212421Z/report.json`。

后续阶段尚未实现的 target/fixture 同样属于对应工作项交付物，不能因此反复要求用户提供命令。若实际修复后仍达到重试上限，Agent 应给出明确技术失败报告；只有阶段自动门禁全绿时，Agent 才能完成里程碑 checkpoint。
