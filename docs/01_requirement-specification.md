# Meridian · 需求规格说明书

> 版本：v1.1（开发基线，2026-09-04）
> 技术栈：Go 1.27.1（后端）+ Nuxt 4/Vue 3 SPA（前端）+ PostgreSQL 16+（存储）；完整冻结基线见 [05_technology-stack-decision.md](./05_technology-stack-decision.md)
> 定位：**多租户系统资产目录 + 多源 Overlay 合并引擎 + 插件化多视图门户**
> 共同契约入口：[contracts/manifest.yaml](../contracts/manifest.yaml)。本文件定义产品意图；接口、枚举、状态机、默认值、存储约束和验收映射以该清单引用的 TypeSpec/OpenAPI 与 YAML 契约为唯一开发口径。

**交付边界已冻结**：`M0-M3` 为 MVP，`M4-M5` 为 v1 扩展，`P2/M6+` 不进入 v1。为保证核心用户故事闭环，AI 待审站内通知、Diff 快照分享、breaking 待办以及 `meridian push/diff` 已纳入 M3；不再受原功能章节的 P1 标签限制。

---

## 1. 产品定位

### 1.1 一句话定义

把散落在各 Git 仓库里的**系统资产**（API 契约、事件契约、数据模型、配置、依赖关系、部署描述等），自动收集或由 AI 补全，形成一份**带版本、可叠加（overlay）、可对比、可检索、可分享的资产目录**，按租户隔离，并为每种资产提供可插拔的多视图呈现——从单份资产详情，到多版本 DIFF，再到跨系统的全局关系视图。

### 1.2 要解决的核心问题

| 编号 | 痛点 | 本平台的解法 |
|---|---|---|
| P1 | 系统资产（接口、事件、表结构、配置、依赖）散落在各仓库，没人知道全貌 | 统一 **仓库 → 服务 → 资产** 目录 + 资产类型注册表 + 全文检索 |
| P2 | 文档与代码脱节，改了代码忘了改文档 | 从 Git 仓库**自动采集**，定时 / Webhook 触发，代码即文档 |
| P3 | 仓库里的原始资产不完整或缺失，补全信息又不能污染仓库 | **Overlay 层模型**：人工 / AI / 第三方补充作为独立层叠加，原始层永不被改写 |
| P4 | 存量服务没有任何资产文件，接入成本高 | **AI 生成源**：接入后平台调用 AI 分析代码生成资产，人工审批后发布 |
| P5 | 资产变更无通知，下游被动踩坑 | **结构化 Diff + Breaking 检测 + 订阅通知 + CI 门禁** |
| P6 | 不同角色要看的东西不一样；看单服务容易，看"全公司谁依赖谁"没有工具 | **插件化多视图**（含跨系统范围聚合视图），每个视图声明式定义输入契约 |

### 1.3 租户与角色

**租户（Tenant）是第一级隔离边界。** 一个用户可属于多个租户，在不同租户里可有不同角色；切换租户即切换全部数据视图。

| 层级 | 角色 | 说明 |
|---|---|---|
| 平台级 | `platform_admin` | 跨租户管理：创建租户、管理用户、查看全平台审计日志、配置全局默认（视图、规则集、通知渠道）。**不可见租户业务数据正文**，只可见元数据与审计 |
| 租户级 | `tenant_admin` | 管本租户成员与配额、本租户内全部资源、本租户默认配置 |
| 租户级 | `maintainer` | 录入仓库、配置服务与资产源、编辑人工层、触发采集、发布版本 |
| 租户级 | `viewer` | 只读：浏览、检索、调试、订阅 |
| — | CI Bot | 通过租户绑定的 PAT 操作，仅限 token 作用域内 |

- 服务级授权（`service_grants`）：可把某个服务的 owner/maintainer/viewer 授予具体用户或团队
- 层级权限点：`layer:edit`（编辑人工层）、`layer:approve`（审批 AI / 第三方层修订）；默认 maintainer 有 edit，服务 owner 有 approve
- 权限模型：`租户隔离（强制）+ 租户内 RBAC + 服务级 ReBAC-lite`，预留 ABAC 扩展位

---

## 2. 核心概念（领域模型）

```
Tenant 租户
   └── Repository 仓库          —— Git 地址 + 凭证 + 分支策略
          └── Service 服务      —— 仓库内的逻辑服务（系统的最小纳管单元）
                 └── Asset 资产（kind + name 的身份槽位）
                        ├── SourceSpec 采集规则 1..N → SourceBinding 文件展开结果
                        ├── Layer 层 1..N（base + overlay）
                        │      ├── LayerRevision 不可变修订
                        │      └── LayerHead 按 ref/global 保存 latest/candidate/effective
                        └── AssetRefTrack（branch/tag）
                               └── AssetVersion → AssetItem

AssetKind 资产类型注册表（平台级定义，租户可启停）
ViewDef   视图注册表（声明 ViewInputSpec 输入契约）
SystemGroup 系统分组（跨仓库把多个 Service 组织成"系统"，全局视图的作用域）
```

### 2.1 概念表

