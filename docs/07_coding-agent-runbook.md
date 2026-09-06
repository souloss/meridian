# Meridian Coding Agent 长程迭代运行手册

> 目的：让 Coding Agent 在无人值守或低频人工介入的情况下，按契约持续交付可验收的增量。
>
> 本文是执行规程，不定义产品行为。产品和技术行为以 [`contracts/manifest.yaml`](../contracts/manifest.yaml) 及其列出的子契约为准；用户故事、Smoke 和断言以 [`contracts/acceptance.yaml`](../contracts/acceptance.yaml) 为准；人工可读进度写入 [`06_implementation-progress.md`](./06_implementation-progress.md)。

## 1. 权威顺序与执行边界

Agent 每次开始工作都必须读取以下内容，并记录本次执行的 git commit 和契约摘要（contract digest）：

1. `contracts/manifest.yaml`：契约所有权、工具链、必需门禁和里程碑。
2. manifest 引用的相关契约：`openapi.yaml`、`domain.yaml`、`storage.yaml`、`events.yaml`、`views.yaml`、`kinds.yaml`、`cli.yaml`、`repository-config.schema.yaml`、`acceptance.yaml`。
3. `docs/01_requirement-specification.md`、`docs/02_backend-technical-design.md`、`docs/03_frontend-technical-design.md`、`docs/04_user-story-smoke-test-plan.md`：只用于理解背景和实现边界。
4. `docs/06_implementation-progress.md`：只用于了解已有实现和证据，不得把叙述中的“已完成”当作验收通过。

发生冲突时停止当前工作项，运行契约校验并报告冲突；不得自行猜测或用 Markdown 覆盖 YAML。契约变更必须单独作为工作项，经过兼容性规则和人工验收后才能继续依赖它的实现。

## 2. 工作项协议

长程开发的机器可读队列为 `contracts/work-items.yaml`。`06_implementation-progress.md` 是该队列的人工可读投影；如果两者不一致，以队列和 git 中最新证据为准。每个工作项至少包含：

```yaml
id: M0-W012
milestone: M0
title: 凭据与 Known Host 完整 Smoke
dependsOn: [M0-W011]
operationIds: [createCredential, rotateCredential, createKnownHost]
stories: [US-11]
assertions: [SMK-005, SMK-035]
scope:
  files: [internal/handler, internal/service, web/app]
  exclusions: [M1 asset pipeline, mobile visual polish]
verify: [contracts-validate, backend-test, smoke-m0-credentials]
requiredArtifacts: [artifacts/smoke/SMK-035.json]
status: ready
owner: coding-agent
attempt: 0
maxAttempts: 3
leaseUntil: null
blockedReason: null
failureKind: null
reportPath: null
lastEvidence: null
nextAction: claim
```

`make smoke`、`make smoke-all` 和 `make quality-gate` 是工作项 `M0-AGENT-003` 的交付物。队列中的 `verify` 必须是 Make target 名称数组，不得写裸 shell 命令或临时替代命令。每个门禁按 `make <target> ITEM=<work-item-id>` 执行，领取前按 `make -n <target> ITEM=<work-item-id>` 做存在性预检，并把完整命令写入报告。Make target 可以封装仓库内脚本；需要额外参数的通用命令须由具名 target 固定参数，不得在 `verify` 中含糊引用。

`make agent-preflight ITEM=<ID>` 只诊断声明入口，输出 `tooling_gap` 不表示需要人工，也不是验收通过。`make contracts-validate` 已校验队列依赖、阶段验收记录及 Smoke 所属映射。`make smoke-all ITEM=<ID>` 从 ID 推导累计阶段，也可显式设置 `MILESTONE=M0`；无 ITEM/MILESTONE 时检查 M0-M5 全套。`scripts/smoke-cases.json` 注册全部 Smoke，尚未有 fixture 的条目必须失败并指出归属工作项，不能删除。扩展新 fixture 时同步扩展 runner 与防假通过测试；零测试、skip、失败和执行中源码变化均不能放行。

Smoke 的 `contractDigest` 对 `contracts/*.yaml` 按文件名排序后，以 `文件名 + NUL + 内容 + NUL` 计算 SHA-256；排除仅记录执行状态的 `work-items.yaml`。`sourceDigest` 记录当前实现树内容，包含未提交源码。`workingTreeDirty=true` 的预检报告不能冒充已提交稳定 checkpoint。

