package service

import (
	"go.yaml.in/yaml/v3"
)

// genericKindItem 是从 dbschema 或 dependency 文档提取的一个已索引条目。
// 其 Key 遵循 kinds.yaml 针对该 kind 的 itemKey 模板。
type genericKindItem struct {
	ItemType   string
	Key        string
	Display    map[string]any
	SearchText string
}

// parseDBSchemaItems 从解码后的 meridian-dbschema-1 文档提取表与列条目。
// 条目键遵循 kinds.yaml：
//
//	${tableName}${columnName == null ? "" : "." + columnName}
func parseDBSchemaItems(document map[string]any) ([]genericKindItem, error) {
	if document["schemaVersion"] != dbschemaSchemaVersion {
		return nil, ErrValidation
	}
	tables, _ := document["tables"].([]any)
	items := make([]genericKindItem, 0)
	for _, entry := range tables {
		table, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := table["name"].(string)
		if name == "" {
			continue
		}
		columns, _ := table["columns"].([]any)
		items = append(items, genericKindItem{
			ItemType: itemTypeTable, Key: name,
			Display:    map[string]any{"name": name, "description": stringValue(table["description"]), "columnCount": len(columns)},
			SearchText: name,
		})
		for _, columnEntry := range columns {
			column, ok := columnEntry.(map[string]any)
			if !ok {
				continue
			}
			columnName, _ := column["name"].(string)
			if columnName == "" {
				continue
			}
			items = append(items, genericKindItem{
				ItemType: itemTypeColumn, Key: name + "." + columnName,
				Display: map[string]any{
					"table": name, "name": columnName, "dataType": stringValue(column["dataType"]),
					"nullable": boolValue(column["nullable"]), "description": stringValue(column["description"]),
				},
				SearchText: name + " " + columnName,
			})
		}
	}
	return items, nil
}

// parseDependencyItems 从解码后的 meridian-dependency-1 文档提取边条目。
// 条目键遵循 kinds.yaml：
//
//	${from}->${to}:${protocol}:${name}
//
// 其中 ${to} 是解析出的 toServiceSlug 或外部目标。
func parseDependencyItems(document map[string]any) ([]genericKindItem, error) {
	if document["schemaVersion"] != dependencySchemaVersion {
		return nil, ErrValidation
	}
	edges, _ := document["edges"].([]any)
	items := make([]genericKindItem, 0)
	for _, entry := range edges {
		edge, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		from, _ := edge["from"].(string)
		to, _ := edge["toServiceSlug"].(string)
		if to == "" {
			to, _ = edge["external"].(string)
		}
		protocol, _ := edge["protocol"].(string)
		name, _ := edge["name"].(string)
		if name == "" {
			name = "default"
		}
		if from == "" || to == "" || protocol == "" {
			continue
		}
		key := from + "->" + to + ":" + protocol + ":" + name
		items = append(items, genericKindItem{
			ItemType: itemTypeEdge, Key: key,
			Display: map[string]any{
				"from": from, "to": to, "protocol": protocol, "name": name,
				"external": stringValue(edge["external"]), "middleware": stringValue(edge["middleware"]),
			},
			SearchText: from + " " + to + " " + protocol,
		})
	}
	return items, nil
}

// validateGenericKindContent 在接受修订前，按规范模式版本校验推送的 dbschema 或
// dependency 文档。
func validateGenericKindContent(kind, content string) error {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return ErrValidation
	}
	switch kind {
	case kindDbschema:
		if document["schemaVersion"] != dbschemaSchemaVersion {
			return ErrValidation
		}
	case kindDependency:
		if document["schemaVersion"] != dependencySchemaVersion {
			return ErrValidation
		}
	}
	return nil
}

// indexGenericKindItems 按类别返回解码资产文档的通用条目，类别无通用条目模式时返回 nil。
func indexGenericKindItems(kind string, document map[string]any) ([]genericKindItem, error) {
	switch kind {
	case kindDbschema:
		return parseDBSchemaItems(document)
	case kindDependency:
		return parseDependencyItems(document)
	default:
		return nil, nil
	}
}