| 概念 | 说明 | 关键约束 |
|---|---|---|
| **Tenant** | 隔离单元。含配额、默认配置、成员 | 所有业务表带 `tenant_id` |
| **Repository** | 被纳管的代码仓库。含 URL、凭证引用、默认分支、扫描路径、忽略规则 | 租户内「规范 URL + 默认分支」唯一；实际跟踪 ref 进入 Track |
| **Service** | 仓库内的一个服务单元。含根目录、语言/框架、展示名、负责人、生命周期 | 属于唯一仓库；根目录在仓库内唯一 |
| **AssetKind** | 资产类型注册表条目：id、内容格式、校验器、条目抽取器、diff 器、overlay 方言、默认视图 | 新增 kind = 注册记录 + 实现接口（F14），不改核心表结构 |
| **Asset** | 服务下某类资产的身份：`kind + name`（如 `openapi:admin-api`、`dbschema:orders`） | 服务内 `kind + name` 唯一 |
| **SourceSpec / SourceBinding** | SourceSpec 描述 mode/path/profile 等采集规则；glob 命中后按实际文件物化 SourceBinding，并关联 Asset/Layer | 一个 spec 可展开多个 binding；binding 的稳定键是 scope(ref/global) + 展开路径，分支间状态不互相覆盖 |
| **Layer** | 资产的一个来源层：`origin ∈ {repo, third_party, manual, ai_generated}` × `role ∈ {base, overlay}` + 应用顺序 order | 每个资产至多一个 base 层（可为空占位）；overlay 按 order 依次应用 |
| **LayerRevision / LayerHead** | Revision 是不可变提交和审批事实；Head 按 exact ref 或 global scope 分离 latest、candidate、effective | reviewStatus 不表示当前；只有 effective 参与合并；回滚只移动 Head |
| **AssetRefTrack** | Asset 在 branch/tag 上独立的 latest/current、健康和版本序列 | release 同步不得覆盖 main；缺省解析仓库默认分支 |
| **AssetVersion** | 某 Track 上一次完整合并和索引的不可变快照 | 只与该 Track latest fingerprint 相同才 no-op；返回历史输入仍创建新版本 |
| **AssetItem** | 从合并结果解析出的扁平条目（openapi→operation；dbschema→table/column；dependency→edge），供检索/统计/diff/全局视图 | 由 kind 的 extractor 产出，随 AssetVersion 重建；每条携带 provenance（来自哪个层） |
| **SystemGroup** | 租户内逻辑分组：若干 Service（可跨仓库）归入一个"系统"，支持一级嵌套（域 → 系统） | 范围聚合视图与订阅的作用域单位 |
| **ViewDef** | 视图注册表条目，含 ViewInputSpec（F6） | 插件化注册，不改核心代码 |
| **CollectionJob** | 一次采集任务（拉代码 → 发现服务 → 各源产层 → 合并 → 归一 → 索引 → 通知） | 手动 / 定时 / Webhook 触发 |

**生命周期**：`draft → published → deprecated → retired`。Service 自身与 AssetVersion 都使用该状态枚举；Service 的删除为软删除，AssetVersion 的发布/弃用/退役由 Track 的 current 指针规则处理，二者不共享状态迁移记录。

### 2.2 设计原则（四条铁律）

1. **资产的可读内容永远 = 层合并的结果。** 即使只有一个 repo base 层、零 overlay，也走同一条合并管道（退化为恒等变换）。心智模型唯一。
2. **层与版本解耦。** 层修订记录"来源真相"；AssetVersion 记录"某一刻对外呈现的合并快照"。"这个字段的描述是谁写的"由 provenance 回答；"上周文档长什么样"由 AssetVersion 回答。
3. **AI 是一种 origin，不是特殊机制。** AI 生成走与人工编辑完全相同的层 / 修订 / 审批管道，差别仅在触发方式与默认状态（待审、不自动发布）。
4. **视图不理解存储，只理解输入契约。** 后端把"取哪些版本 / 条目"按视图声明的 ViewInputSpec 解析好喂给视图。

### 2.3 内置 AssetKind

| kind | 内容格式 | base 层典型来源 | item 类型 | overlay 方言 | 优先级 |
|---|---|---|---|---|---|
| `openapi` | OpenAPI 3.x / Swagger 2（归一为 3.1 存储） | 仓库 glob 路径 | operation | OpenAPI Overlay Spec 1.0 + 通用 Overlay | **P0（标杆实现）** |
| `asyncapi` | AsyncAPI 2/3 | 仓库 glob 路径 | channel / message | 通用 Overlay | P1 |
| `dbschema` | 平台定义 JSON（表/列/索引/外键） | command（如 `atlas inspect`）/ push | table / column | 通用 Overlay | P1 |
| `dependency` | 平台定义 JSON（服务→服务/中间件的边） | command / push / manual | edge | 通用 Overlay | P1（全局视图依赖它） |
| `config` | JSON Schema 描述的配置项清单 | 仓库路径 / command | config_item | 通用 Overlay | P2 |
| `deployment` | 平台定义 JSON（环境/实例/端口） | push / manual | endpoint_binding | 通用 Overlay | P2 |
| `markdown_doc` | Markdown 文档集 | 仓库 glob | doc | 不支持 overlay（整篇替换） | P2 |

> `proto` / `graphql` 等按需通过「command 产出 openapi」或注册新 kind 接入，不占 v1 范围。

---

## 3. 功能需求

> 重要性标签：P0/P1/P2 仅用于描述产品重要性；交付顺序以 `contracts/work-items.yaml` 的 milestone 和依赖为唯一准则。任何未绑定工作项、operation、story 和 assertion 的 P0/P1 条目不得被标记为完成。

### F1 身份、租户与权限（P0）

- F1.1 本地账号密码登录（argon2id 哈希）+ 服务端 session（浏览器只持有 HttpOnly `meridian_session` Cookie）
- F1.2 预留 OIDC / LDAP 接入点（接口层抽象，v1 只实现 local provider）
- F1.3 租户：`platform_admin` 可创建/停用租户；租户含 slug、展示名、配额、默认配置
- F1.4 租户成员：`tenant_members`（user × tenant × role）；一个用户可属多个租户
- F1.5 租户切换：顶栏切换器，切换后所有数据视图与 URL 前缀随之变化（`/t/{tenantSlug}/...`）
- F1.6 **强制隔离**：所有查询必须带 `tenant_id`，取不到即报错，绝不降级为"查全部"
- F1.7 服务级授权：`service_grants`，支持在单个服务上向用户或团队授予 owner / maintainer / viewer
- F1.8 可见性：服务可设 `private`（仅授权可见）/ `internal`（租户内登录可见）/ `public`（匿名可见）
- F1.9 API Key / PAT：v1 仅绑定租户；平台级 bot token 不在 v1。支持作用域（`asset:push`、`asset:read`、`job:run`）、过期时间、最后使用时间、可撤销
- F1.10 租户配额（不做计费）：`max_repositories`、`max_services`、`max_storage_bytes`、采集并发上限；超限明确提示而非静默失败
- F1.11 审计日志：谁在什么时候对什么做了什么，带 `tenant_id`；覆盖层操作（创建/编辑/发布/回滚/审批）；`platform_admin` 可跨租户查，租户管理员只能查本租户
- F1.12 权限点：`layer:edit`、`layer:approve`、`assetkind:manage`（租户级启停资产类型）、`systemgroup:manage`

