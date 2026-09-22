package openapi

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/plugin"
	"go.yaml.in/yaml/v3"
)

const (
	Kind          = "openapi"
	PluginVersion = "1"
)

// Plugin is the built-in OpenAPI capability. It also implements plugin.Factory
// so it can be hosted by the same transport-neutral plugin host as future
// external providers.
type Plugin struct{}

func NewPlugin() *Plugin { return &Plugin{} }

func (Plugin) Descriptor() kinds.Descriptor {
	return kinds.Descriptor{
		Kind: Kind, PluginVersion: PluginVersion, ContractVersion: "meridian-asset-kinds-v1",
		Capabilities: []string{"validate", "normalize", "extract", "structured_diff", "overlay"},
	}
}

func (Plugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "meridian.kind.openapi", Version: PluginVersion, ProtocolVersion: plugin.CurrentProtocolVersion,
		Capabilities: []plugin.Capability{{ID: "kind/openapi", Version: PluginVersion}},
	}
}

func (p Plugin) Open(context.Context) (plugin.Endpoint, error) {
	return endpoint{plugin: p}, nil
}

func (Plugin) Validate(ctx context.Context, input kinds.Document) (kinds.ValidatedDocument, kinds.ValidationReport, error) {
	if err := ctx.Err(); err != nil {
		return kinds.ValidatedDocument{}, kinds.ValidationReport{}, err
	}
	var document map[string]any
	if err := yaml.Unmarshal(input.Content, &document); err != nil {
		return kinds.ValidatedDocument{}, kinds.ValidationReport{Valid: false, Errors: []string{err.Error()}}, err
	}
	if _, ok := document["paths"]; !ok {
		return kinds.ValidatedDocument{}, kinds.ValidationReport{Valid: false, Errors: []string{"openapi document has no paths"}}, errors.New("openapi document has no paths")
	}
	return kinds.ValidatedDocument{Document: input}, kinds.ValidationReport{Valid: true}, nil
}

func (Plugin) Normalize(ctx context.Context, input kinds.ValidatedDocument) (kinds.CanonicalDocument, kinds.ArtifactSet, error) {
	if err := ctx.Err(); err != nil {
		return kinds.CanonicalDocument{}, nil, err
	}
	return kinds.CanonicalDocument{Document: input.Document, Version: "openapi-3.1"}, nil, nil
}

func (Plugin) Extract(ctx context.Context, input kinds.CanonicalDocument, _ kinds.ProvenanceMap) ([]kinds.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	operations, err := parseOperations(input.Document.Content)
	if err != nil {
		return nil, err
	}
	items := make([]kinds.Item, 0, len(operations))
	for _, operation := range operations {
		display := map[string]any{
			"method": operation.Method, "path": operation.Path,
			"summary": operation.Summary, "tags": operation.Tags, "deprecated": operation.Deprecated,
		}
		items = append(items, kinds.Item{
			ItemType: "operation", Key: operation.Method + " " + operation.Path,
			Display: display, SearchText: operation.Method + " " + operation.Path + " " + operation.Summary,
		})
	}
	return items, nil
}

// Diff 对两侧文档的操作条目键派生「移除即破坏」的差异：左有右无的 operation 键视为
// 移除操作并标记 breaking。与宿主的 OpenAPI diff 语义保持一致（operation-removed）。
func (Plugin) Diff(_ context.Context, left, right kinds.CanonicalDocument, _ kinds.RuleSet) (kinds.DiffResult, error) {
	return kinds.RemovedItemKeys(operationKeys(left.Document.Content), operationKeys(right.Document.Content), kinds.DiffCodeOperationRemoved), nil
}

// operationKeys 从 OpenAPI 文档提取 'UPPER(method) normalizedPath' 键集合。
func operationKeys(content []byte) map[string]bool {
	operations, err := parseOperations(content)
	if err != nil {
		return map[string]bool{}
	}
	keys := make(map[string]bool, len(operations))
	for _, operation := range operations {
		keys[operation.Method+" "+operation.Path] = true
	}
	return keys
}

