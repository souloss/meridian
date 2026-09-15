# dbschema（P1 · M4）

> 平台自定义 JSON，权威 schema 见 `contracts/kinds.yaml.contentSchemas.dbschema`。
> 本文为阅读示例与注意事项；**机器唯一口径以 kinds.yaml 为准**，禁止在本文件维护第二份 schema。

## 定位与来源

数据库 schema 资产，把表/列/索引/外键纳管。base 层典型来源：

- `command`（如 `atlas inspect` 产出 JSON）
- `push` / `manual`（手工/CI 推送）

## 模型（阅读示例，权威 = kinds.yaml.contentSchemas.dbschema）

```yaml
schemaVersion: meridian-dbschema-1
database:
  name: orders
  engine: postgresql          # postgresql | mysql | mariadb | sqlite | other
tables:
  - name: orders
    columns:
      - name: id
        dataType: bigint
        nullable: false
      - name: status
        dataType: varchar(32)
        nullable: false
        description: 订单状态
    indexes:
      - name: orders_pk
        columns: [id]
        unique: true
    foreignKeys:
      - name: fk_orders_user
        columns: [user_id]
        referenceTable: users
        referenceColumns: [id]
```

## 契约口径（kinds.yaml）

- `canonicalVersion: meridian-dbschema-1`
- `itemTypes`: `table` / `column`；`itemKey: ${tableName}${columnName == null ? "" : "." + columnName}`
- `overlayDialects`: `platform-v1`
- `defaultViews`: source / layers / items-table / diff / **erd**
- `capabilities`: validate、extract、generic_diff、breaking_rules、overlay（无 normalize）
- `itemSchema`: `dbschema.table`（name/description/columnCount）、`dbschema.column`（table/name/dataType/nullable/description）

## breaking 规则（dbschema-v1）

- table-removed
- column-removed
- column-type-narrowed
- nullable-changed-true-to-false-without-default

> `column-type-narrowed` 已声明但 v1 落地取保守口径：`dataType` 为自由字符串（最简落地），无引擎类型格子时无法可靠判定宽度变窄。v1 differ 对它做保守处理（不产生假 breaking），typed 模型落地后再启用精确判定；规则本身保留在契约，不改动。

## 注意事项

- **ER 图（v1 内，M4）**：`erd` 视图已纳入 v1（M4），作为 dbschema 的 defaultView 之一，从 table/column/foreignKeys 模型渲染表-关系图。首版最简落地：不做引擎类型着色/亲和度，只渲染模型里已有的表、列、主外键关系。
- **dataType 为自由字符串（最简落地取舍）**：v1 不做引擎类型格子/亲和度建模；`column-type-narrowed` breaking 按保守口径处理（见上）。engine 枚举含 `other` 兜底，跨引擎 ER 图/类型着色留待后续。
