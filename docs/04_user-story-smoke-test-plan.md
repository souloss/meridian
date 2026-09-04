# Meridian · 用户故事 SOP 与冒烟测试计划

> 需求文档：[01_requirement-specification.md](./01_requirement-specification.md)
> 配套设计：[后端](./02_backend-technical-design.md) / [前端](./03_frontend-technical-design.md)
> 文档用途：**验收权威**。开发（含 AI 开发者）完成任务后，对照本文的用户故事理解真实业务目标，并按第 3、4 章编写/执行端到端与 API 用例，全部通过才视为"真实完成"。
> 约定：故事编号 `US-xx`，冒烟用例编号 `SMK-xxx`，API 用例编号 `API-xxx`。用例中的角色、数据统一使用第 1.2 节的种子数据。

---

## 1. 总则

### 1.1 判定"任务真实完成"的标准（Definition of Done）

一个里程碑/功能视为完成，当且仅当：

1. 对应用户故事的**每一条验收标准（AC）** 有至少一个自动化用例覆盖并通过；
2. 第 3 章标注了该里程碑的冒烟用例全部通过（`里程碑` 列）；
3. 第 4 章对应的 API 反向用例（错误路径、越权）全部通过；
4. 不引入任何"绕过后端校验的前端假实现"（如前端隐藏按钮代替后端 404、前端算 publish 谓词）——抽查方式见 4.6;
5. 种子脚本（1.2）可从空库一键构建全部前置数据。

### 1.2 统一种子数据（所有用例的前置）

种子脚本 `scripts/seed-e2e.sh`（待实现，属 M0 交付物）构建：

| 对象 | 值 |
| --- | --- |
| 租户 | `acme`（正常）、`rival`（用于越权对照） |
| 用户 | `padmin`（platform_admin）、`alice`（acme tenant_admin）、`bob`（acme maintainer）、`carol`（acme viewer + order-service 的 layer:approve）、`eve`（仅 rival 成员） |
| 示例仓库 | `fixture-repo-a`：本地 bare git 仓库，含 `services/order`（有 `docs/openapi.yaml`，3 个端点）与 `services/pay`（**无任何资产文件**，Go 代码若干）；分支 `main`、`release/2.0`（openapi 有 1 个 breaking 差异：删除端点 `DELETE /orders/{id}`） |
| 示例仓库 | `fixture-repo-b`：含 `.asset-platform.yaml`（声明 openapi + overlay 文件） |
| overlay 样例 | `fixtures/overlay-valid.yaml`（platform/v1，给 orders 补 description）、`fixtures/overlay-oas.yaml`（OpenAPI Overlay 1.0）、`fixtures/overlay-invalid.yaml`（target_kind 不匹配） |
| AI 假命令 | `fixtures/fake-ai-gen.sh`：读 `$OUTPUT_DIR` 写出合法 openapi（含 `$SERVICE_ROOT` 内代码可推断的 2 个端点）；`fixtures/fake-ai-timeout.sh`：sleep 超时；`fixtures/fake-ai-bad.sh`：产出非法 yaml |
| PAT | `bob` 创建的 `ci-token`（scopes: `asset:push`,`asset:read`,`job:run`） |

> 所有 E2E 用例禁止依赖公网 Git/AI 服务；fixture 仓库用本地 `file://` 或容器内 git daemon。

### 1.3 角色视角速览

| 角色 | 关心什么（读故事时的锚点） |
| --- | --- |
| 平台管理员 | 建租户、全局审计，但**看不到租户业务数据正文** |
| 租户管理员 | 成员、凭证、配额、租户级开关（信任模式、kind 启停） |
| 服务维护者 | 接入仓库→出文档的效率；层/资产源配置；发布 |
| 审批人 | AI/第三方修订的审查效率与可追溯 |
| 消费者/浏览者 | 找资产、看文档、比差异、订变更 |
| CI Bot | push 资产、diff 门禁 |

---

## 2. 用户故事 SOP（按旅程编排）