func (Plugin) CompileOverlay(context.Context, string, []byte) ([]kinds.OverlayAction, error) {
	return nil, errors.New("openapi overlay compilation is not yet available through the built-in plugin")
}

type operation struct {
	Method     string
	Path       string
	Summary    string
	Tags       []string
	Deprecated bool
}

func parseOperations(content []byte) ([]operation, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	paths, _ := document["paths"].(map[string]any)
	operations := make([]operation, 0)
	for path, pathItem := range paths {
		item, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for method, rawOperation := range item {
			upper := strings.ToUpper(method)
			if !validMethod(upper) {
				continue
			}
			op, _ := rawOperation.(map[string]any)
			operations = append(operations, operation{
				Method: upper, Path: normalizePath(path), Summary: stringValue(op["summary"]),
				Tags: stringSliceValue(op["tags"]), Deprecated: boolValue(op["deprecated"]),
			})
		}
	}
	slices.SortFunc(operations, func(left, right operation) int {
		if left.Path != right.Path {
			if left.Path < right.Path {
				return -1
			}
			return 1
		}
		if left.Method < right.Method {
			return -1
		}
		if left.Method > right.Method {
			return 1
		}
		return 0
	})
	return operations, nil
}

func validMethod(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func stringSliceValue(value any) []string {
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

type endpoint struct{ plugin Plugin }

type extractResponse struct {
	Items []kinds.Item `json:"items"`
}

func (endpoint endpoint) Invoke(ctx context.Context, call plugin.Call) (plugin.Result, error) {
	if call.Capability != "kind/openapi" {
		return plugin.Result{}, fmt.Errorf("unknown capability %s", call.Capability)
	}
	document := kinds.Document{Content: call.Payload, MediaType: call.ContentType}
	switch call.Method {
	case "validate":
		_, report, err := endpoint.plugin.Validate(ctx, document)
		payload, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, err
	case "extract":
		validated, _, err := endpoint.plugin.Validate(ctx, document)
		if err != nil {
			return plugin.Result{}, err
		}
		canonical, _, err := endpoint.plugin.Normalize(ctx, validated)
		if err != nil {
			return plugin.Result{}, err
		}
		var provenance kinds.ProvenanceMap
		if encoded := call.Metadata["provenance"]; encoded != "" {
			if err := json.Unmarshal([]byte(encoded), &provenance); err != nil {
				return plugin.Result{}, err
			}
		}
		items, err := endpoint.plugin.Extract(ctx, canonical, provenance)
		if err != nil {
			return plugin.Result{}, err
		}
		payload, marshalErr := json.Marshal(extractResponse{Items: items})
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, nil
	case "normalize":
		validated := kinds.ValidatedDocument{Document: document}
		canonical, artifacts, err := endpoint.plugin.Normalize(ctx, validated)
		if err != nil {
			return plugin.Result{}, err
		}
		payload, marshalErr := json.Marshal(struct {
			Document  kinds.Document    `json:"document"`
			Version   string            `json:"version"`
			Artifacts kinds.ArtifactSet `json:"artifacts"`
		}{canonical.Document, canonical.Version, artifacts})
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, nil
	case "diff":
		var request struct {
			Left  kinds.CanonicalDocument `json:"left"`
			Right kinds.CanonicalDocument `json:"right"`
			Rules kinds.RuleSet           `json:"rules"`
		}
		if err := json.Unmarshal(call.Payload, &request); err != nil {
			return plugin.Result{}, err
		}
		result, err := endpoint.plugin.Diff(ctx, request.Left, request.Right, request.Rules)
		if err != nil {
			return plugin.Result{}, err
		}
		payload, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, nil
	case "overlay":
		return plugin.Result{}, errors.New("openapi overlay compilation is not yet available through the built-in plugin")
	default:
		return plugin.Result{}, fmt.Errorf("unknown method %s", call.Method)
	}
}

func (endpoint) Close(context.Context) error { return nil }

var _ kinds.Plugin = Plugin{}
var _ plugin.Factory = Plugin{}