### F2 Git 凭证管理（P0）

- F2.1 持久化凭证类型：`ssh_key`（ed25519 优先，兼容 RSA，可选 passphrase）/ `http_token`（用户名 + PAT）；公开仓库在 Repository 上使用空 `credentialId`，不创建 `none` 凭证记录
- F2.2 加密存储：主密钥（`MASTER_KEY`）→ HKDF 派生每凭证子密钥 → AES-256-GCM；DB 只存密文
- F2.3 掩码展示：只展示指纹、类型、创建人、最后使用时间；SSH 指纹按公钥 RFC4253 blob 的 OpenSSH SHA256 规则生成，HTTP token 用独立稳定密钥做 HMAC-SHA256；用户提交的私钥/token 永不回显；平台生成的 PAT 仅创建响应显示一次
- F2.4 归属与共享：凭证归属租户，默认创建者私有，可显式共享给租户内团队/全体；`platform_admin` 可标记 `global` 凭证供所有租户使用
- F2.5 known_hosts：SSH 首次连接记录 host key 指纹，可配 UI 文案 `accept-new`（线上枚举 `accept_new`）/ 手动录入；手工接口只接收 host/port/publicKey，服务端从 RFC4253 公钥派生 keyType 与 fingerprint，非法公钥为 422、重复为 409
- F2.6 连通性测试：保存前后支持 `git ls-remote` 探活，返回可读的错误分类
- F2.7 凭证轮换：更新凭证后可选自动触发引用它的仓库重新采集
- F2.8 删除前检查引用计数（被仓库引用时阻止或提示强制解绑）

### F3 仓库管理（P0）

- F3.1 录入仓库：URL（支持 SSH 与 HTTPS 两种格式）+ 凭证 + 默认分支 + 备注
- F3.2 高级配置：扫描路径白名单 / 忽略规则（`.gitignore` 语法）；分支策略（跟踪 `main`、`release/*`、tag 前缀）；拉取策略（shallow / full clone、本地缓存目录）；子模块；HTTP(S) 代理
- F3.3 **服务发现**（monorepo 关键）：
  - 自动探测：按约定文件识别服务根（`go.mod` / `pom.xml` / `build.gradle` / `package.json` / `pyproject.toml` / `Cargo.toml` / `proto/` 目录）
  - 手动指定：显式配置 `服务名 + 根目录 + 语言/框架`
  - 探测结果以「候选服务清单」展示，用户勾选后入库；后续发现新服务进「待确认队列」
- F3.4 同步触发：手动 / 定时（Cron）/ Git Webhook（HMAC 签名校验）/ CLI & API
- F3.5 仓库健康度：最近同步状态、耗时、commit SHA、失败原因、连续失败次数
- F3.6 仓库配置文件 `.asset-platform.yaml`（结构见附录 A.3）：仅作为导入/初始化来源，是否采纳由服务级同步策略决定（F4.7）；录入时可一键「从配置文件导入」，导入结果先预览再落库

### F4 服务与系统分组（P0）

- F4.1 服务元信息：展示名、slug、描述、负责人、团队、标签、仓库内路径、语言/框架
- F4.2 生命周期状态流转与状态徽章：仅允许 `draft → published → deprecated → retired`。`draft/published/deprecated` 可采集与维护；公开服务仅在 `published/deprecated` 匿名可读；进入 `deprecated` 同事务产生一次 `service.deprecated` 事件；`retired` 仅保留鉴权后的历史只读、自助通知/todo 与软删除，所有服务内容写入返回 409 `invalid_state`
- F4.3 服务主页 = 概览 + 资产列表（按 kind 分组）+ 变更历史 + 订阅按钮
- F4.4 服务级收藏（Star）与最近浏览
- F4.5 服务删除：单事务软删除 Service/SourceSpec/Layer/Asset，Binding 标 stale、Track inactive 并提升 generation；清理授权/收藏/最近/标签/分组关系，取消 pending 工作并 fence running 提交；Revision/Version/索引/审计历史保留但所有外部路由统一 404
- F4.6 **资产源配置（核心）** —— 每个服务可配 0..N 个资产源，每个源定义**一个层**怎么来：

  | 配置项 | 说明 |
  |---|---|
  | `asset_kind` / `asset_name` | 归属的资产身份；repo/command/push/ai 资产不存在时可按规则创建，manual 必须显式指向已有资产。name 不填时从路径或文件名推导 |
  | `layer_role` | `base` / `overlay` |
  | `layer_origin` | `repo` / `third_party` / `manual` / `ai_generated` |
  | `mode` | `builtin`（读仓库路径）/ `command` / `push` / `manual`（UI 编辑）/ `ai`（F13） |
  | `path` | `builtin` 模式的文件路径，支持 glob：`api/**/openapi.{yaml,json}` |
  | `command` | `command` 模式引用的平台受控 producer profile；租户请求不得提交任意 shell |
  | `order` | overlay 应用顺序（仅 overlay 层） |
  | `timeout` / `enabled` | 单源超时；省略时 builtin/command/push/manual 为 120 秒、AI 为 600 秒；producer 实际超时取 source 与 profile 较小值；可临时停用而不删配置 |
  | `targetAssetId`（manual） | 必填；服务端在同一事务创建 SourceSpec、global Binding 和 Layer，并返回 `initialLayerId` 供后续提交 manual Revision |

  - 约束：一个资产至多一个启用的 base 层；无有效 base 时仅可用空骨架做 AI 输入/合并预览，不得创建 AssetVersion
  - glob 展开为多个资产：配置 `assetNameTemplate={parent_dir}` 后，`api/*/openapi.yaml` 命中 3 个文件时自动展开成 3 个 Asset（名字取目录名），每个各得一个独立 base 层；若沿用默认 `{file_stem}` 导致重名，则以 422 `validation_error` 拒绝本次 scope 的整批物化
  - 路径未命中时的错误处理（体验关键）：返回错误码 `asset_path_not_found` + 实际扫描到的候选文件列表（"找到了 `api/v1/swagger.json`，要改成它吗？"），前端一键修正