> 每个故事 = 背景 → 操作步骤（SOP，可直接转成 E2E 脚本）→ 验收标准（AC，可判定）。

### US-01 平台初始化与租户开通

**背景**：全新部署后，平台管理员开通第一个租户并指定管理员。

SOP：
1. `padmin` 登录 → 进入 `/admin/tenants` → 创建租户 `acme`（slug、展示名、默认配额）。
2. 创建/邀请用户 `alice`，将其加入 `acme` 并授予 `tenant_admin`。
3. `alice` 登录，顶栏租户切换器出现 `acme`，进入 `/t/acme`。

AC：
- [AC1] `padmin` 能创建租户与成员关系；`alice` 登录后 `GET /auth/me` 返回 `{tenants:[{slug:"acme",role:"tenant_admin"}]}`。
- [AC2] `padmin` 访问 `acme` 的业务资源（服务/资产正文 API）得到 404；访问 `/admin/audit` 能看到 acme 的操作元数据。
- [AC3] 租户 slug 重复创建返回明确错误；停用租户后其成员登录只见空租户列表提示。

### US-02 接入仓库到看见第一份文档（Golden Path，最重要）

**背景**：`bob` 要把 `fixture-repo-a` 的 order 服务文档纳管。**这是平台的核心价值路径，必须 5 分钟内走通。**

SOP：
1. `bob` 进入 `/t/acme/repos/new`：填仓库 URL（fixture 地址），凭证选 `none`，[测试连接] 显示成功。
2. 保存后自动进入发现步骤：候选服务清单出现 `services/order`（依据 go.mod）与 `services/pay`；勾选两者确认。
3. 为 `order-service` 配置资产源：kind=`openapi`、mode=`builtin`、path=`docs/openapi.yaml`、role=`base`、origin=`repo`；保存并触发同步。
4. 任务抽屉展示六阶段进度至成功；跳转服务主页。
5. 服务主页资产列表出现 `openapi:openapi`（或推导名），点击进入 Viewer Shell，默认视图渲染出 3 个端点；切换 redoc / source / operations 均正常。

AC：
- [AC1] 从第 1 步到第 5 步全程 ≤ 5 分钟（自动化计时断言 < 300s，人工基准 < 5min）。
- [AC2] `GET /assets/{id}` 返回 1 个 base 层（origin=repo）、layer_manifest 只含该层修订；AssetVersion 状态符合发布策略（默认 autoPublish=false ⇒ draft）。
- [AC3] 再次触发同步且仓库无变化 ⇒ 不产生新 AssetVersion（版本列表长度不变）。
- [AC4] `services/pay` 在目录中显示"openapi 资产缺失"空态与 [用 AI 生成] 入口。
- [AC5] operations 视图行数 = 3，与 openapi.yaml 中端点一致。

### US-03 路径配错的自愈体验

**背景**：`bob` 把 path 配成 `doc/openapi.yaml`（拼错）。

SOP：
1. 修改资产源 path 为错误值，触发同步。
2. 资产源配置页该行出现错误：`asset_path_not_found` + 候选文件列表（含 `docs/openapi.yaml`）。
3. 点击候选 [改为它] → path 自动修正 → 重新同步成功。

AC：
- [AC1] source.last_error.code = `asset_path_not_found` 且 `candidates` 非空、包含正确路径。
- [AC2] 一键修正后无需手工重填其它字段；重新同步后错误清空。
- [AC3] 期间资产保留上一有效版本可访问（health=stale/invalid 不影响历史版本浏览）。

### US-04 人工 overlay：不动仓库补文档

**背景**：`bob` 想给 `GET /orders/{id}` 补业务描述，但不能改仓库。

SOP：
1. 资产页切到 `layers` 视图 → [＋新增层]：role=`overlay`、origin=`manual`、mode=`manual`。
2. [编辑] 打开双栏编辑器，粘贴 `fixtures/overlay-valid.yaml` 内容；右栏实时预览显示合并后该端点已有 description。
3. 保存 → 新修订 active → 自动合并出新 AssetVersion。
4. `source` 视图打开 [层标注]：description 行显示来源为 manual 层。
5. 编辑器中粘贴 `overlay-invalid.yaml` 保存 ⇒ 行级错误标注，未入库。

