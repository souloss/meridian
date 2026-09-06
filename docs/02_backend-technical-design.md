# Meridian 后端技术设计

> 状态：可实施基线（2026-09-04）
> 需求来源：[01_requirement-specification.md](./01_requirement-specification.md)
> 机器可读契约入口：[contracts/manifest.yaml](../contracts/manifest.yaml)
> 技术选型依据：[05_technology-stack-decision.md](./05_technology-stack-decision.md)
> 本文解释实现边界与事务设计；HTTP DTO、枚举、状态机、事件和表结构不在本文重复定义。

## 1. 权威来源与变更规则

开发、联调和验收必须从 `contracts/manifest.yaml` 进入。各契约按所有权分工：

1. `contracts/openapi.yaml`：HTTP 路径、方法、鉴权、请求响应和错误；
2. `contracts/domain.yaml`：状态机、权限、默认值、限制、合并与恢复语义；
3. `contracts/storage.yaml`：持久化约束和事务边界；
4. `contracts/kinds.yaml`、`views.yaml`、`events.yaml`：插件、视图和异步通知；
5. `contracts/cli.yaml`：`meridian` CLI 参数、selector、API 映射、输出和退出码；
6. `contracts/acceptance.yaml`：用户故事、fixture 与可执行断言；
7. 本文及前端设计：实现解释，不得覆盖上述契约。

契约之间没有“后者覆盖前者”的隐式优先级。OpenAPI/数据库中的领域枚举是 `domain.yaml` 的投影，必须完全一致；不一致即校验失败并阻止开发或合并。

契约变更必须先改 YAML，再重新生成 Go/TypeScript 类型并运行契约校验。禁止在 handler、前端类型或 Markdown 中维护第二份枚举和 DTO。

首发范围是 `M0-M3`。其中 AI 待审通知、breaking 待办和 diff 分享属于闭环必需能力，已前移到 `M3`。`M4-M5` 是 v1 扩展；`P2` 不进入首发，具体边界见 manifest。

## 2. 架构与代码边界

后端采用 Go 模块化单体、PostgreSQL、本地持久卷上的 content-addressed blob store、River PostgreSQL 队列和 Git CLI。建议目录：

基础实现栈固定为 Go 1.27.1、`oapi-codegen >=2.8,<3` 的 strict server/models、`chi/v5` 路由及 `nethttp-middleware` 请求校验、`pgx/v5` + `sqlc` 数据访问、goose v3 内嵌 SQL 迁移、River PostgreSQL 队列、标准库 `slog` JSON 日志和 Prometheus client 指标。OpenAPI 资产使用 `pb33f/libopenapi` 解析、规范化输入和结构化 diff，RFC 6902/7386 patch 使用 `evanphx/json-patch/v5` 执行。使用构造函数手工组装依赖，不引入运行时 DI 容器、ORM、Redis 或独立消息中间件；版本范围见 manifest，具体版本由 `go.mod/go.sum` 精确锁定。

Go 1.27.1 的使用边界固定如下：

- AST、provenance 和 item 流的只读遍历优先暴露 `iter.Seq`/`iter.Seq2`，但仅在可以减少中间 slice 且 benchmark 证明确有收益时使用；
- 泛型方法只用于具体辅助类型上的类型保持或类型变换，例如 pipeline/result/collection adapter；domain、repository、provider 和生成的 transport 接口不得声明泛型方法；
- 不为追求链式 API 把业务动作抽象成通用 `Map/Filter/Reduce`。审批、发布、合并和事务仍使用领域命名方法；
- 泛型 helper 必须有普通单测、类型推断 compile test 和 benchmark。出现额外分配、错误语义被隐藏或调用点比直接代码更难读时，改回具体函数；
- 契约 hash、canonical JSON 和生成 DTO 暂不直接切换到新的 JSON API；只有 golden fixtures 与旧产物逐字节一致后才可单独立项迁移。

