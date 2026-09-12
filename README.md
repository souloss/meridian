# Meridian

Meridian 是一个面向企业系统的架构资产控制面：把散落在 Git 仓库、接口契约、数据库和事件定义里的知识，整理成可追踪、可版本化、可审查的系统地图。

我们相信，架构图不应该只是一次性的汇报材料。它应该像代码一样可执行、像数据一样可查询、像变更记录一样可追溯：当接口、服务或数据结构发生变化时，团队能迅速知道影响了谁、为什么影响、下一步该做什么。

## 为什么是 Meridian

- **从代码出发**：连接 Git 仓库，发现服务与资产，减少手工维护目录的成本。
- **契约优先**：OpenAPI、事件、数据库模型和视图由机器可读契约驱动，生成结果可检查、可复现。
- **版本与证据**：每个资产保留来源、版本、层和变更历史，决策不再依赖口头记忆。
- **治理而非展示**：差异、风险、审批、任务和审计串成一条交付链路。
- **多租户与可私有化**：Go 单二进制配合 PostgreSQL，适合在组织内部部署和演进。

Meridian 当前首先打牢身份、租户隔离、凭据、仓库连接、任务、审计、通知和资产目录等 M0 基础；后续按契约推进多源采集、Overlay、AI 评审、关系图、字段血缘与影响分析。尚未实现的能力会明确标注在路线图和验收契约中，不用演示数据冒充生产能力。

## 快速开始

### Docker（推荐体验方式）

需要 Docker Compose。下面的命令会启动 PostgreSQL、执行迁移、创建本地管理员并运行 Meridian：

```bash
docker compose up --build
```

打开 <http://127.0.0.1:18080>，默认本地账号为 `padmin`，密码为 `correct horse battery staple`。这些值只用于本地演示，部署到共享或生产环境前请通过环境变量替换所有密钥和密码。

停止服务：

```bash
docker compose down
```

### 本地开发

项目锁定 Go 1.27.1、Node 24.20.0 和 pnpm 11.20.0，仓库提供 `.vfox.toml` 统一工具链。准备好 PostgreSQL 后，可以按下面的顺序检查代码：

```bash
make frontend-install
make contracts-validate
make backend-test
make frontend-typecheck
```

启动本地后端（会自动执行迁移）：

```bash
make backend-run
```

常用生成与检查命令：

```bash
make contracts-generate                 # 从契约生成 Go/TypeScript 代码
make contracts-generate-then-git-diff-exit-code
make contracts-check                    # 检查 canonical OpenAPI 是否漂移
make contract-tooling-test
make frontend-typecheck
```

默认开发数据库地址为 `postgres://meridian:meridian@127.0.0.1:54329/meridian?sslmode=disable`。如需改变连接信息，请设置 `MERIDIAN_DEV_DATABASE_URL` 或对应的 `MERIDIAN_*` 环境变量，不要把真实凭据写入仓库。

## 代码地图

```text
cmd/meridian/          CLI 入口
internal/handler/      HTTP 路由、认证和 DTO 适配
internal/service/      用例、权限、状态和领域规则
internal/repository/   PostgreSQL 持久化端口实现
internal/task/         River worker、重试和任务事件
internal/storage/      本地内容寻址 Blob 存储
internal/generated/    由契约和 sqlc 生成；禁止手工编辑
migrations/            Goose 迁移与 sqlc 查询源
contracts/             TypeSpec、OpenAPI、领域和验收契约
web/app/               Nuxt 4/Vue 3 前端
scripts/               生成、Smoke 和开发辅助脚本
```

后端依赖方向保持简单：`handler → service → repository`。任务、存储和外部工具通过窄接口接入；业务规则不直接依赖 HTTP 或数据库细节。新增功能时，优先把边界写进契约，再实现用例和适配器。

## 契约与生成代码

`contracts/manifest.yaml` 是契约入口。HTTP API 的唯一可编辑源在 `contracts/api/`，聚合后的 `contracts/openapi.yaml`、`internal/generated/api/` 和前端 API 客户端都由生成流程产出。数据库查询源在 `migrations/queries/`，修改后必须同步审阅 `internal/generated/repository/`。

请不要直接修改生成文件，也不要在 Markdown、handler 或前端重新复制一份 DTO、枚举或权限规则。契约变更应同时更新验收映射，并运行生成无漂移检查。

## 路线图

| 阶段 | 目标 | 状态 |
| --- | --- | --- |
| M0 | 契约与生成、身份/租户/RBAC、凭据、仓库骨架、任务、审计、Outbox、Blob | 已完成基础里程碑 |
| M1 | 仓库发现与同步、服务/资产主链路、OpenAPI 规范化与公开读取 | 契约就绪，业务实现待推进 |
| M2 | Layer、Overlay、来源证据、回滚与字段级 GitOps | 规划中 |
| M3 | AI 生成与评审、版本 Diff、分享、变更门禁和 CLI | 规划中 |
| M4 | 多 kind、依赖图、搜索和全局视图 | 规划中 |
| M5 | 订阅、通知渠道、开放集成、合规与运维加固 | 规划中 |

详细范围、状态机和验收条件以 `contracts/` 与 `docs/` 为准；如果实现与文档不一致，应先修正契约或记录阻塞，而不是静默扩大范围。

## 参与项目

无论你擅长 Go、Vue、数据库、契约设计、测试还是技术写作，都可以从一个小而完整的切片开始：

1. 阅读 [`contracts/manifest.yaml`](contracts/manifest.yaml)、相关领域契约和 [`docs/07_coding-agent-runbook.md`](docs/07_coding-agent-runbook.md)。
2. 从 [`contracts/work-items.yaml`](contracts/work-items.yaml) 选择一个未领取的工作项，确认依赖和验收断言。
3. 先补契约、测试或可复现的失败证据，再实现最小行为；不要提交未接入的占位页面或未经验证的 mock 成功路径。
4. 运行与改动范围匹配的检查，并在提交信息中说明变更职责和验证结果。

欢迎提交以下类型的贡献：

- 补充租户隔离、并发、错误路径和可访问性测试；
- 改进 OpenAPI/TypeSpec、数据库迁移和生成工具链；
- 实现新的资产适配器或视图，并保持适配器与核心模型的边界；
- 优化前端信息架构、键盘操作、移动布局和国际化；
- 改写文档，让复杂的架构决策更容易被新贡献者理解。

每个提交尽量只承担一个职责，生成代码与其源变更放在同一提交，避免把格式化、重命名和行为变更混在一起。遇到契约未定义、权限边界不清或外部环境不可用时，请在 Issue/变更说明中记录阻塞点。

## 维护原则

- 正确性、租户隔离和可审计性优先于短期演示效果。
- 复杂度应该被边界和契约吸收，而不是转移给每个调用方。
- 注释解释业务约束、数据边界和不可见的不变量，不重复朗读代码。
- 生成物必须可重建；构建产物、截图、编辑器配置、凭据和本地 Agent 数据不进入 Git。

如果这些目标与你对“可维护的企业架构工具”的期待一致，欢迎加入 Meridian，一起把架构知识变成团队可以持续依赖的工程资产。