- F4.7 **配置来源与同步策略**（权威来源 = 平台 DB）：
  - 所有服务与资产源配置持久化在平台数据库，是唯一权威来源
  - 服务级 `config_sync_policy`：`ignore`（默认，忽略仓库配置文件）/ `import_once`（仅首次录入或手动导入时读取）/ `sync`（每次同步以仓库文件覆盖 DB，GitOps 模式）
  - 字段级来源标记 `config_source ∈ {db_manual, repo_file, repo_bootstrap}`：只有非 `db_manual` 字段允许被 `sync` 覆盖，避免冲掉人工改动
  - 漂移检测：仓库文件与 DB 配置不一致时，服务页显示「配置漂移」+ 差异对比 + 一键采纳/忽略
- F4.8 **SystemGroup（P1）**：租户内创建系统分组，把服务（可跨仓库）归入；支持一级嵌套（域 → 系统 → 服务）；分组是范围聚合视图（F6）与订阅（F9）的作用域单位

### F5 资产采集、层与版本（P0 核心）

**采集管道六阶段，每阶段可独立重试、可观测：**

```
resolve（拉代码/读缓存）
  → discover（服务发现 + 配置漂移检测）
  → extract（各 SourceBinding 产出各自的 LayerRevision）
  → merge（base + overlays 确定性合并）
  → normalize（kind 校验器：格式归一 / $ref bundle / lint / 质量分）
  → index（extractor 产出 AssetItem + provenance → 全文索引 → 变更检测 → 通知）
```

- F5.1 **Producer 模式**：

  | 模式 | 说明 | v1 状态 |
  |---|---|---|
  | `builtin` | 平台原生实现，v1 提供 `file-glob`：按配置路径读取仓库内已有资产文件 | ✅ 主路径 |
  | `command` | worker 在隔离工作区运行平台管理员注册的 producer profile；请求方只能选择已启用 profile | ✅ 扩展点 |
  | `push` | 外部系统通过 `POST /assets/push` 或 `meridian push` 推送 | ✅（CLI v1 提供） |
  | `manual` | UI 编辑/上传（人工层的编辑入口） | ✅ |
  | `ai` | AI 生成编排（F13） | ✅ |

  - 每个 Producer 在独立工作目录快照上执行，失败不影响其它 Producer
  - Producer 可声明外部依赖（`swag`、`buf`、`node` 等），启动时检测可用性，UI 标记「依赖缺失」
- F5.2 **层修订（LayerRevision）**：
  - 任何源的一次成功提交 = 一个修订事件；与当前 effective/candidate 内容相同则 unchanged；与历史或 rejected 内容相同可创建新事件并复用 blob
  - repo 层修订绑定 git commit；manual 层修订绑定操作用户；ai 层修订绑定生成任务 id + 模型标识 + prompt 摘要
  - 层可回滚到同 scope 内 `not_required/approved` 的历史修订；回滚只移动 effective Head，不新增 Revision，并产生新 AssetVersion
- F5.3 **合并引擎（核心）**：
  - 输入：base effective 修订 + 启用的 overlay 层按 order 排序的 effective 修订序列；exact ref Head 优先，缺失时继承 global Head
  - **纯函数、确定性、带引擎版本号**：相同输入必得相同输出（哈希可验证）；引擎升级后历史版本按原引擎版本重放或冻结结果
  - 冲突语义：后应用的层覆盖先应用的层，每次覆盖记入 provenance
  - 失败语义：overlay 应用失败（target 不匹配等）可配 `strict`（整体失败）/ `lenient`（跳过该 action 并告警，默认）
  - 输出：合并结果 + **provenance 清单**（结果中每个 JSON Pointer 前缀 → 最后写入它的层）
- F5.4 **Overlay 方言**：
  - `openapi`：**OpenAPI Overlay Specification 1.0** 为一等方言（`overlay: 1.0.0` + actions[target(JSONPath)/update/remove]），同时接受通用 Overlay
  - 其它 kind：**平台通用 Overlay 规范**（附录 A.2）——`target`(JSONPath) + `merge`(RFC 7386 深合并) / `remove` / `patch`(RFC 6902 兜底)
  - 两种方言解析后编译到同一内部 action 模型，合并引擎唯一
  - 保存 overlay 修订时即校验语法与 `target_kind` 匹配，不合法拒绝入库
- F5.5 **AssetVersion**：
  - 每次 Track build fingerprint 相对 latest 变化 ⇒ 新 AssetVersion；A→B→A 仍产生第三版，结果 hash 可与历史重复
  - 每 Track 首版 `1.0.0`；breaking/minor/patch 自动递增；手工版本仅首次 publish 可指定且必须大于该 Track 既有版本
  - 状态机仅允许 `draft → published → deprecated → retired`；latest/current 都按 Track 保存，旧版本按保留策略可访问
  - 发布动态检查 manifest、enabled scope 的 candidate、index 完整性和生命周期；AI/第三方 pending 永不进入 manifest。自动 publish 还要求 base 来自 repo 且无 AI 修订