```text
cmd/meridian               唯一二进制入口，Cobra 编排 serve 与业务子命令
configs                    oapi-codegen 等生成器配置
contracts                  唯一机器可读契约源
internal/command           Cobra 命令、参数与进程生命周期
internal/generated/api     oapi-codegen 生成的领域 transport，只读
internal/generated/api
                          跨领域共享 models、错误哨兵和嵌入式 canonical spec
internal/generated/api/<domain>
                          各 API 领域的 models、strict server、路由注册与 spec
internal/generated/repository sqlc 生成的 models/queries/DBTX，只读
internal/handler           HTTP 适配与生成接口实现，按业务域拆文件
internal/domain            纯领域模型、状态机和不变量
internal/service           用例、事务和权限编排
internal/repository        手写事务边界与仓储适配；依赖 generated/repository
internal/engine            合并、overlay 与 provenance 纯函数
internal/differ            结构化 diff 与 breaking 分析
internal/producer          builtin/command/AI provider 调度
internal/task              River worker、租约、重试和恢复
internal/storage           本地 CAS blob 与签名下载
migrations                 goose SQL 与 sqlc 查询源
```

依赖方向固定为 `cmd -> command -> handler -> service`，repository 实现 service 声明的持久化端口；各领域 handler 只依赖本领域生成 transport 和根 `api` 包中的共享 models，并将其映射到用例输入，领域注册适配器集中在 `internal/handler/domain_adapters.gen.go`。领域生成包之间不得互相依赖；跨领域共享类型必须下沉到根 `internal/generated/api`。跨仓储用例事务由 service 通过事务端口编排，单仓储原子写可封装在 repository 内；纯算法包不得依赖数据库、HTTP、生成 DTO 或任务实现。外部工具、Git、对象存储、AI producer 都通过端口适配器接入。

`contracts/api/` 是唯一可编辑的 TypeSpec 源：`main.tsp` 汇总公共协议和所有领域入口，根 `models.tsp` facade 导入 `common/` 技术协议与 `domains/<domain>/models.tsp` 领域模型；`domains/<domain>/routes.tsp` 按领域维护 HTTP operation。脚本把 TypeSpec 编译为 `contracts/openapi.yaml` 以及 `build/contracts/api/{common,domains}`，不在源目录维护任何重复 YAML，也不使用单大文档的 `include-tags` 过滤。当前根 facade 仍作为兼容生成边界，待跨领域 `$ref` 和 Go import-mapping 完成验证后再拆分 Go models 包；12 个领域 spec 由自动遍历脚本生成各自的 strict server；canonical spec 生成根包的嵌入式运行时校验文档。每个领域 server 只注册本领域路径，应用启动时由 `internal/handler/domain_adapters.gen.go` 将各领域注册器装配到同一个 Chi router。所有 operation、schema、属性和参数必须在 TypeSpec 提供语义描述；描述传播为 GoDoc/JSDoc，生成器自身产生的 transport 包装类型再由确定性后处理补注释。契约测试检查 TypeSpec 源、canonical bundle、领域 operation 集合、全部 Go 导出声明/字段以及 TypeScript 生成物，禁止手改生成文件。服务从 M0 起始终装配各领域 Strict Server；确定性生成的领域 `strict_unimplemented.gen.go` 为尚未进入当前实现阶段的 operation 返回 501，当前阶段实现通过编译期覆盖对应方法。

浏览器静态产物固定从 `web/.output/public` embed 到同一 Go 二进制。Go HTTP 层先匹配 `/api/*`、下载制品和真实静态文件；只对已登记的前端路由回退 `index.html`，不得让 API 404 被 SPA fallback 吞掉。带内容 hash 的资源使用一年 immutable cache，`index.html` 使用 no-cache。

## 3. 身份、租户与授权

- 浏览器使用 `meridian_session` HttpOnly cookie；所有 cookie 写请求校验 CSRF token。
- CLI/CI 使用 Bearer PAT；只存 token hash，明文只在创建时返回一次。
- 未认证固定返回 401 `unauthenticated`。已认证但无权和资源不存在统一返回 404 `not_found`，避免资源探测。
- 请求先解析 tenant slug，再校验 active membership，然后查询时强制带 `tenant_id`。不得先查资源再做租户过滤。
- platform admin 只操作控制面；若没有同租户有效成员身份，不得读取租户业务正文。
- capability 由服务端计算并随资源返回。前端隐藏按钮只是体验优化，不能代替后端鉴权。
- 审计中凭证、PAT、webhook secret、仓库内正文、prompt 和生成内容都只记录标识、摘要与 hash。

公开读取使用独立 `/api/v1/public/...` 路径，只允许 public Service 的 current published merged 内容与视图，不暴露 layer、revision、provenance、仓库、搜索和非 current 版本。