若声明的 target、fixture 或报告脚本缺失，但可以在当前仓库和工作项范围内实现，分类为 `tooling_gap`：它是当前工作项的交付物，Agent 必须先补齐真实断言和执行入口再验证，不能因预检缺失而拒绝领取。不得删除门禁、弱化断言或将只覆盖部分场景的单测包装成通过。保留失败证据；有实际修复后才进入下一次尝试，重置租约不等于重置 `attempt`。只有缺少外部权限、外部服务、产品决策或人工环境输入时才使用 `blocked`，技术重试耗尽使用 `failed`，均不能冒充待验收成品。

工作项必须足够小，使一次执行可以完成实现、验证和提交。一个工作项最多绑定一个紧密的用户目标、相关 operation 和断言集合；若同时涉及后端、前端和迁移，仍须能在一次 checkpoint 中验证完整行为。不能创建只有“完善体验”“补齐剩余功能”而没有断言和命令的工作项。

## 3. 状态机、租约与并发

工作项自动状态和里程碑人工验收状态分离：

```text
planned -> ready -> claimed -> in_progress -> verifying -> passed
                                  |              |
                                  v              v
                              needs_retry <------+
                                  |
                                  +--> claimed（有实际修复且未耗尽尝试）
                                  +--> failed（技术重试耗尽）
                                  +--> blocked（缺少外部输入）

milestoneGates: pending -> needs_human_acceptance -> accepted
                                      |
                                      +--> changes_requested -> pending（新增修复工作项）
```

- `planned`：尚需细化的工作项；`ready`：定义完整且可参与筛选，但只有依赖满足才能领取。同里程碑依赖 `passed` 即可；跨里程碑还必须满足前一里程碑 `milestoneGates.<M#>.status=accepted`。
- `claimed`：Agent 已写入 `owner`、`attempt`、`leaseUntil`；租约默认 30 分钟，每完成一个阶段续租。租约过期后，其他 Agent 可恢复，但必须先检查最新 HEAD、工作区和证据。
- `in_progress`：允许修改代码；`verifying`：禁止继续扩展范围，只运行声明的验证。
- `passed`：全部声明自动门禁通过且断言证据完整，可以继续同里程碑依赖项，但不能自动放行下一里程碑。
- `needs_retry`：有下一次有效修复尝试；`failed`：技术失败达到 `maxAttempts`，保留 `failureKind` 并向用户报告技术升级请求，不等于产品待验收；`blocked` 只用于产品决策、外部权限或环境输入。
- `needs_human_acceptance`、`accepted` 是 `milestoneGates` 的状态；队列 `statuses` 中同名工作项值仅兼容历史记录（见 `legacyWorkItemStatuses`），新执行不得写入。`superseded`/`cancelled` 只能用于明确获准的范围替代/取消，必须有替代项和验收映射，不能用来移除未通过的门禁。

同一时刻一个工作项只能有一个有效租约。Agent 在开始写入前执行 `git status --short`，确认文件所有权并将工作项 ID 写入执行报告。发现不属于自己的改动时保留它们，缩小文件范围；只有无法安全避让的实际冲突才暂停并请求协调，不得因无关改动阻塞整个队列或重置工作区。

## 4. 确定性选项算法

每次领取工作项按以下顺序执行，排序相同则使用字典序工作项 ID：

1. 运行 `make contracts-validate`；失败时只处理契约/工具链门禁工作项。
2. 找到最早尚未 `accepted` 的里程碑（`M0` 至 `M5`）。若它是 `needs_human_acceptance`，发送阶段报告并停止领取；若为 `changes_requested`，先把用户反馈转为该阶段的修复工作项并置回 `pending`。
3. 仅在该里程碑过滤 `status=ready/needs_retry`、租约未占用且所有 `dependsOn` 已满足的项，先处理阻塞依赖链的 `needs_retry`，再按 `priority`、ID 排序。同里程碑所有项通过后进入第 8 节，不得越过阶段验收。
4. 每次重试必须记录代码、工具链、环境的有效变化或能推动修复的新诊断；同一未改变的根因不得空跑三次。达到 `maxAttempts` 后转 `failed` 并报告，不得无限重置计数。
5. 领取前检查 operation 是否仍为 501、assertion 是否已有有效证据、代码是否已被其他提交覆盖。即使实现已存在也必须运行本项声明门禁；不能仅凭已有代码标记 `superseded`。

