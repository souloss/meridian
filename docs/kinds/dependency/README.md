# dependency（P1 · M4）

> 平台自定义 JSON，权威 schema 见 `contracts/kinds.yaml.contentSchemas.dependency`。
> 本文为阅读示例与注意事项；**机器唯一口径以 kinds.yaml 为准**。

## 定位与来源

服务/资源之间的依赖边（服务→服务、服务→中间件）。base 层典型来源：

- `command` / `push` / `manual`

## 模型（阅读示例，权威 = kinds.yaml.contentSchemas.dependency）

```yaml
schemaVersion: meridian-dependency-1
edges:
  - from: order-service         # source-service-slug
    toServiceSlug: pay-service  # 与 external 二选一
    protocol: http              # http|grpc|event|database|cache|queue|other
    name: default
  - from: order-service
    external: redis://cache     # 外部依赖，与 toServiceSlug 二选一
    protocol: cache
```

## 契约口径（kinds.yaml）

- `canonicalVersion: meridian-dependency-1`
- `itemTypes`: `edge`；`itemKey: ${from}->${to}:${protocol}:${name}`
- `overlayDialects`: `platform-v1`
- `defaultViews`: source / layers / items-table / diff / **dep-graph**
- `capabilities`: validate、extract、generic_diff、overlay（无 normalize、无 breaking_rules）
- `itemSchema`: `dependency.edge`（fromServiceId/toServiceId/external/protocol/middleware）

## 注意事项

- **边两端**：`toServiceSlug`（平台内服务）与 `external`（外部资源）二选一，`oneOf` 约束。
- **协议枚举**：http/grpc/event/database/cache/queue/other。
- **全局依赖视图依赖它**：`dep-graph` 视图（P1）以 dependency 边为输入，作用域为 SystemGroup/租户。
- **无 breaking 分级**：dependency 缺省走 generic_diff，无 breaking_rules。