- F5.6 归一化与下载：
  - openapi：Swagger 2.0 / OAS 3.0 归一为 OpenAPI 3.1 存储；`$ref` bundle 产出自包含版本供渲染；原始文件保留
  - 规范校验 + Lint（规则集可配）+ 质量分，展示在服务页
  - 下载物：合并后文件、各层原始文件、provenance JSON、bundled 文件、Markdown（P1）

### F6 多视图查看器（P0 核心）

**设计目标：新增一个视图 = 注册一条 ViewDef + 一个前端组件/iframe，不改核心代码。每个视图用 ViewInputSpec 声明式描述自己接受什么输入。**

- F6.1 **ViewDef**：唯一注册表为 [contracts/views.yaml](../contracts/views.yaml)，声明 id、输入限制、mount、options schema、columns 和里程碑；租户只保存启停/排序/默认参数覆盖。
- F6.2 **ViewInputSpec —— 视图输入契约（四模式）**：`single`、`versions`、`collection`、`scope` 的 discriminator 与 DTO 由 [contracts/openapi.yaml](../contracts/openapi.yaml) 定义，内建视图的接受范围由 `views.yaml` 定义。
  - 后端提供统一**输入解析器**：前端路由/分享链接携带 input descriptor（如 `{mode:'versions', assetId, versionIds:[v3,v7]}` 或 `{mode:'scope', systemGroupId, kinds:['dependency']}`），后端校验其满足目标视图的 ViewInputSpec 后，解析为 `DocRef[]`（含签名内容 URL）或 AssetItem 查询结果喂给视图
  - 分支/版本表达：`versions` 模式的 branch/tag selector 先解析为 Track 的固定 versionId；快照与分享只保存固定结果，不随 latest 漂移
  - 视图运行时统一入参：`{ docs: DocRef[] | ItemQueryResult, scope?, options, theme }`
- F6.3 **内置视图**：

  | 视图 | input.mode | kinds | 说明 | 优先级 |
  |---|---|---|---|---|
  | `swagger-ui` | single | openapi | 交互式调试（Try it out） | P0 |
  | `redoc` | single | openapi | 三栏阅读型 | P0 |
  | `source` | single | * | 合并后源码 + 语法高亮 + **层标注开关**（hover 显示每段来自哪个层，基于 provenance） | P0 |
  | `layers` | single | * | 层管理视图：层列表/启停/排序、各层修订历史、逐层预览合并前后差异、回滚 | P0 |
  | `operations` | single | openapi | 端点表格，筛选排序 | P0 |
  | `items-table` | single | * | 通用条目表格（任何 kind 的免费默认视图） | P0 |
  | `diff` | versions(2..2) | * | 结构化 DIFF（F7） | P0 |
  | `changelog` | versions(2..50) | * | 跨版本聚合变更日志 | P1 |
  | `collection-table` | collection | * | 跨服务同类资产汇总/对齐度 | P1 |
  | `dep-graph` | scope | dependency | 跨系统全局依赖图（按 SystemGroup / 租户聚合） | P1 |
  | `catalog-dashboard` | scope | * | 资产大盘：数量/质量分/覆盖率（多少服务还没有 openapi 资产） | P1 |
  | `erd` | single | dbschema | 数据模型 ER 图 | P2 |
  | `rapidoc` / `markdown` / `mock` / `embed` | single | openapi | 轻量渲染 / Wiki 导出 / Mock / 嵌入片段 | P2 |
- F6.4 三种挂载方式：`component`（Nuxt 内置组件，用于 diff / layers / items-table 等深度定制视图）/ `iframe`（第三方 bundle 如 Swagger UI、Redoc，由后端托管 dist，强隔离、升级只换资源包）/ `external`（外链）
- F6.5 视图偏好记忆：用户级 + 服务级默认视图；视图开关支持平台级默认 + 租户级覆盖（启停/排序/默认参数）
- F6.6 **可分享链接**：任意「input descriptor + 视图 + 视图参数」组合可生成 URL；支持签名分享链接（带过期时间的匿名只读 token）

### F7 Diff 与兼容性分析（P0）

- F7.1 对比对象（由输入契约自然覆盖）：同资产两版本 / 同资产两分支 / 跨服务两资产 / 本地上传文件 vs 平台版本
- F7.2 **Diff 器按 kind 插件化**：
  - `openapi`：结构化变更模型（libopenapi/what-changed + 自定义分级）。层级 `Document → Path → Operation → Parameter / RequestBody / Response / Schema / Security`；类型 `added / removed / modified`；级别 `breaking / risky / non_breaking / informational`
  - 其它 kind：通用结构化 diff（基于 AssetItem 集合的 added/removed/modified + JSON 树 diff）；breaking 规则由 kind 注册时可选提供（如 dbschema：删列 / 类型变窄 / 非空化 = breaking）
- F7.3 **Breaking 判定规则可配置**，openapi 默认规则：删除/重命名端点或路径；新增必填请求参数/字段；收紧参数约束（maxLength 变小、enum 减少）；响应字段删除或类型变窄；状态码删除；认证方式收紧；media type 删除
- F7.4 三种 Diff 呈现：`tree`（层级变更树，默认）/ `side-by-side`（并排文本）/ `list`（条目级变更清单 + 级别筛选）
- F7.5 变更统计卡片：新增 X / 删除 Y / 修改 Z / 破坏性 N
- F7.6 Diff 结果可保存为快照、生成分享链接、导出 Markdown / JSON
- F7.7 Changelog 聚合：跨多版本自动生成人类可读变更日志（P1）
- F7.8 **层感知 diff（P1）**：两个 AssetVersion 的差异按「引起变化的层」分组——"3 处来自 repo 层 commit abc123，2 处来自人工层编辑"

### F8 检索与目录（P1）