Agent 不得因为“看起来简单”跳过前置工作项，也不得跨里程碑实现未声明的功能。每次只领取一个工作项；并行开发必须使用独立分支或 worktree，并在报告中声明依赖提交。

## 5. 单工作项执行循环

### 5.1 开始阶段

将状态改为 `claimed`，递增 `attempt`，记录 `startedAt`、当前 HEAD、工具链版本（Go/Node/pnpm）和 contract digest。读取对应 operation 的 OpenAPI 定义、领域规则、存储边界和 acceptance fixture；列出本项明确不做的内容，尤其是移动端视觉细节。已有工作项跨度过大时可拆成同阶段、有明确断言的子项，但原项保持未通过，直到所有原始断言和门禁一起闭环。

### 5.2 实现阶段

先写或更新能表达断言的领域/集成测试，再实现最小生产代码。所有 API 类型从生成代码获得；不得手写 DTO、绕过权限、用假成功或把未实现 operation 伪装成 200。迁移必须同时包含 up/down、列注释和回滚测试；长任务必须使用持久化 Job 状态作为事实源。

### 5.3 验证阶段

将状态改为 `verifying`，按工作项的 `verify` 数组顺序执行 `make <target> ITEM=<ID>`，并保存 stdout、stderr、退出码和耗时。另需执行以下通用基线；这些基线不能替代工作项门禁：

```sh
make contracts-validate
make contracts-generate-then-git-diff-exit-code
go test ./...
go vet ./...
git diff --check
```

涉及数据库时追加空库 `up -> down -> up`、事务回滚和 DDL 注释审计；涉及前端时追加生成、typecheck、Playwright 和 axe。涉及某个 Smoke 时必须运行该 Smoke 的独立 fixture；不能用单元测试替代 acceptance 断言。

跨越尚未实现 operation 的累计安全、权限或覆盖矩阵按里程碑分层执行：中间里程碑只验证 `acceptance.yaml` 明确列出的代表性接口或截至该里程碑已实现范围，M5 在全部冻结 operation 实现后执行完整矩阵。阶段性 Smoke 与最终全量回归必须使用不同 assertion/Smoke ID 并分别进入对应工作项，不能让 M0 为 M1-M5 的未实现 operation 伪造 404，也不能把 M0 的代表性结果冒充最终全量结果。以后发现同类跨阶段断言时，先建立独立契约修复工作项，把阶段范围和最终回归写成机器可读映射，再继续实现。

M0-M3 默认 desktop-first：先交付固定桌面主路径，再优化移动视觉、动画、精细间距和非关键响应式排版，后者可以列入 `exclusions` 并创建后续工作项。功能、权限、错误状态、键盘可用性和 A11y 不能后置；已有契约必需的移动功能断言也不能豁免。M3 放行前，桌面完整用户流程必须可实际演示。

### 5.4 通过、失败与提交

- 全部命令通过且断言有证据：生成结构化报告，更新队列为 `passed` 和 `06_implementation-progress.md`，提交 checkpoint。同阶段仍有项则继续；全阶段通过则生成阶段报告，等待人工放行。
- 命令失败：保留失败产物和最短复现命令，按第 6 节分类；不得以“环境偶发”直接标记通过。
- 提交前必须重新生成代码并确认无生成漂移。一个 checkpoint 只包含一个工作项及其进度/证据，不混入下一项或他人未提交修改。

建议提交格式：`test(M0-W012): complete credential smoke coverage`。提交信息必须包含工作项 ID；报告记录完整 SHA，不能只记录短标题。

## 6. 失败、阻塞和恢复规则

失败按以下类别处理：