AC：
- [AC1] 合并后文档中目标端点含新增 description；base 层修订内容不变（读 base 修订原文验证）。
- [AC2] `GET /asset-versions/{vid}/provenance` 中该字段 ptr 归属 manual 层。
- [AC3] 非法 overlay 返回 422 `overlay_invalid`，修订列表无新增。
- [AC4] 关闭该 overlay 层（enabled=false）后新版本不含 description；重新开启后恢复（且两次哈希与历史一致，验证确定性）。
- [AC5] `carol`（viewer）看不到 [编辑]/[新增层] 操作（capabilities 驱动），直接调 API 返回 404。

### US-05 AI 冷启动闭环（经典用法）

**背景**：`services/pay` 没有任何资产。`bob` 用 AI 生成，`carol` 审批。

SOP：
1. `bob` 在 pay 服务空态点 [用 AI 生成]（租户已配置 ai 命令 = `fake-ai-gen.sh`）→ 任务开始。
2. 任务成功后产生 ai_generated base 层修订，状态 `pending_review`；`carol` 收到站内通知，审批中心 badge +1。
3. `carol` 打开审批抽屉：查看修订内容与"合并前后对比"（前=空骨架），点 [通过]。
4. 合并触发，pay 服务出现可浏览的 openapi 文档（draft 版本）；`bob` 手动 publish。
5. 复跑一次生成（内容相同）⇒ 提示 unchanged，无新修订。

AC：
- [AC1] 审批前：资产不可见有效文档（无 AssetVersion 或版本不含 pending 内容）；publish 接口返回 `version_not_publishable`（若强行造版本场景）——核心断言：**pending 修订绝不进入合并结果**。
- [AC2] 审批通过后 AssetVersion 生成且 layer_manifest 引用该修订（status=approved）；publish 成功。
- [AC3] 修订 `ai_meta` 含 jobId 与 promptDigest；审计日志含 approve 记录（actor=carol）。
- [AC4] 驳回路径：驳回必填意见；驳回后修订 rejected、不可再 approve、可重新生成产生新修订。
- [AC5] 幂等：相同产出不新建修订（AC 对应 SOP 步骤 5）。
- [AC6] 信任模式：租户开 `aiTrustMode` 后重跑，修订直接 approved、免审批，但版本仍是 draft。

### US-06 变更、Diff 与 breaking 确认

**背景**：`release/2.0` 分支删除了 `DELETE /orders/{id}`。消费者需要感知，负责人需要确认。

SOP：
1. `bob` 配置仓库跟踪 `release/2.0` 并同步。
2. 资产页 diff 视图：左=main 最新版本，右=release/2.0 分支最新；统计卡显示 1 breaking（端点删除，红色）。
3. 切换 tree / side-by-side / list 三种呈现；按级别筛选只看 breaking。
4. [保存快照] + [分享] 生成链接；匿名打开链接可见同一 diff（只读）。
5. 因新版本含 breaking，`order-service` 负责人收到待办；在 `/todos` 点 [确认知悉] 关闭。

AC：
- [AC1] `POST /diff` 返回 summary.breaking=1，change 项 level=breaking、type=removed、itemKey=`DELETE /orders/{id}`。
- [AC2] 三种呈现均正确渲染同一数据（E2E 断言关键节点存在）。
- [AC3] 分享链接匿名可读、过期后 404；链接不泄漏其它资产访问能力（用链接 token 请求其它 versionId 的 contentUrl ⇒ 404）。
- [AC4] breaking_todo 在版本 index 阶段自动创建，ack 后状态 acked、审计留痕。
- [AC5] 本地文件 diff：上传修改版 openapi 与平台版本对比，结果一致逻辑（uploadRef 路径）。

### US-07 GitOps 配置导入与漂移

**背景**：`fixture-repo-b` 团队习惯在仓库里维护 `.asset-platform.yaml`。

