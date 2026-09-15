# Meridian 架构设计遗漏与优化建议 (Design Review & Refinements)

> 状态：提案 (Proposed, 2026-09-06)；2026-09-15 对齐后：1.2 与 1.4 已采纳并落地到契约（见各节「决议」），其余条目维持待办。
> 目的：记录 M0-M5 设计规范中发现的重大边界遗漏、分布式系统缺陷以及关键 UX 优化项。这些问题在实施前需得到明确决策与修正，避免引发后续返工。

## 1. 重大设计缺陷与遗漏 (Critical Defects & Omissions)

### 1.1 分布式任务取消的“断层”假象 (Task Cancellation Illusion)
- **发现位置**: `00_development-readiness.md`
- **问题描述**: 规定“取消在同一 PostgreSQL 事务内锁定 Meridian Job、调用 River JobCancelTx 并写入 cancelled”。
- **风险分析**: River Queue 基于数据库，调用 Cancel 只是修改了数据库中的 Job 状态。但 Worker 极可能正在执行长耗时、阻塞型的 I/O 任务（如拉取巨大的 Git 仓库、执行 AI 生成编排、运行 `bwrap` 沙箱命令）。如果不通过 Go 的 `context.Context` 监听 cancellation 信号并级联传递给底层进程或网络请求，前端虽然显示“已取消”，但后端 Worker 依然在空转甚至写临时磁盘。这会导致资源的隐性泄露、并发队列阻塞，且后续的事务隔离会被破坏。
- **修正要求**:
  - Worker 的 handler 必须挂载对 River 传递的 `context.Done()` 的监听。
  - 启动外部命令（Producer profile 等）时，必须将该 context 传递给 `exec.CommandContext`，以确保收到 Cancel 信号时，底层的 `bwrap` 等进程能被强制发送 `SIGKILL` 并在限定时间内清理临时工作目录。

### 1.2 破坏性变更待办 (Breaking Todo) 的广播风暴
- **发现位置**: `01_requirement-specification.md` (F9.4) & `02_backend-technical-design.md` (第 10 节)
- **问题描述**: 设计指出当发现 Breaking Change，会给服务负责人生成待办，且“唯一键为 `(assetVersionId, assigneeUserId)`。多个用户 owner 各一条”。
- **风险分析**: B 端系统中，一个 Service 的 `owner` 或 `maintainer` 通常映射到一个团队（Team，可能有数十人）。如果发生一次 Breaking Change，系统就给这个团队的每位成员都创建一条独立的数据库 Todo，将会引发极为严重的“通知噪音”和警报疲劳。更致命的是，事件响应的状态会因此割裂：如果团队中有一人处理（Ack）了该变更，其他所有人的面板上这条待办依然悬挂，违背了协作原则。
- **修正要求**:
  - Todo 应作为 Team/Service 级别的业务实体进行分配，而非直接打散平铺至 Individual。
  - 引入**“认领（Claim）”**与**“共享完成”**机制。任何具备权限的成员确认 (Ack) 之后，该版本下的破坏性待办即转为关闭，其他人的看板应同步消隐。

- **决议（2026-09-15，已采纳）**：`breaking_todos` 改为**服务级待办**，唯一键 `(asset_version_id, service_id)`；任何有权限的服务成员 ack 即关闭，行内记录 `acked_by`/`acked_at`。契约见 `domain.yaml.events.breakingTodos` 与 `storage.yaml.tables.breaking_todos`；逐用户展开（`assignee_id`）已移除。

### 1.3 AI Base 替换时的 Overlay 静默失效与漂移
- **发现位置**: `00_development-readiness.md` (模糊点解释)
- **问题描述**: 当无代码的服务使用了 AI 生成的 Base 后，再次接入 Repo 代码源码时，可通过 `replaceAiBase=true` 原子性地用代码替换 AI Base。
- **风险分析**: 用户极有可能在旧的 AI Base 上添加了多个人工 Overlay 补丁（利用 JSONPath 给某些字段写了详细的业务描述）。AI 幻觉生成的 JSON 节点结构和真实代码提取出的结构必然存在细微差异。当底层发生彻底替换时，大量 JSONPath 会因找不到 Target 而瞬间失效。由于现有的 Overlay Merge 引擎默认使用 `lenient` 模式（静默跳过非法 target 并记录 warning），这将导致用户辛苦编写的人工资产静默丢失，且无从溯源。
- **修正要求**:
  - 阻断“一键替换”操作。`replaceAiBase` 接口必须分拆为 `Preview` 与 `Apply` 两阶段。
  - `Preview` 时需运行合并验证引擎，并向前端结构化返回将要失效（Orphaned）的 Overlay Actions 列表，必须强制人工知晓风险后再执行确认。