| 类别 | 例子 | 动作 |
| --- | --- | --- |
| `code_failure` | 单测、类型检查、Smoke 断言失败 | `needs_retry`，保留日志；最多 `maxAttempts` 次 |
| `tooling_gap` | Make target、fixture 或报告脚本缺失且可在仓库内补齐 | 在当前工作项修复后 `needs_retry`；不计为外部阻塞 |
| `contract_failure` | `$ref`、operationId、生成漂移或契约冲突 | 暂停依赖项，创建契约修复工作项 |
| `environment_failure` | 端口、数据库、工具链或权限不可用 | 先记录预检并修复仓库可控配置；只有缺少外部输入才 `blocked`，仓库内技术失败按尝试上限升级为 `failed` |
| `flaky_failure` | 同一命令一次失败、一次成功 | 不得吞掉；增加重试证据，连续两次失败按 `code_failure` |
| `product_decision` | 需求存在两个有效解释 | `blocked`，报告选项和受影响 operation，不自行决定 |

恢复时先读取最后一份报告，验证报告中的 HEAD 仍在当前分支祖先链上，再检查契约 digest、sourceDigest 和工作区。若相关源码或产品契约已经改变，旧证据标记为 `stale`，相关命令必须重跑；仅追加报告/进度的提交不使测试过的代码失效。worker 或 Agent 崩溃后，使用最后一个 checkpoint 恢复；未提交改动只能在同一租约内继续，否则先保存诊断补丁，不得覆盖。

技术失败达到 `maxAttempts` 后标记 `failed`，填写 `failureKind` 并发送最短复现和已尝试修复的技术升级报告；需要外部输入时使用 `blocked`。`tooling_gap` 必须先在仓库内实际修复，不能只重复缺失命令；修复仍失败同样受尝试上限约束。重置 `attempt` 须保留历史并有明确用户授权，不能由 Agent 自行循环。本次用户授权 `M0-AGENT-002` 从旧阻塞恢复到 `needs_retry/attempt=0` 的原因记录在队列 `recovery`，不构成未来无限重试授权。

## 7. 证据与报告

每次工作项必须生成：

- `artifacts/agent/<work-item-id>/<timestamp>/report.json`：机器读取的最终结果；
- `artifacts/agent/<work-item-id>/<timestamp>/commands.log`：命令、退出码、耗时和工具链；
- 失败时保留 fixture、数据库迁移日志、Job 阶段日志或 producer completion manifest；敏感值必须脱敏。

`report.json` 最小结构（字段名与 `contracts/work-items.yaml:evidence` 一致）：

```json
{
  "workItem": "M0-W012",
  "status": "passed",
  "commit": "<full-sha>",
  "baseCommit": "<full-sha>",
  "contractDigest": "sha256:<digest>",
  "startedAt": "<UTC timestamp>",
  "reportPath": "artifacts/agent/M0-W012/<timestamp>/report.json",
  "stories": ["US-11"],
  "assertions": [{"id": "...", "status": "passed", "evidence": "..."}],
  "commands": [{"command": "make smoke-m0-credentials ITEM=M0-W012", "exitCode": 0, "durationSeconds": 42, "stdoutPath": "...", "stderrPath": "..."}],
  "changedFiles": ["..."],
  "deferred": ["mobile visual polish"],
  "failures": [],
  "failureKind": null,
  "nextAction": "claim-next"
}
```

向用户报告时使用以下模板，确保每次都能独立验收：

```text
工作项：M0-W012 / 标题
结果：passed | needs_retry | blocked | failed
提交：<full SHA>（基于 <base SHA>）
本次实现：<用户可观察行为，最多三条>
验收映射：US-xx；SMK-xxx；assertion-id
验证：<命令> -> <结果>；证据：<路径>
未完成/延期：<明确列出，不使用“基本完成”>
风险或需要人工决定：<没有则写“无”>
下一工作项：<ID、依赖和预计验收目标>
```

## 8. 人工验收节点与里程碑放行

Agent 在以下节点必须暂停领取后续里程碑，并发送报告供人工验收：

