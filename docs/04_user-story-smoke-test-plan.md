# Meridian 用户故事与验收计划

> 状态：可执行基线（2026-09-04）
> 需求来源：[01_requirement-specification.md](./01_requirement-specification.md)
> 验收权威：[contracts/acceptance.yaml](../contracts/acceptance.yaml)
> API operation、fixture、里程碑和断言 ID 只在 YAML 中维护；本文解释用户目标与人工验收路径。

## 1. 完成定义

功能只有同时满足以下条件才完成：

1. 对应用户故事的全部 assertion 有自动化用例并通过；
2. 对应里程碑的 Smoke 和 API 负向用例通过；
3. OpenAPI 生成的前后端代码无漂移；
4. 状态、权限、发布条件和 diff 均由后端判定，前端没有假实现；
5. 每个故事在独立 fixture profile 中运行，不依赖其它故事的执行顺序；
6. 审计、outbox、job、blob 和数据库没有半成品或跨租户泄漏。

`bootstrap-only` 只含初始平台管理员，供 US-01 创建首租户。`standard` 已含 acme/rival、五个用户和两个 fixture 仓库，供其它故事使用。测试框架每例重建场景或使用独立 schema；不得一边预置 acme、一边再次断言创建 acme。

关键角色：padmin 仅平台控制面；alice 是 acme tenant admin；bob 是 maintainer；carol 是 viewer，并分别拥有 order/pay 的 `layer:approve`；eve 只属于 rival。

## 2. 用户故事

### US-01 平台初始化与首租户

目标：全新部署后，平台管理员创建用户和租户，并交付首位租户管理员，但不能借平台身份读取租户正文。

SOP：

1. 以 padmin 登录，在平台控制面创建 alice；
2. 创建 acme，绑定 alice 为 tenant admin；
3. alice 登录并进入 acme；
4. padmin 查询平台审计，再尝试访问 acme 业务正文；
5. 验证重复 slug 和停用租户路径。

验收：alice 的 me 响应包含 active acme membership；padmin 能看脱敏审计但正文为 404；重复 slug 为 409；停用后 acme 从 alice 可用租户中消失。

### US-02 五分钟 Golden Path

目标：维护者从连接仓库到浏览首份 OpenAPI 不超过五分钟。

SOP：

1. padmin 注册受控 producer profile，bob 只能看到已启用且依赖可用的 profile；
2. bob 用无凭证方式测试并保存 fixture-repo-a；
3. 发现 `services/order` 和 `services/pay`，批量接受；
4. 为 order 创建 repo/builtin/base SourceSpec，路径为 `docs/openapi.yaml`；
5. 同步并观察六阶段 Job；
6. 打开 order Asset，再打开返回的 Track latest AssetVersion；
7. 查看默认、Redoc、source、operations，operations 应为三个；
8. 仓库不变时立即再次同步。

验收：总耗时小于 300 秒；Asset 有一个 repo base；`layerManifest` 从 AssetVersion 读取；第二次同步 revision/version 数都不变；pay 返回 missingKinds 的 openapi AI 入口。独立生命周期场景还要验证 public draft/retired 为 404、published/deprecated 可读、deprecated 事件只产生一次、retired 写入为 409，且 Service 流转不改 AssetVersion 生命周期。删除场景验证完整逻辑级联、pending 取消、running fence、内部历史保留但所有外部路由 404。

### US-03 路径错误自愈

目标：路径配错后，用户能定位候选并恢复，同时历史文档不断供。

SOP：把 order 路径改成 `doc/openapi.yaml` 并同步；检查 `asset_path_not_found` 和稳定排序候选；选择正确路径重跑。

验收：首次失败立即将已有历史的资源标 stale，而非等三次；current version 仍可读；正确候选存在；成功后 lastError 和 stale 清除。若从未有有效版本，则 health 为 invalid。

### US-04 人工 Overlay 与历史回退

目标：不改仓库补充描述，可证明字段来源，可关闭、重开和回滚。

SOP：

1. 新建 manual/overlay Layer，编辑合法 overlay 并 preview；
2. 保存修订，等待新版本，读取 provenance；
3. 提交非法 overlay；
4. 关闭并重开 Layer；
5. rollback 到历史 effective 修订。

验收：base 原文不变；description 指向 manual revision；非法内容 422 且不入库；即时相同 fingerprint no-op；A -> B -> A 产生新 versionId，version 数 +1、revision 数不变，merged hash 允许等于历史值；viewer 无编辑能力且直接调用为 404。

### US-05 AI 冷启动、审批与发布

目标：pay 完全缺失资产时，维护者能生成待审 base，审批后才出现第一版文档。

SOP：

1. bob 在 pay missing kind 上选择 fake-ai-success profile；
2. 任务创建 Asset/AI base/pending candidate，carol 收到最小待审通知；
3. carol 查看 review context 并 approve；
4. 观察 merge job，bob publish draft；
5. 重复相同生成；另测 reject 后再次生成同内容；
6. 开启 trust mode 后另起独立场景。
7. 在独立场景给已审批 AI base 接入 repo base：先验证未带 flag 返回 `base_layer_exists`，再用 `replaceAiBase=true` 创建。