- F8.1 全局搜索：条目级（method + path + summary + 表名 + 列名 + 描述等，跨 kind）、资产级、服务级、仓库级
- F8.2 实现：PostgreSQL `tsvector` 全文 + `pg_trgm` 模糊 + GIN(JSONB)；**所有索引带 `tenant_id`**（隔离下沉到检索层）
- F8.3 筛选与分面：团队、标签、生命周期、语言、资产类型（kind）、层来源（是否含 AI 层）、SystemGroup、是否有破坏性变更
- F8.4 目录首页：卡片/列表切换、最近更新、我负责的、我收藏的、待我处理（breaking 未确认、AI 层待审批）
- F8.5 标签体系：自由标签 + 租户管理员维护的标签字典
- F8.6 条目级跨 kind 检索：搜 `order_status` 同时命中 openapi 字段、dbschema 列、config 配置项

### F9 协作、订阅与通知（P1；M3 先交付待审通知与 breaking 待办）

- F9.1 订阅粒度：服务级 / 单资产级 / SystemGroup 级 / AssetKind 级（"本系统任何 dbschema 变更都通知我"）
- F9.2 事件类型：新版本发布、破坏性变更、采集失败、服务废弃、`ai_layer.generated`（AI 层产出待审）、`layer.approved` / `layer.rejected`
- F9.3 通知渠道：站内消息（先做）+ Webhook（企微/飞书/钉钉/自定义）+ 邮件（可插拔 provider）；平台级默认 + 租户级覆盖
- F9.4 **Breaking Change 确认机制**：产生破坏性变更时给负责人生成待办，确认「已知悉/已通知调用方」后才能关闭
- F9.5 服务页轻量评论/备注（不做完整讨论区）

### F10 任务调度与可观测（P0）

- F10.1 任务类型：`repo.sync`、`repo.discover`、`asset.produce`、`asset.merge`、`asset.index`、`asset.ai_generate`、`diff.run`
- F10.2 任务队列：基于 PostgreSQL 的 **River** 队列（原子领取、重试/退避、超时回收）；接口抽象为 `Queue`
- F10.3 并发控制：全局 worker 数、按租户配额限流、单仓库工作区串行（避免 git 冲突）；采集并发默认 4
- F10.4 任务日志：分阶段记录，流式查看（SSE / 轮询）
- F10.5 任务中心：默认只看当前租户；平台管理员可跨租户
- F10.6 工作区管理：clone 缓存按租户分目录，LRU 清理 + 磁盘配额

### F11 开放能力与集成（P1；M3 先交付 push/diff CLI 与相关 API）

- F11.1 REST API 全覆盖：UI 能做的 API 都能做；平台 OpenAPI 文档自描述（并作为自身的一份 openapi 资产纳管，dogfooding）
- F11.2 CLI `meridian`：`push`（CI 推送资产/层）、`diff --fail-on breaking`（门禁）、`list` / `sync` / `produce` / `ai-gen`（F13 参考实现）；参数、selector、等待语义和退出码以 [contracts/cli.yaml](../contracts/cli.yaml) 为唯一契约
- F11.3 CI 门禁：MR/PR 阶段阻断破坏性变更；结果回传平台（P2）
- F11.4 出向 Webhook：资产发布、破坏性变更、AI 层待审时回调外部系统
- F11.5 SDK / 客户端代码生成：集成 `oapi-codegen` / `openapi-generator`（P2）

### F12 运维与合规（P1）

- F12.1 健康检查 `/healthz`、`/readyz`；Prometheus `/metrics`（含按租户维度）
- F12.2 结构化日志（JSON），每条带 `tenant_id` 与 `trace_id`
- F12.3 备份：`pg_dump` 定时备份 + 保留策略；MVP blob 使用 `MERIDIAN_BLOB_ROOT` 持久卷上的 content-addressed 文件存储，按与数据库一致的恢复点联合备份；工作区是可重建缓存，不进入备份
- F12.4 配置：env + 配置文件 + 运行时配置入库（代理、cron、通知渠道），区分平台级与租户级
- F12.5 数据保留：任务日志、旧版本、归档层修订的保留天数可配（平台默认 + 租户覆盖）
- F12.6 租户数据导出/删除：导出单租户全部数据（JSON + blob），支持彻底删除租户及其数据

### F13 AI 生成源（P0 闭环骨架，P1 完善）

> 平台**不内置 AI 模型**：AI 调用通过平台管理员注册的 producer profile 或外部系统 push 完成，平台负责编排、状态机与审批。业务请求不能提交原始 shell。官方参考 profile 可调用 OpenAI 兼容 API；成本、限流和模型鉴权由该受控 producer 负责。

- F13.1 **冷启动流程（P0）**：
  1. 接入仓库/分支/服务后，发现某服务无 `openapi` 资产或没有有效 base
  2. 目录页/服务页展示「资产缺失」+「用 AI 生成」按钮；也可在采集管道配置为自动触发
  3. 服务级冷启动接口创建资产槽位与 `ai` SourceSpec/Layer，投递 `asset.ai_generate`：在隔离的只读代码快照中运行选定 profile，产出写入独立 `$OUTPUT_DIR`
  4. 产出通过 kind 校验器 → 新 LayerRevision（状态 `pending_review`）→ 通知有 `layer:approve` 权限的人
  5. 审核界面 = `layers` 视图 + 合并前后 diff 预览；**通过** ⇒ candidate 原子转 effective、触发 merge、版本可发布；**驳回** ⇒ 修订保留 rejected 留痕，可带意见重新生成
