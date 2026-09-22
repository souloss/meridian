package dependency

import (
	"context"
	"errors"

	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/plugin"
	"go.yaml.in/yaml/v3"
)

const (
	Kind          = "dependency"
	PluginVersion = "1"
	SchemaVersion = "meridian-dependency-1"
)

type Plugin struct{}

func NewPlugin() *Plugin { return &Plugin{} }

func (Plugin) Descriptor() kinds.Descriptor {
	return kinds.Descriptor{Kind: Kind, PluginVersion: PluginVersion, ContractVersion: "meridian-asset-kinds-v1", Capabilities: []string{"validate", "extract", "generic_diff", "overlay"}}
}

func (Plugin) Manifest() plugin.Manifest {
	return plugin.Manifest{ID: "meridian.kind.dependency", Version: PluginVersion, ProtocolVersion: plugin.CurrentProtocolVersion, Capabilities: []plugin.Capability{{ID: "kind/" + Kind, Version: PluginVersion}}}
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
		return kinds.ValidatedDocument{}, kinds.ValidationReport{Errors: []string{"invalid dependency schemaVersion"}}, errors.New("invalid dependency schemaVersion")
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
		return nil, errors.New("invalid dependency schemaVersion")
	}
	edges, _ := document["edges"].([]any)
	items := make([]kinds.Item, 0)
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
		items = append(items, kinds.Item{ItemType: "edge", Key: key, Display: map[string]any{"from": from, "to": to, "protocol": protocol, "name": name, "external": stringValue(edge["external"]), "middleware": stringValue(edge["middleware"])}, SearchText: from + " " + to + " " + protocol})
	}
	return items, nil
}

// Diff 对两侧文档的条目键派生「移除即破坏」的差异：左有右无的 edge 键视为移除。
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

// itemKeys 提取 dependency 文档的服务边条目键集合。
func itemKeys(content []byte) (map[string]bool, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	if document["schemaVersion"] != SchemaVersion {
		return nil, errors.New("invalid dependency schemaVersion")
	}
	keys := make(map[string]bool)
	edges, _ := document["edges"].([]any)
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
		keys[from+"->"+to+":"+protocol+":"+name] = true
	}
	return keys, nil
}

func (Plugin) CompileOverlay(context.Context, string, []byte) ([]kinds.OverlayAction, error) {
	return nil, errors.New("dependency overlay compilation is not yet available through the built-in plugin")
}

func stringValue(value any) string { result, _ := value.(string); return result }

var _ kinds.Plugin = Plugin{}
var _ plugin.Factory = Plugin{}
