# 资产类型（AssetKind）说明目录

> 本目录是各资产类型的**人工可读**说明与实现要点。
> **机器唯一口径**：`contracts/kinds.yaml`（插件接口、registry、contentSchema、itemSchema、breakingRules）+ `contracts/domain.yaml`（枚举、范围、`excludedFromV1`）。
> 禁止在本文维护第二份枚举/DTO/schema；本目录只做解释、放标准链接与注意事项。不一致时以契约为准。

## 插件模型

资产类型通过 kind 插件接入。v1 冻结的接口与约束在 `kinds.yaml.pluginInterface`：

| 方法 | 签名 | 职责 |
|---|---|---|
| `validator` | `Validate(ctx, Document) (ValidatedDocument, ValidationReport, error)` | 内容合法性校验（schema + lint + 质量分） |
| `normalizer` | `Normalize(ctx, ValidatedDocument) (CanonicalDocument, ArtifactSet, error)` | 格式归一（swagger2→oas3.1 等） |
| `extractor` | `Extract(ctx, CanonicalDocument, ProvenanceMap) ([]Item, error)` | 产出扁平 AssetItem（供检索/diff/全局视图） |
| `differ` | `Diff(ctx, CanonicalDocument, CanonicalDocument, RuleSet) (DiffResult, error)` | 结构化 diff（缺省退化 generic_diff） |
| `overlayCompiler` | `CompileOverlay(ctx, dialect, []byte) ([]overlay.Action, error)` | 把 overlay 方言编译为内部 action |

确定性约束：输入只允许 `content-bytes` / `plugin-version` / `rule-set-version`，禁止 `clock` / `random` / `mutable-environment`。

实现模式：

- **v1**：编译期注册（代码内注册，`goPackage: internal/kinds`），新增 kind = 注册记录 + 实现接口，不改核心表结构。
- **进程外 gRPC 插件**：P2/M6+，当前在 `domain.yaml.excludedFromV1`（`grpc-asset-kind-plugins`）。

## 内置 kind 一览

| kind | 里程碑 | 状态 | 目录 |
|---|---|---|---|
| openapi | M1 | v1 标杆实现 | [openapi/](./openapi/) |
| asyncapi | M5 | v1 内 | [asyncapi/](./asyncapi/) |
| dbschema | M4 | v1 内 | [dbschema/](./dbschema/) |
| dependency | M4 | v1 内 | [dependency/](./dependency/) |
| markdown_doc | — | **P2 / excludedFromV1** | [markdown_doc/](./markdown_doc/) |

## 能力词表（capabilities）

| 能力 | 含义 | 缺省 |
|---|---|---|
| `validate` | 校验内容合法性 | 必实现 |
| `normalize` | 归一为 canonical 版本 | 可选 |
| `extract` | 产出 AssetItem | 必实现 |
| `structured_diff` | kind 专属结构化 diff（如 openapi 的 what-changed） | 缺省退化 `generic_diff` |
| `generic_diff` | 通用结构化 diff（AssetItem 集合 + JSON 树） | 兜底 |
| `breaking_rules` | 提供 breaking 判定规则 | 缺省则 diff 无 breaking 分级 |
| `overlay` | 支持 overlay 叠加 | 缺省只支持 platform-v1 通用 overlay（或整篇替换） |
