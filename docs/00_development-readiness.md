# Meridian 开发就绪结论

> 审查日期：2026-09-05
> 结论：技术栈已完成评估并冻结；业务契约中的上一轮阻塞点已补齐，可开始 M0 编码。机器入口为 [contracts/manifest.yaml](../contracts/manifest.yaml)，长程自动开发入口为 [contracts/work-items.yaml](../contracts/work-items.yaml) 和 [07_coding-agent-runbook.md](./07_coding-agent-runbook.md)。

> 实施状态：M0 正在开发，尚未宣告完成。契约生成、静态单二进制、数据库/River 迁移、强类型 sqlc 仓储以及身份/租户/PAT 主链路已经通过自动化门禁；凭据、仓库连接、job/audit/outbox 和 M0 控制面仍按 `acceptance.yaml` 推进。文档“可开始编码”表示设计无产品决策阻塞，不表示全部 M0 operation 已实现。

## 技术栈冻结结论

| 领域 | 冻结口径 | 不允许的替代实现 |
| --- | --- | --- |
| 后端与交付 | Go 1.27.1、Chi v5、oapi-codegen v2、pgx/sqlc、River、goose 内嵌 SQL；Nuxt 静态产物由 Go embed | Node/Nitro 运行时、Redis/RabbitMQ、ORM、golang-migrate |
| 前端框架 | Nuxt 4 + Vue 3 SPA（`ssr:false`），Node 24 LTS + pnpm 11 | 硬切 React/Next 或另起独立 Vite 应用 |
| API 客户端 | Orval 8 `vue-query` + Fetch + 单一 custom fetcher | openapi-fetch、Axios、手写 DTO/CRUD SDK |
| UI 与状态 | Nuxt UI 4 + Tailwind 4；TanStack Vue Query 5 管 server state；Pinia 3 只管 UI state | Naive UI/UnoCSS 并行组合、Redux/Zustand 保存 API 实体 |
| 资产文本 | CodeMirror 6 + `@codemirror/merge`；1 MiB 可编辑，5 MiB+ Worker 只读 | Monaco 作为默认编辑器 |
| 依赖图 | Cytoscape.js 3 canvas；500 节点/2000 默认边、5000 硬上限、超限服务端邻域 | React Flow/Vue Flow/Sigma 双引擎 |
| 校验与本地化 | Ajv 8 校验动态 JSON Schema，Zod 4 校验静态表单，Nuxt i18n 10 提供 zh-CN/en | 重复 OpenAPI DTO 或公共 CDN 图标/运行时依赖 |

Go 泛型方法只用于具体 helper 类型的类型保持/变换；不得放进 domain、repository、provider 或生成 transport interface。所有范围和职责见技术决策 ADR，冲突时 manifest/YAML 优先。

## 已关闭的原始模糊点