验收：审批前没有 AssetVersion，pending 不在 manifest；approve 返回 mergeJobId；审批后版本引用 approved revision；相同 effective/candidate 返回 unchanged；曾 rejected 的相同内容可以形成新 review attempt；信任模式只免审批，仍不得自动 publish；repo base 替换必须显式确认，旧 AI base 被归档且历史可读，不产生双 base 或把完整 AI 文档误作 overlay。

### US-06 分支 Diff、快照、分享与待办

目标：release 分支首次同步即可相对 main 识别 breaking，并形成不可漂移的分享和负责人待办。

SOP：

1. 先确保 main 有 current published 版本；
2. 同步 `release/2.0` 首版；
3. 比较 main 与 release，检查删除 `DELETE /orders/{id}`；
4. 保存 snapshot，导出并创建短期 share link；
5. 匿名读取，同 token 尝试访问其它 version；
6. owner ack 自己的 todo。

验收：release 首版 `baselineVersionId` 固定为同步开始时 main current，否则 main latest；breaking=1；tree/side-by-side/list 使用同一结果；分享冻结且只能访问 descriptor 白名单；每个展开后的用户 owner 恰好一条 todo。

### US-07 GitOps 导入与字段级漂移

目标：仓库配置可同步，同时人工改过的字段不被静默覆盖。

SOP：对 fixture-repo-b 先 preview 再 apply；变更 repo_file 字段并同步；在 UI 手改该字段；仓库再次变更；分别执行 take_file、keep_db、ignore。

验收：apply 使用相同 commit/configDigest；repo_file 可更新，db_manual 不覆盖；keep_db 固定人工来源；ignore 绑定当前 file digest，文件内容再次变化后漂移重新出现；配置文件服务根 `.` 在 preview 前规范化为 API/DB 空 `rootDir`，后续同步不产生伪漂移。

### US-08 搜索与系统级视图

目标：架构师从条目检索进入固定版本位置，并查看限定系统范围的依赖与覆盖率。

独立 setup：order/pay 均先有 approved published openapi；dependency push 后必须先 approve，禁止依赖 US-05 或 US-09 的残留状态。

SOP：创建 trade SystemGroup 并加入两个服务；推送并审批 dependency；搜索 orders 并打开 deep link；查看依赖图和覆盖率。

验收：deep link 固定 assetId/versionId/itemKey；图只含 acme/trade 成员；两个节点一条边；openapi 覆盖为 2/2。

### US-09 CI Push 与门禁

目标：CI 用最小权限、幂等地推送第三方修订并获得稳定 diff 退出码。

SOP：用 bob PAT push；检查 pending review；分别执行 `--fail-on breaking` 和 `--fail-on risky`；测试错误 scope 和 `createIfMissing=false`。

验收：无 scope 为 404；资源不存在且禁止创建为 404；third-party 默认 pending；breaking 命中退出码 3；risky 同时匹配 risky 和 breaking；稳定 sourceSystem/Idempotency-Key 重试不重复修订。

### US-10 订阅、站内信与 Webhook

目标：用户订阅目标和事件，站内与 webhook 可恢复且不重复解释业务事件。

SOP：upsert 订阅；发布 breaking 版本；检查 inbox 与 webhook；模拟 5xx 后重试；取消订阅后再发布。

验收：订阅唯一；一次 breaking publish 产生 `version.published` 和 `version.breaking` 两个 eventId、两条通知；webhook 重试保持 eventId；消费者据此去重；取消只影响新事件；secret 只在创建/轮换时显示一次。

### US-11 隔离、秘密与公开能力

目标：无权者无法推断资源存在，secret 不回显，公开与分享能力不能升级。

SOP：生成跨租户/不存在资源矩阵；撤销 PAT；读取 credential/PAT；由 padmin 创建、轮换和删除 global credential，alice 从租户可选凭据列表使用它；手工录入 RFC4253 known host 并提交伪造派生字段、非法 key 和重复 key；访问 public 服务和 private 服务；用 share token 请求 descriptor 外内容；执行 viewer 的业务管理写和 self-service 写。

验收：未认证与撤销 PAT 为 401 `unauthenticated`；已认证无权/不存在均为 404，比较 body 时排除 requestId；秘密不回显，SSH/HTTP fingerprint 分别按冻结算法服务端派生，相同秘密轮换为 422；global credential 仅平台管理员可维护、租户仅可选择，轮换 job 逐项携带 tenantSlug/repositoryId/jobId，强制删除后引用仓库原子解绑并标记需认证；KnownHost 只信任 publicKey 并服务端重算，非法 422、重复 409；public 只暴露 current published merged 视图；viewer 的 layer/repo/service 管理写为 404，但订阅、通知已读和自己的 todo ack 成功。

### US-12 失败恢复与并发

目标：任务失败可诊断、可恢复，而且不会重复副作用或漏掉运行期间的新变更。