## 4. 聚合与分支模型

核心层次为：

```text
Tenant -> Repository -> Service -> Asset -> AssetRefTrack -> AssetVersion
                                  Asset -> Layer -> LayerRevision
                                              \-> LayerHead(scope)
```

### 4.1 Asset 与 Track

Asset 是 `(tenant, service, kind, name)` 的稳定逻辑槽位。分支或 tag 上的版本指针属于 `AssetRefTrack`，键为 `(asset_id, ref_type, ref_name)`：

- `latestVersionId` 是该 Track 最近一次完整 index 成功的版本；
- `currentVersionId` 是该 Track 当前选中的 published 版本；
- health、generation、SemVer 序列也按 Track 隔离；
- 删除或移动 Git ref 不删除历史，Track 只标记 inactive。

未显式给 ref 时解析仓库默认分支。禁止在 Asset 表上设置全局 `current_version_id`，否则 release 同步会覆盖 main。

### 4.2 SourceSpec 与 SourceBinding

用户配置的是 SourceSpec。非 manual 创建只持久化配置，不读仓库；sync/produce 入队时固定 ref 与 commit，执行后按 `(sourceSpecId, scopeType, scopeKey, expansionKey)` 物化 SourceBinding，再关联 Asset 与 Layer。manual 创建例外：请求必须带已有 `targetAssetId`，服务端在同一事务内创建 SourceSpec、一个 global SourceBinding 和对应 Layer，并在响应返回 `initialLayerId`；调用方随后用该 ID 创建 manual LayerRevision。文件消失时 binding 保留最后一次非空 `resolvedPath` 并标 stale，不删除历史；仅无仓库文件的 push/manual binding 可让该字段为 null。重命名产生新 binding，旧 binding 保留。字段、默认超时、合法 mode/role/origin 组合及字段互斥以 `domain.yaml` 为准。

每个 Asset 至多一个未删除的 base Layer，可有多个 overlay。AI 冷启动在没有有效 base 时创建 AI base；后来接入 repo base 时，用户必须显式提交 `replaceAiBase=true`，服务端在单事务内归档旧 AI source/layer 并创建 repo base。AI 历史修订继续可读，但不会被错误当作 overlay 应用；任何时刻都不能出现两个可用 base。

### 4.3 Service 生命周期

Service 创建为 draft，只有 `draft -> published -> deprecated -> retired`，同状态 patch 不重复产生事件。draft/published/deprecated 允许按权限采集和维护；public 服务只有 published/deprecated 可匿名访问。published 进入 deprecated 时，Service 状态、审计与 `service.deprecated` outbox event 在一个事务提交。

retired 是终态：鉴权后的历史读取、自助通知/todo 和软删除仍可用，其余 Service/Source/Layer/Version 写入返回 409 `invalid_state`。退役事务取消该服务 pending 工作；运行中工作在提交 revision/version/index 前重读生命周期并拒绝落库。仓库级任务跳过 retired 服务，不影响同仓库其他可采集服务。软删除则按 `domain.yaml.serviceLifecycle.deletion` 在单事务归档关联配置和资产、清理派生关系并提升 Track generation；历史物理行保留但任何详情、版本和 public 路由均返回 404。

### 4.4 LayerRevision 与 LayerHead

修订是不可变提交事件，审批结果由 `reviewStatus` 表示；是否参与合并只由 LayerHead 的 `effectiveRevisionId` 表示。二者不可混用：

- repo/manual 为 `not_required`；AI/third-party 默认 `pending_review`；信任模式可直接 `approved`；
- pending 只更新 latest/candidate，不进入 effective 和 manifest；
- 新 candidate 会 supersede 旧 candidate；审批旧 candidate 返回 409；
- approve 原子更新 review、effective、candidate 和 generation，并投递 merge；
- reject 只清 candidate，旧 effective 继续生效；
- rollback 只移动 effective 到同 scope 的 `not_required|approved` 历史修订，不创建修订；
- 与当前 effective/candidate 内容相同才返回 unchanged。与 rejected 或历史内容相同仍可产生新的 review 事件，但复用 blob。

repo/command/AI 默认使用精确 ref scope，manual/push 默认 global。合并先找 exact head，缺失才继承 global。global 改变只 fanout 给仍在继承 global 的活跃 Track。

