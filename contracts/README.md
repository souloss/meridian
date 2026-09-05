# Meridian 共享契约

前后端、CLI、worker、测试和运维从 [manifest.yaml](./manifest.yaml) 进入，不从 Markdown 复制 DTO、枚举或状态机。

开发工具链、框架职责与替换门禁见 [技术栈冻结决策](../docs/05_technology-stack-decision.md)；可执行版本范围仍以 `manifest.yaml.toolchain` 为准。

| 文件 | 唯一负责内容 |
| --- | --- |
| [openapi.yaml](./openapi.yaml) | HTTP operation、鉴权、请求响应、错误 |
| [domain.yaml](./domain.yaml) | 领域枚举、权限、状态机、限制、默认值、并发与恢复语义 |
| [storage.yaml](./storage.yaml) | PostgreSQL 表、键、索引、外键策略、事务边界 |
| [kinds.yaml](./kinds.yaml) | AssetKind 插件接口、内建 kind、内容 schema、breaking 规则 |
| [views.yaml](./views.yaml) | ViewDef、输入约束、挂载和表格列 |
| [events.yaml](./events.yaml) | 事件、webhook envelope、签名和投递 |
| [acceptance.yaml](./acceptance.yaml) | fixture、用户故事、Smoke 和 assertion 映射 |
| [cli.yaml](./cli.yaml) | `meridian` CLI 参数、selector、API 映射、输出与退出码 |
| [repository-config.schema.yaml](./repository-config.schema.yaml) | `.asset-platform.yaml` JSON Schema |

## 使用规则

1. 先改拥有该语义的 YAML，再更新其它投影；
2. OpenAPI 生成 Go server/client 与 TypeScript client，生成目录见 manifest；
3. 所有 operation、schema、字段和参数必须在 OpenAPI 写清 `description`；Go/TypeScript 生成器会传播这些语义，并为纯传输胶水补充稳定的 GoDoc/JSDoc，禁止手改生成文件；
4. OpenAPI 结构校验使用同目录 `.redocly.yaml`，摘要和标签文案是编辑元数据，不能替代 operationId、schema、鉴权和响应校验；
5. 领域枚举在 OpenAPI、数据库和测试中的投影必须与 `domain.yaml` 一致；
6. `$ref`、operationId、view/kind/event/acceptance 交叉引用必须由 CI 校验；
7. 冲突是构建错误，不通过覆盖顺序解决；
8. Markdown 只解释业务和实现，不成为第二份契约。

`status: frozen` 表示已允许编码，不表示永不变更。破坏性变更按 manifest 的兼容策略升级版本并保留旧 API 一个发布周期。