SOP：分别触发路径失败、AI timeout、非法 AI 输出、同步重复请求、相同 Idempotency-Key 的首次并发与不同 body、运行中再次变更、worker crash 和 retry。

验收：单 source 失败结构化记录并保留旧 effective；多个 source 部分成功时根任务为 `succeeded_with_warnings`；无 completion marker 不创建 revision；replay-unsafe 为 `outcome_unknown`；同 repo+ref 业务去重、同 repo 工作区互斥；相同幂等摘要的首次并发只提交一次且响应相同，不同摘要返回 409；运行中变更提升 desired generation，当前任务结束后恰好一个 successor 追平；同 mergeRequestId 不重复版本。

## 3. Smoke 执行

Smoke ID、里程碑、用户故事和 assertion 的完整映射在 `acceptance.yaml`。固定执行方式：

1. 解析 manifest 和所有 YAML，校验 OpenAPI 本地引用与 operationId；
2. 从空库通过运维命令运行 migration up/down/up；应用启动只允许 auto-up，迁移期间 readiness 必须为 false；
3. 为每个 SMK 建独立 fixture，不共享状态；
4. 通过生成 client 发请求，前端 E2E 不直接拼 DTO；
5. 每例输出 assertion ID、requestId、jobId、versionId 和失败证据；
6. 全套 Smoke 目标 15 分钟内完成，体验基准图像测试可独立 nightly 运行。

本地和 CI 的唯一入口是 `make smoke SMK=SMK-xxx`；当前阶段累计回归使用 `make smoke-all MILESTONE=M0`（或 `ITEM=M0-AGENT-003`），无参数 `make smoke-all` 表示 M0-M5 全套。runner 由各工作项逐步补齐 fixture，输出带时间目录的不可覆盖报告，以及 `artifacts/smoke/<smk>.json` 最新索引与 JUnit XML。当前凭据三项使用隔离 PostgreSQL、本地 TLS Git 和 SSH fixture；其它未实现条目返回 `tooling_gap`，由所属工作项实现，不是外部阻塞。零测试、跳过或只覆盖部分断言的测试不得作为 Smoke 通过证据；后续 webhook/fake producer 等 fixture 随对应工作项接入。

重点回归：SMK-013 必须断言 rollback 后 version +1/revision 不变；SMK-016 必须断言 candidate 阻止 publish；SMK-027 必须断言 successor 实际处理最新 generation，而不只是“同时一个 worker”；SMK-034 必须断言 public/lifecycle 门禁、deprecated outbox 事务与 retired 运行中任务 fencing；SMK-035～038 分别锁定 known host 派生、幂等首次并发、Service 删除和仓库根目录规范化。

## 4. API 反向矩阵

每个有权限 operation 至少生成以下适用组合：

| 维度 | 预期 |
| --- | --- |
| 无认证 / 撤销 PAT | 401 `unauthenticated` |
| 已认证但跨租户、无 capability、资源不存在 | 404 `not_found` |
| 重复唯一资源 | 409 `duplicate` |
| 非法状态迁移 / 旧 candidate approve | 409 `invalid_state` |
| ETag 或 expected head 过期 | 412 `precondition_failed` |
| 内容超限 | 413 `content_too_large` |
| body/descriptor/overlay 语义非法 | 422 对应契约错误码 |
| 可重试写入重复 Idempotency-Key | 返回原资源或 job，`deduplicated=true` |

审批状态迁移、生命周期、LayerHead、Track 以及 job 状态的组合从 `domain.yaml` 生成，不在测试代码手写第二套。状态属性测试至少覆盖非法逆向边和 terminal 状态。

## 5. 测试数据与副作用

- Git fixture 使用本地 bare repository 或容器内 Git，不依赖公网；
- AI fixture 通过平台 producer profile 注册，测试请求不得传任意命令；
- 每个 producer 在空输出目录写 completion manifest；timeout/invalid fixture 明确控制 replaySafe；
- 邮件/webhook 使用录制 server，验证签名、eventId 和重试，不调用真实外部系统；
- 时间、UUID 和 job 调度器可注入，保证过期、backoff 和并发断言稳定；
- teardown 只删除当前 case 的 schema、blob prefix 和临时 repo；失败时保留路径及 manifest 用于诊断。

## 6. 里程碑放行

| 里程碑 | 必过故事/范围 |
| --- | --- |
| M0 | US-01、US-11 的身份/隔离/PAT/配额基础 + 05 文档第 8 节六项 spike |
| M1 | US-02、US-03、US-12 的仓库主链、互斥与恢复基础 |
| M2 | US-04、US-07 的 Layer/overlay/字段级 GitOps |
| M3 | US-05、US-06、US-09、US-12 的 AI 失败恢复，含 diff share、todo、最小审批通知 |
| M4 | US-08，多 kind/search/group/global views |
| M5 | US-10，通用订阅/inbox/webhook 与合规加固 |

任一契约引用未解析、operationId 缺失、fixture 依赖其它故事、断言仍含“任选其一/可能/或”等非确定语义时，不得放行。