## 5. 采集、合并与版本

### 5.1 流水线

统一阶段为 `resolve -> discover -> extract -> merge -> normalize -> index`：

1. resolve 固定 repo、commit、ref 和 SourceBinding；
2. discover 产生候选，不自动接管业务配置；
3. extract 在独立临时目录运行 producer，校验 completion manifest；
4. merge 读取一次事务快照中的 effective heads；
5. normalize 调用 kind plugin 产出 canonical blob、items、provenance 和 diff 输入；
6. index 在一个事务中写 version、manifest、items、Track heads、todo 和 outbox。

单 source 失败时保留该 source 的旧 effective revision，其他 source 可继续，根任务结束为 `succeeded_with_warnings`，Track 标 stale 并记录混合 commit manifest。没有有效 base 时不创建 AssetVersion；空骨架只供 AI 输入和 preview。

### 5.2 Overlay 与确定性

合并规则来自 `domain.yaml` 和 kind plugin：base 后按 `ord` 应用 overlay；重复顺序直接拒绝；JSONPath 只支持冻结子集；数组删除按反向文档序；canonicalization 使用 RFC 8785。provenance 以 JSON Pointer 记录最终写入该字段的 layer/revision。

`libopenapi` 负责 OpenAPI 3.x 解析、引用解析、AST/索引和 `what-changed` 变化事实；Meridian kind plugin 负责把变化事实映射为产品等级、规则集和 todo。`json-patch/v5` 只负责已编译 RFC patch 的确定性执行，不负责 JSONPath 目标选择、Layer 顺序、scope 继承或 provenance；这些仍由 Meridian overlay compiler 负责。所有 parser/patch 选项必须显式设置，禁止依赖库升级后变化的默认值。

合并产物的 build fingerprint 包含有序 revision manifest、merge engine、overlay compiler、normalizer/plugin 版本和合并模式。只在 fingerprint 与同 Track latest 完全相同时 no-op。

### 5.3 版本与历史回退

- 每个 Track 首版为 `1.0.0`；breaking 升 major，仅新增升 minor，其余有效输入变化升 patch；
- 手工版本号只允许首次 publish 设置，且必须大于该 Track 所有既有 SemVer；
- A -> B -> A 必须产生第三个版本，允许 merged hash 与第一个相同；
- 唯一约束是 Track+sequence、Track+version 和 mergeRequestId，merged hash 只建索引；
- publish 只移动 current，不修改旧 published 状态；
- 生命周期只能 `draft -> published -> deprecated -> retired`；deprecated/retired 当前版本时回退到最近 published，无候选则 current 为空；
- publish 动态检查 manifest 审批状态、enabled scope 的 candidate、index 完整性和资源生命周期。

自动 diff 基线：同 Track 用 previous latest；非默认分支首版用同步开始时的默认分支 current，否则 default latest；默认分支首版无基线。基线 ID必须写入版本，后续不得随 head 漂移。

## 6. GitOps、命令与 AI

`.asset-platform.yaml` 必须通过 `repository-config.schema.yaml` 校验。数据库是运行时权威源，配置来源按 JSON Pointer 保存：

- `repo_file` / `repo_bootstrap` 字段可被后续 sync 覆盖；
- 页面人工编辑字段立即变成 `db_manual`，仓库不得静默覆盖；
- preview 生成 `previewId + commit + configDigest`，apply 必须校验三者未漂移；
- keep_db 保留数据库并记录人工来源；take_file 写入仓库值；ignore 绑定当前 file digest，仓库变化后漂移重新出现。

command/AI 只允许平台管理员通过 `/api/v1/admin/producer-profiles` 维护的 producer profile，不接受业务请求传任意 shell。租户只能读取 `/api/v1/t/{tenantSlug}/producer-profiles` 返回的 enabled/available 安全元数据并按 ID 选择。AI/API 密钥由部署环境或 secret file 提供，profile 只能按名称选取部署白名单的环境变量；值不进入 API、数据库、审计或日志。worker 使用非 root UID 和镜像内固定路径 `/usr/bin/bwrap` 的 bubblewrap、只读仓库、独立空输出目录、默认断网和资源限额；bwrap 缺失时 profile 标记 unavailable，禁止降级为裸进程。每次执行写 completion manifest；缺 marker 时仅 `replay_safe` profile 可在新目录重试，否则结束为 `outcome_unknown`。