- F13.2 AI 层的角色：服务无任何 base 时，AI 层可作为 base（origin 仍为 ai_generated）；已有 repo base 时，AI 只能产出 overlay（如"为所有缺 description 的端点补描述"）
- F13.3 工程约束：以 producerRunId 保证任务重试幂等，content_hash 只用于 blob 复用；AI producer 超时默认 10min、command producer 默认 5min（SourceSpec 可收紧，实际取 source/profile 较小值）；completion manifest 决定恢复策略；AI 修订永不自动 publish
- F13.4 **审批状态机**（对 `ai_generated` 与 `third_party` 层生效）：`pending_review → approved / rejected / superseded`；仅 effective 的 `approved/not_required` 修订参与 merge；新 candidate 自动 supersede 旧 candidate；租户信任模式只跳审批、不自动发布
- F13.5 重新生成与增量（P1）：repo base 更新后一键"基于新代码重新生成 AI overlay"，并展示新旧 AI 修订的 diff
- F13.6 AI command 在受限工作目录执行，支持禁网选项；provenance 只存 prompt 摘要不存全文

AI 产出质量必须通过固定 fixture 验收：生成文档必须通过 kind schema 和 lint，关键 fixture 的端点召回、HTTP method/path、参数和响应结构满足 `ai-quality-gate` 的最低阈值；任何 schema/lint 失败或无法解释的产出不得进入 approved/effective。

### F14 AssetKind 扩展机制（P0 定接口，P1 完善）

**新增一种资产类型的成本 ≤ 1 人周（验收指标）。** kind 注册包含：

| 接口 | 必须 | 说明 |
|---|---|---|
| `Validator` | ✅ | 校验内容合法性；可选归一化（如 swagger2 → oas3.1） |
| `ItemExtractor` | ✅ | 合并结果 → AssetItem[]（含每条的展示字段与检索文本） |
| `Differ` | ⭕ | 缺省用通用结构化 diff（F7.2） |
| `BreakingRules` | ⭕ | 缺省则 diff 无 breaking 分级 |
| `OverlayDialect` | ⭕ | 缺省只支持通用 Overlay |
| 前端视图 | ⭕ | 缺省自动获得 `source` / `layers` / `items-table` / `diff` 四个通用视图 |

- F14.1 v1 kind 以 Go 编译期插件（代码内注册）实现，接口从 v1 冻结；进程外插件（gRPC）为 P2 扩展点
- F14.2 租户级可启停 kind；停用后存量数据只读不删
- F14.3 平台用 `dbschema` + `dependency` 两个非 API kind 验证抽象完备性（dogfooding，M4 验收项）

---

## 4. 非功能需求

| 类别 | 要求 |
|---|---|
| 性能 | 目录页 < 300ms（P95）；文档页首屏 < 1.5s；条目检索 < 500ms（十万级条目）；merge 纯函数 P95 < 200ms（1MB 文档 + 10 层）；全局依赖图（500 服务 / 5000 边）首屏 < 2s；大文档（5MB+）渲染不阻塞主线程 |
| 容量 | 50 租户 / 500 仓库 / 2000 服务 / 2 万资产 / 10 万层修订 / 50 万条目 |
| 并发 | 单实例应用 + PostgreSQL；采集并发默认 4，按租户配额限流；单仓库工作区串行 |
| 确定性 | merge 可重放：给定 AssetVersion 的层修订清单与引擎版本，任何时候重放得到相同哈希 |
| 隔离 | 租户间数据完全不可见；越权访问返回 404（避免资源存在性泄漏）；检索、任务、审计全部带租户维度 |
| 安全 | 凭证静态加密；密钥不进日志/响应；CSRF；PAT 仅展示一次；分享链接签名 + 过期；producer 隔离执行且默认断网；依赖漏洞扫描进 CI |
| 可用性 | 应用无状态（状态在 PG 与 blob 存储）；任务中断可恢复 |
| 可维护 | 分层清晰（handler / service / repo）；核心逻辑单测覆盖 ≥ 70%；必测项：租户越权用例（每个资源）、overlay 合并确定性用例、provenance 正确性用例；DB 迁移版本化可回滚 |
| 易用 | 首次启动 5 分钟内看到第一个服务的文档（内置示例租户 + 示例仓库一键体验） |
| 国际化 | 前后端文案抽离，v1 中英文，先中文 |
| 主题 | 亮/暗主题，viewer 主题跟随 |
| 部署 | 生产/标准体验使用单 Go 二进制（内嵌前端）+ PostgreSQL 16；docker-compose 一条命令启动。M5 体验镜像是在同一容器受控拉起 PostgreSQL 16 进程，不引入第二种数据库引擎，禁止生产使用 |

所有性能、容量、恢复和覆盖率指标必须通过固定 fixture、固定并发模型、固定工具链和报告文件测量；测量协议与门禁命令由 `docs/07_coding-agent-runbook.md` 和工作项 `verify` 字段定义。未有报告的数字只视为目标，不视为已验收。

---

## 5. 明确不做（Out of Scope v1）

- ❌ **API 网关 / 流量代理**：本平台是目录与文档，不做运行时流量转发、鉴权、限流
- ❌ **接口自动化测试平台**：不做用例管理与定时拨测（只提供 Try it out 单次调试）
- ❌ **服务注册中心 / 服务治理**：不接管服务实例健康检查与发现
- ❌ **租户计费**：做配额与用量统计，不做计费账单
- ❌ **内置 AI 模型**：只提供编排、状态机与审批；AI 调用由外部工具承担
- ❌ **通用低代码建模器**：AssetKind 的内容 schema 由代码注册定义，不做 UI 自定义资产类型（P2 再评估）
- ❌ **Overlay 可视化编辑器完整版**：v1 人工层提供 YAML 编辑 + 实时合并预览；表单化逐字段编辑放 P2
- ❌ **跨租户全局视图**：scope 最大到租户
- ❌ **多数据库支持**：仅 PostgreSQL；数据访问层按可移植设计（sqlc/pgx），但不为假想的移植牺牲功能

---

## 6. 架构决策记录

