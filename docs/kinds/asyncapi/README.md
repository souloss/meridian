# asyncapi（P1 · M5）

> 外部标准，本目录只放链接、契约口径与注意事项。
> 机器口径：`contracts/kinds.yaml`（registry `id: asyncapi`、itemSchema `asyncapi.channel` / `asyncapi.message`）。

## 标准来源

| 标准 | 链接 |
|---|---|
| AsyncAPI 3.0.0 | https://www.asyncapi.com/docs/reference/specification/v3.0.0 |
| AsyncAPI 2.6.0 | https://www.asyncapi.com/docs/reference/specification/v2.6.0 |

## 契约口径（kinds.yaml）

- `acceptedVersions`: asyncapi-2、asyncapi-3 → `canonicalVersion: asyncapi-3`
- `itemTypes`: `channel` / `message`；`itemKey: ${itemType}:${qualifiedName}`
- `overlayDialects`: `platform-v1`（无专属方言）
- `defaultViews`: source / layers / items-table / diff
- `capabilities`: validate、normalize、extract、generic_diff、overlay

## 注意事项

- **归一存储**：asyncapi 2 归一为 asyncapi 3。
- **generic diff**：无 openapi 那样的专属结构化 diff，走通用 diff（AssetItem 集合 + JSON 树）。
- **item 双类型**：channel 与 message 分开展平，检索/统计按 itemType 区分。
- **事件契约**：asyncapi 承载异步事件/消息契约（EDA），与 openapi 的同步 HTTP 契约并列。