### 1.4 SystemGroup 层级与 Service 拓扑结构的递归冲突
- **发现位置**: `01_requirement-specification.md` (F4.8) vs `00_development-readiness.md`
- **问题描述**: 需求中称 SystemGroup 支持“一级嵌套（域 -> 系统 -> 服务）”，但在 Readiness 的结论中约束为“最大深度 1”。
- **风险分析**: 这一“深度 1”在 DDD 的定义中极度模糊。如果是 Group 嵌套 Group 最大深度 1，加上底层的 Service，实际关系形成了 `Parent Group -> Child Group -> Service` 的 3 层树。由于目前表结构使用 `parentId` 进行自引用，未明确规定 Service 能否挂载到 Parent 节点；也未规定 Service 是否可以属于多个系统组。如果处理不当，会导致复杂的全局视图依赖图（Recursive CTE 查询）产生死循环或是组合爆炸。
- **修正要求**:
  - 如果约束 3 层，建议取消自引用的 `parentId`，采用两张独立实体表 `SystemDomain`（域）与 `SystemGroup`（系统），从数据结构上杜绝任意嵌套和深度扩散。
  - 明确规定 `Service` 与 `SystemGroup` 的映射是 M:N（多对多）还是 1:N（一对多），并在 `storage.yaml` 补充唯一约束和业务层 Check。

- **决议（2026-09-15，已采纳）**：采用**单层 SystemGroup**（去 `parent_id` 自引用与递归 CTE），服务 M:N 挂多个组；「域」概念取消。契约见 `domain.yaml.systemGroups` 与 `storage.yaml.tables.system_groups`（已删 `parent_id` 列）。

## 2. 关键优化项 (Important Optimizations)

### 2.1 永久链接与“跟随最新”路由截断问题 (UX 破坏)
- **发现位置**: `03_frontend-technical-design.md` (5.4 Viewer Shell)
- **问题描述**: 规定了路由带有 `latest` 或分支名时，前端马上拉取后端解析出的固定 `versionId` 并重写浏览器 URL，理由是“防止分享深链时随 latest 漂移”。
- **优化建议**: 这种一刀切逻辑摧毁了企业文档管理中最核心的用户诉求之一——**固定书签引用**。跨团队研发时，调用方通常将 `.../versions/latest` 加入收藏夹，以便每次点开自动拉取最新的接口文档。强制修改 URL 栏会导致调用方总是查看到过时的快照版本。
- **修正要求**: 浏览器地址栏需维持动态 `latest`。只有当用户主动点击页面功能栏的**“分享/复制快照链接（Copy Snapshot Link）”**时，才向剪贴板写入冻结了确定 `versionId` 的 Deep Link。

### 2.2 废弃 (Retired) 资产对检索索引的长期污染
- **发现位置**: `01_requirement-specification.md` (F4.2 & F8)
- **问题描述**: 设计定义 `retired` 状态为服务的终态，“内部历史保留，外部路由统一返回 404”。但在全局搜索中未定义该状态的行为边界。
- **优化建议**: 长期迭代中，废弃（retired）的资产数量往往远超活跃资产。如果全局搜索对 `tsvector` 依然全量扫描，会造成搜索结果中充斥着大量的僵尸接口，严重降低核心功能的查询体验。
- **修正要求**: 在全局搜索（F8 核心能力）的实现契约中，增加 `lifecycle != retired` 作为系统级的隐式默认过滤条件（Default Predicate）。若架构师确有需要，在前端的分面过滤（Facets）中提供一个“包含已退役资产（Include Retired）”的可选 Switch 供其突破界限。