| 编号 | 决策 | 影响 |
|---|---|---|
| D1 | 资产主路径 = 仓库文件 base 层 + overlay 叠加；AI / 人工 / 第三方全部收敛进层模型，无特例代码路径 | 采集、审批、diff、provenance 共用一套管道 |
| D2 | 存储 = PostgreSQL 16（层/版本/provenance 用 JSONB，全局视图用递归 CTE，队列用 River） | docker-compose 为标准部署；M5 单容器体验镜像只是附带并拉起同版本 PostgreSQL 进程 |
| D3 | 服务/资产配置的权威来源 = 平台 DB；`.asset-platform.yaml` 为导入来源（`ignore` / `import_once` / `sync` 三策略 + 字段级来源标记 + 漂移检测） | 人工修改不被 GitOps 覆盖 |
| D4 | 多租户强制隔离：所有业务表带 `tenant_id`，唯一约束限定租户内，检索索引带租户列，工作区按租户分目录，配额与审计按租户 | 隔离不可事后补 |
| D5 | Overlay 双方言：openapi 兼容官方 OpenAPI Overlay Spec 1.0，其余 kind 用平台通用 Overlay；两方言编译到同一内部 action 模型，合并引擎唯一 | 生态兼容 + 全 kind 一致 |
| D6 | 视图输入契约四模式（single / versions / collection / scope），分享链接、权限校验、缓存基于同一 input descriptor | 视图能力可被机器校验，新视图零侵入 |
| D7 | merge 是带版本号的纯函数 | 可重放、可审计、可缓存 |
| D8 | MVP blob 驱动固定为本地持久卷上的 SHA-256 content-addressed store；S3-compatible 驱动不在 v1 范围 | 单实例和单容器部署没有额外对象存储依赖；key、原子写与签名下载见 `domain.yaml.storage.blobStore` |

---

## 7. 里程碑

| 里程碑 | 周期(参考) | 内容 | 验收标准 |
|---|---|---|---|
| **M0 地基** | 2-3 周 | 契约生成、租户/权限/PAT、凭证、仓库连接骨架、River、审计/outbox、核心迁移 | 越权与 PAT 用例全绿；契约生成无漂移；凭证连通性可测 |
| **M1 资产主链路** | 3-4 周 | 仓库发现/同步、SourceSpec/Binding、默认分支 Track、openapi file-glob → merge → normalize/index → Viewer/public read、任务恢复 | 5 分钟内从录入仓库看到第一份文档；相同 latest fingerprint no-op |
| **M2 层与 Overlay** | 3 周 | LayerHead/Revision、通用 Overlay、OpenAPI Overlay 1.0、manual 编辑/预览、GitOps 字段来源、provenance | 三层合并确定性、历史回退和漂移用例通过 |
| **M3 AI 闭环 + Diff** | 3 周 | AI 冷启动/审批、分支 Track、openapi diff/breaking、snapshot/share、todo、最小通知、`meridian push/diff` | 无文档 → AI → 审批 → 发布 → release diff → 分享/待办/CI 门禁完整演示 |
| **M4 多 kind + 全局视图** | 3-4 周 | dbschema、dependency 两个 kind（含通用 diff）、SystemGroup、dep-graph、catalog-dashboard、检索 | 新 kind 接入实测 ≤ 1 人周；全局依赖图可用 |
| **M5 协作与开放** | 3 周 | 通用订阅、站内信、通知渠道、出向 webhook、asyncapi、运维合规加固 | 事件至少一次投递、签名重试与订阅取消用例通过 |
| M6+ | — | config / deployment kind、erd / mock 视图、gRPC kind 插件、SDK 生成、层感知 diff | — |

---

## 附录 A · 实现约定

### A.1 核心表契约

核心表、字段、唯一约束、复合租户外键及事务边界已冻结在 [contracts/storage.yaml](../contracts/storage.yaml)。实现不得从本需求文档反推 DDL。关键拆分包括 SourceSpec/SourceBinding、LayerHead 和 AssetRefTrack；不存在 `layers.current_revision_id` 或 Asset 级全局 current version。

### A.2 通用 Overlay 规范

```yaml
overlay: platform/v1
target_kind: dbschema            # 必须与资产 kind 匹配，保存时校验
actions:
  - target: "$.tables[?(@.name=='orders')].columns[?(@.name=='status')]"
    merge:                       # RFC 7386 JSON Merge Patch，深合并进选中节点
      description: "订单状态，见状态机文档"
  - target: "$.tables[?(@.name=='tmp_migration')]"
    remove: true                 # 删除选中节点
  - patch:                       # 逃生舱：RFC 6902 JSON Patch，作用于整份文档
      - { op: add, path: "/tables/-", value: { name: "audit_log", columns: [] } }
```

- 语义：`target` 用 JSONPath 选中 0..N 个节点；action 按数组顺序应用
- target 命中 0 节点：`lenient` 模式（默认）记警告跳过；`strict` 模式整体失败
- OpenAPI Overlay 1.0 文档解析后编译为同一内部 action 序列，两方言共用一个合并引擎

### A.3 仓库配置文件 `.asset-platform.yaml`

探测位置（优先级）：`<service_root>/.asset-platform.yaml` → `<repo_root>/.asset-platform.yaml`。是否采纳由服务级 `config_sync_policy` 决定（F4.7）。

以下仅为阅读示例；唯一可验证结构是 [contracts/repository-config.schema.yaml](../contracts/repository-config.schema.yaml)。`command` 模式通过全局唯一名称 `producerProfile` 引用平台受控 profile，导入时解析为内部 UUID，不接受原始 shell。

```yaml
version: 1
services:
  - name: order-service
    root: services/order
    assets:
      - kind: openapi
        name: admin-api
        base: { mode: builtin, path: "docs/openapi.yaml" }
        overlays:
          - { mode: builtin, origin: repo, path: "docs/openapi.overlay.yaml" }
      - kind: dbschema
        name: main
        base: { mode: command, producerProfile: "atlas-inspect-json" }
```

### A.4 HTTP API 契约

HTTP 路径、operationId、请求响应、认证与错误仅见 [contracts/openapi.yaml](../contracts/openapi.yaml)。本文不保留 API 示例副本，避免路径与生成代码漂移。