| 节点 | 必须通过的范围 | 人工重点 |
| --- | --- | --- |
| M0 | US-01、US-11、M0 Smoke、负向隔离矩阵、六项 executable spike | 登录租户切换、凭据非回显、跨租户 404、桌面控制面主路径 |
| M1 | US-02、US-03、US-12 的 M1 断言及对应 Smoke | Golden Path、同步恢复、互斥和 SSE 重连、OpenAPI Viewer/public read |
| M2 | US-04、US-07 及对应 Smoke | overlay 审批/回滚、provenance、GitOps 字段来源 |
| M3 | US-05、US-06、US-09 及对应 Smoke（包含 SMK-033） | AI 审批和失败恢复、repo base 替换 AI base、diff/share/todo、CLI 门禁 |
| M4 | US-08 及多 kind/search/group Smoke | 搜索 deep link、租户/分组图范围、覆盖率 |
| M5 | US-10、US-02 的体验镜像断言（SMK-030）及协作/合规 Smoke | 通知投递、Webhook 重试、订阅取消、运维恢复、体验镜像五分钟首文档 |

每个里程碑的全部有效工作项（包括新增修复项）必须先为 `passed`；`acceptance.yaml` 映射的所有 assertion、Smoke、负向用例、生成无漂移和必要性能/A11y/安全门禁必须有当前有效证据。然后生成 `artifacts/agent/milestones/<M#>/<timestamp>/report.json` 和用户报告，把 `milestoneGates.<M#>.status` 从 `pending` 改为 `needs_human_acceptance`，写入 `reportPath`、候选 `acceptedCommit` 和 `contractDigest`，暂停领取下一里程碑。

阶段报告必须包含可运行的桌面入口/启动命令、演示主路径、所有工作项及断言证据索引、失败/延期项和风险，且明确写出等待验收的阶段。用户明确 `accepted` 后记录 `acceptedBy`、`acceptedAt`，核对报告提交和契约仍有效，再将状态置为 `accepted` 并领取下一里程碑。未回复不能视为默认通过；不允许预填验收人。`changes_requested` 必须保留反馈和原报告，生成同阶段修复工作项并置回 `pending`，完成后再报告。人工验收不能替代缺失的自动断言。

人工验收回复应包含：`accepted` 或 `changes_requested`、验收人、时间、报告路径和具体断言 ID。`changes_requested` 必须生成新的修复工作项，保留原报告，不直接改写历史状态。

## 9. 进度文件和队列的原子更新

每个 checkpoint 按此顺序完成：

1. 写入代码、测试和必要契约变更。
2. 执行验证并保存预检 artifacts，运行生成无漂移和 `git diff --check`。
3. 提交代码和测试，工作项仍为 `verifying`，不得混入他人的工作区修改。
4. 在该源码提交上重跑声明门禁，正式报告的 `commit` 固定为此完整 SHA，记录源码摘要；禁止填写尚未产生的未来提交 SHA。
5. 仅提交证据索引、工作项结果和 `06_implementation-progress.md`。这个 evidence checkpoint 的 SHA 可以不同于报告的被测源码 SHA；不反复修改报告追赶自身提交哈希。
6. 再次检查生成无漂移及被测源码未变；通过后才领取下一工作项。崩溃在步骤 3 后则继续验证，不将 `verifying` 当成已完成。

提交后若验证失败，按尝试次数回到 `needs_retry` 或 `failed`，不得把已提交 SHA 标成稳定提交，直到修复产生新的通过证据。报告中的 `commit` 是已验证的代码 checkpoint 完整 SHA，后续记录状态的提交可引用它，不能填写尚不存在的自身提交 SHA。任何契约或工具链变更都必须在报告中列出兼容性影响，并重新运行所有受影响里程碑门禁；若影响已验收阶段，其 gate 必须回到 `pending` 并重新报告。

## 10. 长程停止条件

Agent 可以持续运行，直到满足下列任一条件：

- 某里程碑已达到 `needs_human_acceptance`，阶段报告已交给用户，等待明确放行；
- 所有 `M0-M5` 工作项为 `passed`、六个 `milestoneGates` 均为 `accepted`，且 `06_implementation-progress.md` 与 acceptance 门禁一致，此时才可宣告项目完成；
- 遇到 `product_decision`、外部权限或环境输入导致的 `blocked`，并已发送完整报告；
- 技术失败达到上限，已标记 `failed`、填写 `failureKind`、报告技术升级请求并保留可复现证据。

除上述条件外不得自行宣告项目完成、跳过未实现的 operation、删除失败 fixture 或将移动端视觉延期误报为功能完成。每次自动循环结束都必须留下“下一工作项”或明确的人工阻塞原因，保证下一次 Agent 能从持久化状态继续。