SOP：
1. `bob` 录入 repo-b，向导识别配置文件并展示导入预览（服务 + 资产源树）；确认导入。
2. 服务的 `config_sync_policy` 设为 `sync`；修改仓库内配置文件（改 path）并推送，触发同步。
3. DB 配置被更新为仓库值；`bob` 在平台手工改了某字段（`config_source` 变 `db_manual`）后，仓库再变更该字段 ⇒ 不被覆盖，服务页出现"配置漂移"提示，可 [采纳] 或 [忽略]。

AC：
- [AC1] 导入落库字段带 `config_source=repo_file`；预览与落库一致。
- [AC2] `sync` 策略只覆盖非 `db_manual` 字段（字段级断言）。
- [AC3] 漂移提示出现/消除逻辑正确；采纳后差异清空。

### US-08 检索与全局视图

**背景**：架构师想全局找 `order` 相关条目并看系统依赖关系。

SOP：
1. `alice` 创建 SystemGroup `trade`，把 order-service、pay-service 拖入。
2. 为两服务分别 push `dependency` 资产（CI token，声明 order→pay 边）。
3. ⌘K 搜索 `orders`：条目组命中 openapi operation，点击深链到 Viewer 并定位。
4. 打开 `/t/acme/groups/trade` 依赖图：两个节点一条边；双击节点进服务主页。
5. 资产大盘：覆盖率环图显示 openapi 覆盖 2/2；下钻列表为空。

AC：
- [AC1] 条目检索命中含高亮与正确深链（assetId+versionId+itemKey）。
- [AC2] dep-graph 的 scope resolve 只返回本租户、本分组数据。
- [AC3] `rival` 租户搜索 `orders` 结果为空（隔离下沉到检索层）。
- [AC4] 检索支持分面：`kind=openapi` / `hasAiLayer=true` 过滤结果正确。

### US-09 CI 集成：push 与门禁

**背景**：CI 在构建后推送资产，并在 MR 阶段阻断 breaking。

SOP：
1. CI 用 `ci-token` 调 `POST /assets/push`（third_party origin，overlay 层）→ 修订 pending_review。
2. `assetctl diff --left <本地文件> --right <平台 latest> --fail-on breaking`：有 breaking 时非零退出。
3. `assetctl push` 与 API push 等价（CLI 冒烟）。

AC：
- [AC1] scope 不足的 token（仅 `asset:read`）push 返回 404/403 语义正确（按后端口径 404）。
- [AC2] `createIfMissing=true` 自动建资产；false 且不存在 ⇒ 明确错误。
- [AC3] `--fail-on breaking` 在 fixture breaking 场景退出码非 0，输出含变更清单；无 breaking 退出 0。
- [AC4] push 的修订同样受审批门禁约束（不进合并直至 approved）。

### US-10 订阅与通知

SOP：
1. `carol` 在 order-service 页订阅"新版本发布 + 破坏性变更"。
2. `bob` 发布一个含 breaking 的新版本。
3. `carol` 收到站内信（两条事件合并或分别，按实现约定断言存在性）；通知列表可标记已读。
4. 租户配置一个通用 webhook 渠道 + [测试发送]；发布事件触发出向 webhook（fixture 接收端断言收到）。

AC：
- [AC1] 订阅 upsert 幂等；取消订阅后不再收到。
- [AC2] outbox 投递失败自动重试（fixture 接收端先 500 后 200，最终送达）。
- [AC3] webhook payload 含事件类型、租户、资产、版本、diff 摘要字段。

### US-11 安全与隔离（贯穿）

SOP（自动化为主）：
1. `eve`（rival 租户）持有效会话遍历 acme 的资源 API（仓库/服务/资产/版本/层/修订/任务/审计/检索）。
2. `carol`（viewer）尝试全部写操作 API。
3. 创建 PAT 后二次读取；查看凭证详情。