平台配置优先级固定为启动参数 > 环境变量 > 配置文件 > 编译默认值；数据库运行时配置只覆盖契约明确列出的动态项，不得覆盖数据库连接、master key、监听地址或 sandbox 安全边界。敏感值只允许启动参数引用的 secret file 或环境变量注入。

AI 冷启动是服务级命令，因为此时 Asset 尚不存在。请求包含 kind/name/ref/profile，不包含 shell。producer 输出先校验再创建 LayerRevision；`producerRunId` 保证任务重试 exactly-once。信任模式只跳过审批，不自动发布。

## 7. 持久化与事务

表、字段、唯一键、外键和索引以 `storage.yaml` 为准。实现必须遵守以下事务边界：

- approve/reject/rollback：锁 LayerHead 后 CAS candidate/effective；
- version index：blob 已就绪后，在一个数据库事务内写 version、manifest、items、Track heads、breaking todo 和 outbox；
- share link、PAT、credential secret 只存 hash/密文；
- outbox 与业务变更同事务写入，dispatcher 至少一次投递；消费者按 eventId 去重；
- 软删除资源不级联删除历史版本或审计；
- retention 只能删除未被 head、version manifest、audit 引用的 rejected/superseded/已删除层修订。

数据库迁移随二进制以 `//go:embed` 打包，并通过 goose v3 provider 执行。进程启动后先获取一个 session 级 PostgreSQL advisory lock，再把 River schema 升到该应用版本记录的精确 target，随后执行应用 `up` migration；全部成功后才开放 HTTP readiness 和启动 worker。任一步失败即退出，禁止带半迁移 schema 提供服务。启动流程永不执行 `down`；回退只允许运维显式运行 `meridian migrate down --steps N`，并且必须先通过备份恢复演练。默认每个应用 migration 在单事务内完成；并发索引等无法事务执行的 DDL 必须显式标注 no-transaction，并附单独决策记录和失败恢复步骤。

业务写入与 River job 插入使用同一个 `pgx.Tx`；outbox 行也在该事务写入。River worker 的 schema migration 不代替业务 outbox：外部 webhook、通知等仍从 outbox 至少一次投递，而内部异步执行由 River 接管。

MVP blob 实现固定为 `domain.yaml#/storage/blobStore`：`MERIDIAN_BLOB_ROOT` 指向持久卷，key 只由 SHA-256 digest 推导。写入先在同一文件系统流式计算摘要并写临时文件、执行 `fsync`，再用不替换目标的 hard-link 原子发布；若目标已存在或由并发进程率先发布，必须重新校验目标大小和摘要后丢弃临时文件，绝不覆盖已有 CAS 对象。数据库只保存 immutable blob key、hash、size 和 media type。上传校验 MIME、大小和解析结果；应用签发的下载 token 最长 300 秒并绑定 blob/制品类型，分享下载还绑定有效 shareLinkId，分享 token 不能换取其它资源内容。S3-compatible driver 不属于 v1。

## 8. HTTP 与生成代码

所有 HTTP operation 均定义在 `openapi.yaml`。实现流程：

1. lint OpenAPI 3.1、解析全部 `$ref`、检查 operationId 唯一；
2. 对 common 与各 API 领域分别运行 `oapi-codegen`，生成领域 server interface/models/spec；
3. 各领域 transport adapter 实现本领域生成接口，业务层不得直接使用框架 request；
4. CI 重新生成并断言工作区无差异；
5. 以契约中的 examples 和 `acceptance.yaml` 生成 contract tests。

全局错误体为 `{code,message,details?,requestId}`。列表使用 `page/pageSize`，排序使用 `sort/order`。资源写操作使用 `If-Match` 或显式 expected head；可重试副作用使用 `Idempotency-Key`。内容超限返回 413 `content_too_large`，语义校验返回 422。

长任务统一返回 202 JobAccepted。数据库状态是事实源；前端同时使用 Job 状态查询和持久化日志 SSE。SSE 必须支持状态快照、持久 sequence、`Last-Event-ID` 断线重放、15 秒 heartbeat 和终态后关闭；断线时先按游标重连，无法恢复时退化为状态轮询。

## 9. 任务、并发与恢复