| 原问题 | 冻结口径 | 权威位置 |
| --- | --- | --- |
| Asset 的 current/latest 是否跨分支 | 否；每个 branch/tag 使用独立 AssetRefTrack | `domain.yaml.assetRefTracks` |
| Layer 修订“已审批”和“当前”混在一个 status | reviewStatus 与 LayerHead 分离；只有 effective 参与合并 | `domain.yaml.layerHeads` |
| 连续 pending 谁可审批 | 每 scope 一个 candidate，新 candidate supersede 旧 candidate | `domain.yaml.layerHeads` |
| pending 是否可能进入版本 | 永不；无有效 base 时也不创建版本 | `domain.yaml.versions` |
| 旧 draft 能否绕过新 pending 发布 | 不能；enabled applicable candidate 动态阻止 publish | `domain.yaml.versions.publish` |
| rollback 是否建 Revision、同 hash 是否建 Version | rollback 不建 Revision；返回历史输入必须建新 Version，hash 可复用 | `domain.yaml.versions` |
| SemVer 从何开始、手工版本何时允许 | Track 首版 1.0.0；手工值只在首次 publish，且大于已有版本 | `domain.yaml.versions` |
| release 首版如何判断 breaking | 默认分支 current，否则 latest，固定写 baselineVersionId | `domain.yaml.versions.comparisonBase` |
| glob 一个配置如何对应多个资产 | SourceSpec 展开为稳定 SourceBinding | `domain.yaml.sourceExpansion` |
| 资产源在领域、HTTP、数据库的名称不一致 | 全部统一为 SourceSpec；展开结果统一为 SourceBinding | `openapi.yaml`、`domain.yaml.sourceExpansion`、`storage.yaml` |
| 配置导入谁覆盖谁 | DB 是权威；仓库文件仅作导入来源，preview/apply 两阶段，不做字段级来源与漂移 | `domain.yaml.configAuthority` |
| 无 Asset 时 AI 从哪里启动 | Service 级冷启动 operation，创建 Asset/Layer/candidate | `openapi.yaml`、`domain.yaml.producers` |
| command 是否能执行请求方 shell | 不能；只能选择平台受控 producer profile | `domain.yaml.producers` |
| worker 崩溃后外部命令是否重放 | completion manifest 优先；仅 replay-safe 可在新目录重试，否则 outcome_unknown | `domain.yaml.producers` |
| sync/merge 并发如何既去重又不漏更新 | repo+ref 去重、repository 工作区互斥、Track generation dirty successor | `domain.yaml.jobs` |
| 单 source 失败的根任务状态 | 其它源可成功，根任务 succeeded_with_warnings，历史版本继续可读 | `domain.yaml.jobs/health` |
| 分享链接是否可访问其它文档 | 不可；token 仅访问冻结 descriptor 的 artifact allowlist | `domain.yaml.sharing`、`openapi.yaml` |
| global credential 如何管理和解绑 | 仅平台管理员维护；租户可选；强制删除原子解绑并将仓库标为需认证 | `domain.yaml.credentials`、`openapi.yaml` |
| 凭据轮换后的仓库是否、何时重新采集 | 请求字段 `resyncRepositories` 决定；true 时密钥更新后原子投递每个活跃引用仓库一次，同幂等键重放原结果 | `domain.yaml.credentials.rotation`、`openapi.yaml` |
| 全局凭据轮换返回的 job 如何关联 | 每项固定返回 tenantSlug、repositoryId、jobId、deduplicated，按 tenantSlug/repositoryId 排序 | `openapi.yaml.CredentialSyncJob`、`domain.yaml.credentials.rotation` |
| SSH known host 的主键和返回字段 | 候选/记录统一使用 host、port、keyType、fingerprint；记录额外含 manual/accept_new 来源，publicKey 只在候选中返回 | `openapi.yaml`、`storage.yaml.known_hosts` |
| KnownHost 手工字段是否可信 | 客户端只交 host/port/publicKey；服务端解析 RFC4253 keyType 并重算 SHA256 fingerprint；非法 422、重复 409 | `domain.yaml.knownHosts`、`openapi.yaml` |
| SSH/HTTP 凭据 fingerprint 如何计算 | SSH 按 OpenSSH 公钥 SHA256；HTTP 用独立稳定 32-byte key 对长度前缀 username+token 做 HMAC-SHA256；相同 fingerprint 的轮换被拒绝 | `domain.yaml.credentials.rotation.fingerprint`、`openapi.yaml.CredentialFingerprint` |
| producer profile 谁配置、租户能提交什么 | 平台管理员维护绝对路径/参数/资源限制；租户只能选择可用 profile，不能提交命令 | `domain.yaml.producers`、`openapi.yaml` |
| blob 用什么实现、如何写入和下载 | v1 固定为本地持久卷的 SHA-256 content-addressed store；原子写；HMAC 下载 token 最长 300 秒；S3 不进 v1 | `domain.yaml.storage.blobStore` |
| 并发更新令牌从哪里取得 | 所有受 `If-Match` 保护的资源 DTO（包括列表项）携带 opaque `etag`，详情响应头与 body 相同 | `domain.yaml.optimisticConcurrency`、`openapi.yaml` |
| 可重试操作哪些强制幂等 | 22 个 operationId 逐项列入领域契约，必须在 OpenAPI 声明 required 并接收 UUID `Idempotency-Key` | `domain.yaml.wire.idempotency.requiredOperationIds`、`openapi.yaml` |
| 幂等请求 hash 与首次并发如何处理 | SHA-256(JCS 规范对象)，固定 path/query/If-Match/body 范围；事务 advisory lock 串行，同 hash 等待并重放，不同 hash 等待后 409 | `domain.yaml.wire.idempotency.requestDigest/concurrentFirstRequest` |
| 平台默认设置如何影响租户 | 作为创建租户时的初始化模板原子复制；修改不追溯现有租户；平台默认通知仅允许无密钥站内信 | `domain.yaml.platformDefaults`、`openapi.yaml` |
| breaking 通知是一条还是两条 | `version.published` 与 `version.breaking` 两个事件、两条通知 | `events.yaml` |
| viewer 是否禁止全部写操作 | 否；订阅、通知已读、自己的 todo ack 是合法 self-service 写 | `domain.yaml.authorization` |
| 撤销 PAT 后返回 401 还是 404 | 401 `unauthenticated`；已认证无权/不存在才是 404 | `domain.yaml.authentication` |
| fixture 是否跨故事共享 | 不共享；bootstrap-only 与 standard 分开，每个故事独立 setup | `acceptance.yaml.fixtures` |
| Service 和 Asset 是否共享生命周期记录 | 不共享；Service 自身使用生命周期，AssetVersion 按 Track 独立流转，Asset 只投影 current/latest 状态 | `domain.yaml.stateMachines.lifecycle`、`domain.yaml.assetRefTracks` |
| Service 各生命周期能做什么 | draft/published/deprecated 可采集维护；公开读取只允许 published/deprecated；deprecated 原子发事件；retired 历史只读并以 generation fence 阻止运行中任务落库 | `domain.yaml.serviceLifecycle`、`acceptance.yaml` |
| 删除 Service 具体影响哪些数据 | 事务软删 Service/Source/Layer/Asset，Binding stale、Track inactive，清关系和 pending job；历史物理行保留但外部统一 404 | `domain.yaml.serviceLifecycle.deletion`、`storage.yaml.transactionBoundaries.deleteService` |
| 仓库根服务在 YAML/API/DB 如何表达 | `.asset-platform.yaml` 使用精确 `.`；preview 前规范化为空串，API/DB 始终用空 rootDir | `domain.yaml.configAuthority.serviceRoot`、`repository-config.schema.yaml` |
| Source mode 的字段与超时如何确定 | 每种 mode 固定 origin、必填/禁止 path/profile；SourceSpec 省略超时按 mode 为 120/600 秒，profile 省略超时按 command/ai 为 300/600 秒，执行取 source/profile 较小值 | `domain.yaml.sourceCompatibility`、`domain.yaml.producers.profiles`、`openapi.yaml`、`repository-config.schema.yaml` |
| manual SourceSpec 如何落到可编辑 Layer | 请求必须带已有 `targetAssetId`；服务端原子创建 SourceSpec、global Binding、Layer 并返回 `initialLayerId`，随后才能提交 Revision | `domain.yaml.sourceExpansion`、`openapi.yaml.SourceSpecInput` |
| Job scope 如何表达 | 固定枚举 tenant/repository/service/source/track/version/asset/diff/system；scopeId 与 ref 字段按 type 约束，system 无 scopeId | `domain.yaml.jobs`、`openapi.yaml.JobScopeType` |
| AssetVersion 生命周期写入如何并发保护 | publish/deprecate/retire 必须携带该版本 opaque ETag/If-Match；响应 body/header 同步返回新 ETag | `domain.yaml.optimisticConcurrency`、`openapi.yaml` |
| SystemGroup 层级和字段 | parentId 明确返回/可写；最大深度 1；跨租户、环、自父级统一 422 `nesting_too_deep`；删除只解除关系 | `domain.yaml.systemGroups`、`openapi.yaml`、`storage.yaml` |
| 搜索筛选闭环 | repository/team/group/kind/tag/lifecycle/language/itemType/AI 层/breaking 变更均为 SearchFilter；SearchHit 支持 repository/service/asset/item 且 deep link 固定版本 | `openapi.yaml.SearchFilter`、`01_requirement-specification.md.F8` |
| M0 Smoke 是否调用后续能力 | M0 只跑基础认证、隔离、凭据、仓库骨架和迁移；订阅/todo/完整搜索/图/AI 等按 operation milestone 执行 | `acceptance.yaml`、`openapi.yaml.x-milestone` |
| glob 文件消失是否清空绑定路径 | 不清空；保留最后一次非空 resolvedPath 并标 stale；仅 push/manual 无仓库文件时可为 null | `domain.yaml.sourceExpansion`、`openapi.yaml` |
| 最近浏览如何记录和排序 | 仅成功的服务详情读取按 user+service upsert；失败/无权不更新；按 viewedAt 降序、serviceId 升序 | `domain.yaml.repository.recentServices`、`acceptance.yaml` |
| MVP 是否包含分享/todo/最小通知/CLI | 包含，全部在 M3；完整协作和多 kind 后移 M4-M5 | `domain.yaml.releaseScope`、`acceptance.yaml` |