AC：
- [AC1] 越权访问一律 404，响应体不含资源存在性信息（错误信息不区分"不存在"与"无权"）。
- [AC2] viewer 写操作全部 404；layers/审批操作遵循 `layer:edit`/`layer:approve` 授权。
- [AC3] PAT 明文仅创建响应出现一次；凭证任何 API 不回显私钥/token 明文，仅指纹。
- [AC4] 分享链接只能访问 descriptor 限定内容（US-06 AC3）。

### US-12 失败与恢复（运维视角）

SOP：
1. 把仓库凭证改错触发同步 ⇒ resolve 阶段失败，健康面板 failStreak+1，错误分类为 auth。
2. AI 命令换成 `fake-ai-timeout.sh` ⇒ 任务超时；系统检查 OUTPUT_DIR 无产出 ⇒ failed，不产生半成品修订；换 `fake-ai-bad.sh` ⇒ 校验失败 failed，错误含校验详情。
3. 同仓库快速点 5 次 [立即同步] ⇒ 只执行 1 个任务（去重返回同 jobId）。
4. worker 进程被 kill 后重启 ⇒ 运行中任务被超时回收并重试，最终成功；无重复版本产生。

AC：
- [AC1] 每类失败在任务详情有阶段级错误与可读分类；连续失败 ≥3 资产标 stale。
- [AC2] 超时（结果未知）不产生 pending 修订垃圾数据；重试幂等。
- [AC3] 任务去重与恢复不破坏 I4（版本唯一性）与 I7（单仓库串行）。

---

## 3. 冒烟测试计划（Smoke Suite）

> 冒烟 = 每次合入主干 / 发布前必跑的最小有效集，目标 **15 分钟内**全绿。执行环境：docker-compose 起全栈 + `seed-e2e.sh`。E2E 用 Playwright，API 用 Go testcontainers 或 hurl 脚本。`里程碑` 列表示该用例自哪个里程碑起纳入冒烟。