- repo sync 的业务去重键是 repository+ref；同一 repository 另有互斥锁，避免不同分支并发操作同一工作区；
- asset merge 的 desired/processed generation 持久化。运行期间再次变更只置 dirty/提升 generation，当前任务结束后循环或投递 successor，直到追平；
- 任务有 attempt、maxAttempts、nextAttemptAt 和结构化 error；retry 只对契约允许的状态开放；
- worker 用数据库租约与 heartbeat。启动时回收过期租约；
- normalize/index 失败不能暴露半成品版本；失败重试沿用 mergeRequestId；
- external producer 遵守第 6 章 completion marker 规则，不允许对未知副作用盲重放；
- 所有 job 的 deduplicated 响应都返回现有 jobId，调用方可以继续观察同一任务。

## 10. Kind、View、搜索与通知

Kind plugin 的 normalize/item/diff 接口和 built-in kind 规则在 `kinds.yaml`。首发先实现 openapi，M4-M5 再启用 asyncapi/dbschema/dependency；禁用 kind 不删除历史。

View resolve 只接受 `views.yaml` 中的 InputDescriptor。后端校验 view、kind、scope 和版本访问权，返回短时签名内容 URL 或结构化数据。第三方 iframe 资源同源托管但使用严格 CSP 与 sandbox，不开放任意网络代理。

索引行必须带 tenant/service/asset/version/itemKey，查询第一条件是 tenant。高亮返回结构化片段，不返回服务端拼接 HTML。深链固定 assetId/versionId/itemKey，不能依赖 latest。

breaking todo 唯一键为 `(assetVersionId, assigneeUserId)`。多个用户 owner 各一条。`version.published` 与 `version.breaking` 是两个独立事件和通知；webhook 至少一次投递，以 eventId 去重，签名和重试见 `events.yaml`。

## 11. 实施顺序

| 里程碑 | 后端交付 |
| --- | --- |
| M0 | 契约校验/生成、数据库骨架、auth/tenant/RBAC、blob/job/audit/outbox |
| M1 | repository/service/source、discover/sync、默认分支 Track、openapi normalize/index、public read |
| M2 | LayerHead、manual overlay、provenance、rollback、字段级 GitOps |
| M3 | AI producer/review、多 branch/tag Track UI、version lifecycle、diff/snapshot/share、breaking todo、CLI push/diff、最小通知 |
| M4 | GitOps 完整漂移、dbschema/dependency、group/search/global views |
| M5 | 通用订阅/inbox/webhook、asyncapi、合规与运维加固 |

每个里程碑只在 `acceptance.yaml` 映射的 AC、Smoke 和负向 API 用例全部通过后完成。测试 fixture 必须按场景独立建立，不允许依赖上一用户故事的残留状态。

## 12. 测试与发布门禁

最低门禁：

- YAML 可解析，manifest 引用完整，OpenAPI lint 和 codegen 无漂移；
- domain 单测覆盖状态迁移、审批 CAS、分支 head、A-B-A、candidate publish blocker；
- repository 测试覆盖 tenant 条件、唯一键、事务回滚和 outbox；
- pipeline 集成测试覆盖单源失败、worker crash、dirty successor、未知 producer 结果；
- kind golden tests 覆盖 deterministic normalize、overlay 和 diff；
- API contract tests 覆盖 401/404/409/412/413/422 及 requestId；
- E2E/Smoke 逐条执行 `acceptance.yaml`，包含 public/share/PAT 撤销的安全反例；
- 发布前做 migration expand/contract 演练、`pg_dump` + blob 持久卷联合备份恢复、密钥轮换和 blob GC 检查。

## 13. 术语

- **Asset**：服务下某 kind/name 的稳定逻辑槽位。
- **AssetRefTrack**：Asset 在 branch/tag 上独立的版本序列与 head。
- **SourceSpec**：用户配置的采集规则。
- **SourceBinding**：SourceSpec 对实际文件展开后的物化绑定。
- **LayerRevision**：不可变的内容提交和审批记录。
- **LayerHead**：某 Layer/scope 的 latest、candidate、effective 指针。
- **effective**：当前参与合并；与审批状态不是同一概念。
- **AssetVersion**：某 Track 上一次完整、不可变的 index 结果。
- **build fingerprint**：决定是否需要新版本的完整输入摘要。
- **current**：某 Track 当前选中的 published 版本。
- **latest**：某 Track 最近创建且 index 完成的版本。