## 开发入口

1. M0 首先实现 YAML lint、OpenAPI/JSON Schema 校验、交叉引用检查、Go/Orval 代码生成和静态 SPA embed；
2. 按 `storage.yaml` 建 migration，按 `openapi.yaml` 生成 transport 类型；
3. 依 `domain.yaml` 写纯领域状态与属性测试，再连接 repository 和 worker；
4. 前端只消费生成 client、`views.yaml` 和 capability；
5. 每个里程碑以 `acceptance.yaml` 对应 story/smoke 为放行门禁；M0 先通过 [05_technology-stack-decision.md](./05_technology-stack-decision.md) 的 6 个 executable spike。

执行环境必须满足 `contracts/manifest.yaml`：Go `>=1.27.1,<1.28`、Node `>=24.20.0,<25`、pnpm `>=11,<12`。仓库已提交 `.vfox.toml`，M0 构建统一通过 vfox 使用 Go 1.27.1、Node 24.20.0 和 pnpm 11.20.0，不依赖调用终端原有的全局版本。

## 自动开发边界

业务和架构选择不得由 Agent 临时猜测；若契约未覆盖，工作项必须进入 `blocked`。依赖 patch 只能在 manifest 范围内升级，并通过 ADR 第 8 节门禁。移动端视觉精修、部署域名和生产密钥可以按运行手册后置，但不能改变功能、安全和可访问性验收。`package.json`、`pnpm-lock.yaml`、`go.mod` 和 `go.sum` 必须纳入版本控制，不能以未锁定的 `latest` 作为实现依据。