| 编号 | 名称 | 类型 | 步骤摘要（映射故事） | 通过判据 | 里程碑 |
| --- | --- | --- | --- | --- | --- |
| SMK-001 | 健康与迁移 | API | `migrate up` 空库 → `/healthz` `/readyz` | 200；迁移可 down 再 up | M0 |
| SMK-002 | 登录与租户切换 | E2E | US-01 步骤 1–3 | me 含角色；切换后 URL 前缀正确 | M0 |
| SMK-003 | 越权矩阵 | API | US-11 AC1/AC2 全资源遍历（代码生成的矩阵用例） | 全部 404 | M0 |
| SMK-004 | PAT 生命周期 | API | 创建（scope/过期）→ 使用 → 撤销后调用 | 明文一次；撤销后 404 | M0 |
| SMK-005 | 凭证加密与探活 | API | 创建 ssh/http 凭证 → test → 读取 | 无明文回显；错误分类正确 | M0 |
| SMK-006 | **Golden Path** | E2E | US-02 全程 | 5 个 AC 全过；计时 <300s | M1 |
| SMK-007 | 采集幂等 | API | 同内容二次 sync | 版本数不变（I4） | M1 |
| SMK-008 | 路径未命中自愈 | E2E | US-03 | 候选列表含正确路径；一键修复 | M1 |
| SMK-009 | glob 展开 | API | path=`api/*/openapi.yaml`（fixture 3 文件） | 生成 3 个 Asset 各带 base 层 | M1 |
| SMK-010 | 视图 resolve 契约 | API | 4 种 descriptor 正反例（合法 + mode 不匹配 + kinds 不匹配 + 越界 docs 数） | 合法 200；非法 422 `input_spec_mismatch` | M1 |
| SMK-011 | overlay 合并与溯源 | API | US-04 AC1/AC2/AC4（含确定性重放断言：重放 merge 哈希一致） | provenance 正确；哈希稳定 | M2 |
| SMK-012 | overlay 编辑 E2E | E2E | US-04 步骤 1–5 | 实时预览、行级错误、层标注 | M2 |
| SMK-013 | 层回滚 | API | 回滚 manual 层到旧修订 | 新版本内容=旧合并结果；修订数不变（I6） | M2 |
| SMK-014 | 非法 overlay 拒收 | API | overlay-invalid 提交 | 422，无修订入库 | M2 |
| SMK-015 | **AI 冷启动闭环** | E2E | US-05 步骤 1–5 | AC1–AC5 全过 | M3 |
| SMK-016 | 审批门禁 | API | pending 修订 → merge 输入断言 + publish 谓词 | I2/I3 成立 | M3 |
| SMK-017 | AI 失败三态 | API | US-12 步骤 2（成功/超时/坏产出） | 三种终态正确、无脏数据 | M3 |
| SMK-018 | diff 与 breaking | E2E | US-06 步骤 1–4 | breaking=1、三视图、快照分享 | M3 |
| SMK-019 | breaking 待办 | API | 版本 index → todo 创建 → ack | 状态流转 + 审计 | M3 |
| SMK-020 | CLI 门禁 | CLI | US-09 AC3 | 退出码语义正确 | M3 |
| SMK-021 | push 全路径 | API | US-09 AC1/AC2/AC4 | createIfMissing、审批约束 | M3 |
| SMK-022 | 多 kind 通用链路 | API | dbschema push → items 抽取 → 通用 diff | 新 kind 免费获得四通用视图数据 | M4 |
| SMK-023 | 分组与依赖图 | E2E | US-08 步骤 1–4 | 图渲染 + 深链 + 隔离 | M4 |
| SMK-024 | 检索隔离与分面 | API | US-08 AC3/AC4 | 跨租户零命中；分面正确 | M4 |
| SMK-025 | 订阅通知投递 | API | US-10 AC1/AC2 | 重试后送达；退订生效 | M5 |
| SMK-026 | 分享链接安全 | API | US-06 AC3 + 过期/撤销 | 限定内容；过期 404 | M5 |
| SMK-027 | 任务去重与恢复 | API | US-12 步骤 3–4 | I7 去重；kill 恢复无重复版本 | M1 |
| SMK-028 | GitOps 漂移 | API | US-07 AC1–AC3 | 字段级覆盖规则正确 | M2 |
| SMK-029 | 配额限制 | API | 造满 maxRepositories 后再建 | 409 `quota_exceeded` 明确提示 | M0 |
| SMK-030 | 体验模式启动 | 部署 | 单容器镜像启动 → 示例租户可浏览 | 5 分钟内首文档（US-02 AC1 的部署版） | M5 |

---

## 4. API 反向用例与专项断言

> 冒烟表覆盖正路径 + 关键负路径；本章列出必须逐条落实现的专项断言，供 AI 开发者据此写 API 级用例（编号可作为测试函数名后缀）。

### 4.1 错误码契约（每条一个用例）

| 编号 | 触发方式 | 期望 |
| --- | --- | --- |
| API-001 | builtin 源 path 未命中 | `asset_path_not_found` + candidates ≥1 |
| API-002 | 提交 target_kind 不匹配 overlay | 422 `overlay_invalid` + 行级错误 |
| API-003 | publish 含 pending 修订版本 | 409 `version_not_publishable` + 阻塞修订列表 |
| API-004 | 创建第二个 base 源 | 409 `base_layer_exists` |
| API-005 | descriptor 引用未采集分支 | 422 `branch_not_indexed` |
| API-006 | 超配额建仓库 | 409 `quota_exceeded` |
| API-007 | 移除最后一个 tenant_admin | 409 `last_admin` |
| API-008 | 删除被引用凭证 | 409 `credential_in_use` + 引用列表；`?force=true` 成功 |
| API-009 | ai 源无命令配置时触发生成 | 422 `ai_command_missing` |
| API-010 | descriptor 与 ViewDef 不匹配（4 种） | 422 `input_spec_mismatch` |
| API-011 | reject 缺 comment | 422 校验错误 |
| API-012 | SystemGroup 嵌套超一级 | 422 `nesting_too_deep` |

### 4.2 状态机断言

