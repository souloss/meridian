package dbschema

import (
	"context"
	"errors"

	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/plugin"
	"go.yaml.in/yaml/v3"
)

const (
	Kind          = "dbschema"
	PluginVersion = "1"
	SchemaVersion = "meridian-dbschema-1"
)

type Plugin struct{}

func NewPlugin() *Plugin { return &Plugin{} }

func (Plugin) Descriptor() kinds.Descriptor {
	return kinds.Descriptor{Kind: Kind, PluginVersion: PluginVersion, ContractVersion: "meridian-asset-kinds-v1", Capabilities: []string{"validate", "extract", "generic_diff", "overlay"}}
}

func (Plugin) Manifest() plugin.Manifest {
	return plugin.Manifest{ID: "meridian.kind.dbschema", Version: PluginVersion, ProtocolVersion: plugin.CurrentProtocolVersion, Capabilities: []plugin.Capability{{ID: "kind/" + Kind, Version: PluginVersion}}}
}

func (p Plugin) Open(context.Context) (plugin.Endpoint, error) { return kinds.NewLocalEndpoint(p), nil }

func (Plugin) Validate(ctx context.Context, input kinds.Document) (kinds.ValidatedDocument, kinds.ValidationReport, error) {
	if err := ctx.Err(); err != nil {
		return kinds.ValidatedDocument{}, kinds.ValidationReport{}, err
	}
	var document map[string]any
	if err := yaml.Unmarshal(input.Content, &document); err != nil {
		return kinds.ValidatedDocument{}, kinds.ValidationReport{Errors: []string{err.Error()}}, err
	}
	if document["schemaVersion"] != SchemaVersion {
		return kinds.ValidatedDocument{}, kinds.ValidationReport{Errors: []string{"invalid dbschema schemaVersion"}}, errors.New("invalid dbschema schemaVersion")
	}
	return kinds.ValidatedDocument{Document: input}, kinds.ValidationReport{Valid: true}, nil
}

func (Plugin) Normalize(ctx context.Context, input kinds.ValidatedDocument) (kinds.CanonicalDocument, kinds.ArtifactSet, error) {
	if err := ctx.Err(); err != nil {
		return kinds.CanonicalDocument{}, nil, err
	}
	return kinds.CanonicalDocument{Document: input.Document, Version: SchemaVersion}, nil, nil
}

func (Plugin) Extract(ctx context.Context, input kinds.CanonicalDocument, _ kinds.ProvenanceMap) ([]kinds.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var document map[string]any
	if err := yaml.Unmarshal(input.Document.Content, &document); err != nil {
		return nil, err
	}
	if document["schemaVersion"] != SchemaVersion {
		return nil, errors.New("invalid dbschema schemaVersion")
	}
	tables, _ := document["tables"].([]any)
	items := make([]kinds.Item, 0)
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
		items = append(items, kinds.Item{ItemType: "table", Key: name, Display: map[string]any{"name": name, "description": stringValue(table["description"]), "columnCount": len(columns)}, SearchText: name})
		for _, columnEntry := range columns {
			column, ok := columnEntry.(map[string]any)
			if !ok {
				continue
			}
			columnName, _ := column["name"].(string)
			if columnName == "" {
				continue
			}
			items = append(items, kinds.Item{ItemType: "column", Key: name + "." + columnName, Display: map[string]any{"table": name, "name": columnName, "dataType": stringValue(column["dataType"]), "nullable": boolValue(column["nullable"]), "description": stringValue(column["description"])}, SearchText: name + " " + columnName})
		}
	}
	return items, nil
}

// Diff 对两侧文档的条目键派生「移除即破坏」的差异：左有右无的表/列键视为移除。
func (Plugin) Diff(_ context.Context, left, right kinds.CanonicalDocument, _ kinds.RuleSet) (kinds.DiffResult, error) {
	leftKeys, leftErr := itemKeys(left.Document.Content)
	if leftErr != nil {
		return kinds.DiffResult{}, leftErr
	}
	rightKeys, rightErr := itemKeys(right.Document.Content)
	if rightErr != nil {
		return kinds.DiffResult{}, rightErr
	}
	return kinds.RemovedItemKeys(leftKeys, rightKeys, kinds.DiffCodeItemRemoved), nil
}

// itemKeys 提取 dbschema 文档的表与列条目键集合。
func itemKeys(content []byte) (map[string]bool, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	if document["schemaVersion"] != SchemaVersion {
		return nil, errors.New("invalid dbschema schemaVersion")
	}
	keys := make(map[string]bool)
	tables, _ := document["tables"].([]any)
	for _, entry := range tables {
		table, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := table["name"].(string)
		if name == "" {
			continue
		}
		keys[name] = true
		columns, _ := table["columns"].([]any)
		for _, columnEntry := range columns {
			column, ok := columnEntry.(map[string]any)
			if !ok {
				continue
			}
			columnName, _ := column["name"].(string)
			if columnName != "" {
				keys[name+"."+columnName] = true
			}
		}
	}
	return keys, nil
}

func (Plugin) CompileOverlay(context.Context, string, []byte) ([]kinds.OverlayAction, error) {
	return nil, errors.New("dbschema overlay compilation is not yet available through the built-in plugin")
}

func stringValue(value any) string { result, _ := value.(string); return result }
func boolValue(value any) bool     { result, _ := value.(bool); return result }

var _ kinds.Plugin = Plugin{}
var _ plugin.Factory = Plugin{}
