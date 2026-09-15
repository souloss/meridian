# openapi（P0 标杆实现 · M1）

> 外部标准，本目录不重写规范，只放链接、契约口径与注意事项。
> 机器口径：`contracts/kinds.yaml`（registry `id: openapi`、itemSchema `openapi.operation`、breakingRules `openapi-v1`）。

## 标准来源

| 标准 | 链接 | 说明 |
|---|---|---|
| OpenAPI 3.1.0 | https://spec.openapis.org/oas/v3.1.0.html | canonical 存储版本 |
| OpenAPI 3.0.3 | https://spec.openapis.org/oas/v3.0.3.html | 接受输入，归一为 3.1 |
| Swagger / OpenAPI 2.0 | https://swagger.io/specification/v2/ | 接受输入，归一为 3.1 |
| OpenAPI Overlay 1.0.0 | https://spec.openapis.org/overlay/v1.0.0.html | 一等 overlay 方言 |
| JSON Schema 2020-12 | https://json-schema.org/draft/2020-12 | 动态校验依赖 |

## 契约口径（kinds.yaml）

- `acceptedMediaTypes`: yaml / json
- `acceptedVersions`: swagger-2.0、openapi-3.0、openapi-3.1 → `canonicalVersion: openapi-3.1`
- `itemTypes`: `operation`；`itemKey: ${upper(method)} ${normalizedPath}`
- `overlayDialects`: `oas-overlay-1.0` + `platform-v1`（双方言，编译到同一内部 action）
- `defaultViews`: swagger-ui / redoc / source / layers / operations / items-table / diff
- `capabilities`: validate、normalize、extract、structured_diff、breaking_rules、overlay

## 注意事项

- **归一存储**：Swagger 2.0 与 OAS 3.0 输入归一为 3.1 后存储；`$ref` bundle 产出自包含版本供渲染，原始文件保留。
- **解析/结构化 diff**：`pb33f/libopenapi` 负责解析、引用解析、AST/索引与 `what-changed` 变化事实；kind 插件负责把变化事实映射为产品等级与 todo。
- **item = operation**：extractor 从合并结果产出扁平 operation 条目，供检索/统计/diff。
- **breaking 规则（openapi-v1）**：operation-removed、required-request-parameter-added、required-request-field-added、request-constraint-tightened、response-field-removed、scalar-type-narrowed、response-status-removed、authentication-tightened、media-type-removed。
- **overlay 双方言**：官方 OAS Overlay 1.0 与平台通用 Overlay 都可作为 overlay 层；通用方言结构见 `kinds.yaml.platformOverlaySchema`。