| 编号 | 断言 |
| --- | --- |
| API-020 | 修订状态只允许 `pending_review→approved/rejected`；对 active/rejected 调 approve ⇒ 404/409，无跃迁 |
| API-021 | 生命周期只允许 `draft→published→deprecated→retired` + `deprecated→published`；非法跃迁拒绝 |
| API-022 | rejected 修订永不出现在任何 layer_manifest（造数据后全表断言） |
| API-023 | 层 enabled=false 后触发 merge，manifest 不含该层 |

### 4.3 确定性与溯源（平台核心资产，重点投入）

| 编号 | 断言 |
| --- | --- |
| API-030 | 固定 3 层输入重放 merge 100 次哈希一致（进 CI 基准） |
| API-031 | overlay 覆盖 base 同字段 ⇒ provenance 归 overlay；未覆盖字段归 base |
| API-032 | lenient 模式 target 零命中 ⇒ warnings 含记录、其余 action 正常应用；strict 模式 ⇒ 整体失败无版本 |
| API-033 | OpenAPI Overlay 1.0 与 platform/v1 表达同一变更 ⇒ 合并结果哈希一致（双方言等价样例） |
| API-034 | 层顺序交换 ⇒ 冲突字段归属随 last-write 变化（order 语义） |

### 4.4 隔离矩阵（生成式用例）

- 对 §7 接口表**每个带路径参数的端点**自动生成三元组用例：`(acme资源ID, eve会话)`、`(acme资源ID, rival PAT)`、`(不存在ID, alice会话)` ⇒ 全部 404 且响应体一致（防存在性泄漏）。此矩阵作为 CI 固定关卡（对应 SMK-003）。

### 4.5 性能守护（基准测试，非冒烟但发布必跑）

| 编号 | 指标 | 阈值 |
| --- | --- | --- |
| PERF-01 | merge P95（1MB 文档 + 10 层） | < 200ms |
| PERF-02 | 条目检索 P95（50 万条目种子） | < 500ms |
| PERF-03 | 目录页 API P95 | < 300ms |
| PERF-04 | dep-graph resolve（500 节点/5000 边） | < 2s |

### 4.6 防"假实现"抽查清单（评审用）

- [ ] 前端隐藏按钮的每个写操作，直接 curl 后端仍被拒（对照 capabilities 与 404）。
- [ ] publish 谓词、审批门禁、越权判断均只在后端实现（前端源码 grep 无重复业务判定）。
- [ ] merge/provenance 无随机性来源（代码审查：时间/map 遍历/随机数进 merge 路径 = 违规）。
- [ ] 用例断言的是**业务结果**（版本哈希、manifest、provenance、DB 状态），不是 mock 调用次数。
- [ ] E2E 不依赖公网；fixture 可离线重建。

---

## 5. 执行与报告约定

- 目录：`e2e/`（Playwright 场景，按 US 编号组织）、`tests/api/`（API 用例，按 API/SMK 编号命名）、`scripts/seed-e2e.sh`。
- CI 分层：PR = 单测 + 受影响 SMK；主干合入 = 全量 SMK；发布 = 全量 SMK + 隔离矩阵 + PERF。
- 报告：CI 输出按本文编号的通过矩阵；任何 SMK 失败阻断发布。里程碑验收会以本文档编号逐条打勾为准。
- 本文档为验收权威；实现与文档冲突时，先改文档评审再改实现，不允许"以实现为准"倒推。

## 6. 专业名词表

| 统一名称 | 英文/代码标识 | 一句话口径 |
| --- | --- | --- |
| 黄金路径 | Golden Path（US-02/SMK-006） | 录入仓库→首个文档的核心价值路径，5 分钟硬指标 |
| 种子数据 | `seed-e2e.sh` | 所有用例统一前置数据脚本，离线可重建 |
| 隔离矩阵 | isolation matrix（§4.4） | 对全部资源端点生成式越权用例，404 一致性关卡 |
| 假实现 | fake implementation（§4.6） | 仅前端/仅 mock 层面满足表象而后端无真实约束的实现，验收禁止 |
| 双方言等价样例 | dialect equivalence fixture（API-033） | 用两种 overlay 方言表达同一变更、断言合并哈希一致的测试资产 |


