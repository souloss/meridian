# markdown_doc（P2 · excludedFromV1）

> 当前不在 v1 交付范围：`domain.yaml.excludedFromV1` 包含 `markdown-doc-asset-kind`，`kinds.yaml` 无其 registry/contentSchema/itemSchema 定义。
> 本目录仅记录需求口径与「若纳入 v1」的最简草案；**草案不生效，落地需契约变更（先改 kinds.yaml + domain.yaml 再走校验）**。

## 需求口径（01_requirement-specification.md 2.3）

| 项 | 值 |
|---|---|
| content 格式 | Markdown 文档集 |
| base 层典型来源 | 仓库 glob |
| item 类型 | doc |
| overlay 方言 | **不支持 overlay（整篇替换）** |
| 优先级 | P2 |

## 若纳入 v1 的最简口径（草案）

- registry 条目：`id: markdown_doc`、`itemTypes: [doc]`、`capabilities: [validate, extract, generic_diff]`（无 normalize、无 overlay）。
- itemSchema `markdown_doc.doc`：`required: [title, path]`（+ 从正文提取的检索文本）。
- **merge 语义退化为「整篇替换」**：无 overlay 叠加，merge 管道对 markdown_doc 恒等退化（base 层即最终内容），provenance 粒度到整篇文档。

## 注意事项

- **无结构化 overlay**：markdown 不参与 JSONPath/merge 叠加，provenance 只能标到文档级，字段级来源不可用。
- **检索**：全文检索文本从 markdown 正文提取（`tsvector`），无扁平结构化 item 可检索字段。
- 若 v1 需要文档类资产，先确认「整篇替换、无 overlay、无字段级 provenance」是否满足，再走契约变更。
