// Package kinds defines the host-side contract for asset kind capabilities.
// The value types are deliberately transport-friendly so a remote endpoint can
// be adapted without changing the application-facing contract.
package kinds

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/meridian-labs/meridian/internal/plugin"
)

var (
	ErrInvalidKindPlugin = errors.New("invalid kind plugin")
	ErrDuplicateKind     = errors.New("duplicate kind plugin")
	ErrKindUnavailable   = errors.New("kind plugin unavailable")
)

type Descriptor struct {
	Kind            string   `json:"kind"`
	PluginVersion   string   `json:"pluginVersion"`
	ContractVersion string   `json:"contractVersion"`
	Capabilities    []string `json:"capabilities"`
}

type Document struct {
	Content   []byte `json:"content"`
	MediaType string `json:"mediaType"`
}

type ValidatedDocument struct {
	Document Document `json:"document"`
}

type ValidationReport struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type CanonicalDocument struct {
	Document Document `json:"document"`
	Version  string   `json:"version"`
}

type Artifact struct {
	Name      string `json:"name"`
	Content   []byte `json:"content"`
	MediaType string `json:"mediaType"`
}

type ArtifactSet []Artifact

type ProvenanceMap map[string]string

type Item struct {
	ItemType   string         `json:"itemType"`
	Key        string         `json:"key"`
	Display    map[string]any `json:"display"`
	SearchText string         `json:"searchText"`
	Provenance map[string]any `json:"provenance,omitempty"`
}

type RuleSet struct {
	Version string         `json:"version"`
	Rules   map[string]any `json:"rules,omitempty"`
}

type DiffResult struct {
	Breaking bool             `json:"breaking"`
	Changes  []map[string]any `json:"changes,omitempty"`
}

// 差异变更码，供 kind 的 Diff 实现与宿主消费者共用，避免同值常量多份漂移。
const (
	// DiffCodeOperationRemoved 是 openapi-v1 的 operation-removed 破坏规则码。
	DiffCodeOperationRemoved = "operation-removed"
	// DiffCodeItemRemoved 是通用 kind（dbschema / dependency 等）的条目移除码。
	DiffCodeItemRemoved = "item-removed"
)

// 差异变更条目在 Changes []map[string]any 中使用的一致键名，供消费者映射为强类型变更。
// 这是 wire contract 的一部分，外部插件与宿主共享同一份字段语义。
const (
	// DiffChangeLevel 是变更级别（breaking / non_breaking / …）。
	DiffChangeLevel = "level"
	// DiffChangeCode 是变更码（如 operation-removed、item-removed）。
	DiffChangeCode = "code"
	// DiffChangePath 定位被变更的条目。
	DiffChangePath = "path"
	// DiffChangeSummary 是人类可读的变更描述。
	DiffChangeSummary = "summary"
	// DiffChangeBefore 与 DiffChangeAfter 携带两侧原始值（缺失时为 nil）。
	DiffChangeBefore = "before"
	DiffChangeAfter  = "after"
)

// RemovedItemKeys 派生「移除即破坏」的通用差异：左有右无的条目键视为被移除并标记为
// breaking。code 是变更码（openapi 用 operation-removed，其余 kind 用 item-removed）。
// 它是内置 kind 的 Diff 实现共用的确定性逻辑，使 external runtime 与内置实现返回同构结果。
func RemovedItemKeys(left, right map[string]bool, code string) DiffResult {
	removed := make([]string, 0, len(left))
	for key := range left {
		if !right[key] {
			removed = append(removed, key)
		}
	}
	slices.Sort(removed)
	changes := make([]map[string]any, 0, len(removed))
	for _, key := range removed {
		changes = append(changes, map[string]any{
			DiffChangeLevel:   "breaking",
			DiffChangeCode:    code,
			DiffChangePath:    key,
			DiffChangeSummary: key,
			DiffChangeBefore:  map[string]any{"key": key},
			DiffChangeAfter:   nil,
		})
	}
	return DiffResult{Breaking: len(changes) > 0, Changes: changes}
}

type OverlayAction struct {
	Operation string `json:"operation"`
	Target    string `json:"target,omitempty"`
	Value     any    `json:"value,omitempty"`
}

// Plugin is the typed domain contract a kind implementation is written against.
// Built-in kinds implement it and are bridged into the transport-neutral host
// by NewLocalEndpoint; the interface does not itself cross any process boundary.
type Plugin interface {
	Descriptor() Descriptor
	Validate(context.Context, Document) (ValidatedDocument, ValidationReport, error)
	Normalize(context.Context, ValidatedDocument) (CanonicalDocument, ArtifactSet, error)
	Extract(context.Context, CanonicalDocument, ProvenanceMap) ([]Item, error)
	Diff(context.Context, CanonicalDocument, CanonicalDocument, RuleSet) (DiffResult, error)
	CompileOverlay(context.Context, string, []byte) ([]OverlayAction, error)
}

func capabilityID(descriptor Descriptor) string { return "kind/" + descriptor.Kind }

// NewLocalEndpoint adapts a typed Kind plugin to the transport-neutral
// endpoint. It is the in-process endpoint implementation used by built-in
// adapters; external runtimes provide their own plugin.Endpoint instead.
func NewLocalEndpoint(kindPlugin Plugin) plugin.Endpoint {
	return localEndpoint{plugin: kindPlugin, capability: capabilityID(kindPlugin.Descriptor())}
}

// Registry indexes installed kind capabilities by kind. Every entry is an
// endpoint owned by the plugin host; there is no path that stores or returns a
// typed plugin outside the host lifecycle.
type Registry struct {
	mu          sync.RWMutex
	endpoints   map[string]plugin.Endpoint
	descriptors map[string]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{endpoints: make(map[string]plugin.Endpoint), descriptors: make(map[string]Descriptor)}
}

// RegisterEndpoint binds a descriptor to any runtime endpoint, including a
// future RPC/process proxy. The registry does not inspect or own transport
// details beyond the endpoint lifecycle owned by the plugin host.
func (registry *Registry) RegisterEndpoint(descriptor Descriptor, endpoint plugin.Endpoint) error {
	if endpoint == nil || descriptor.Kind == "" || descriptor.PluginVersion == "" || descriptor.ContractVersion == "" {
		return ErrInvalidKindPlugin
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.endpoints[descriptor.Kind]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateKind, descriptor.Kind)
	}
	registry.endpoints[descriptor.Kind] = endpoint
	descriptor.Capabilities = slices.Clone(descriptor.Capabilities)
	registry.descriptors[descriptor.Kind] = descriptor
	return nil
}

func (registry *Registry) LookupEndpoint(kind string) (Descriptor, plugin.Endpoint, error) {
	registry.mu.RLock()
	endpoint, ok := registry.endpoints[kind]
	descriptor := registry.descriptors[kind]
	registry.mu.RUnlock()
	if !ok {
		return Descriptor{}, nil, fmt.Errorf("%w: %s", ErrKindUnavailable, kind)
	}
	return descriptor, endpoint, nil
}

func (registry *Registry) Descriptors() []Descriptor {
	registry.mu.RLock()
	descriptors := make([]Descriptor, 0, len(registry.descriptors))
	for _, descriptor := range registry.descriptors {
		descriptor.Capabilities = slices.Clone(descriptor.Capabilities)
		descriptors = append(descriptors, descriptor)
	}
	registry.mu.RUnlock()
	slices.SortFunc(descriptors, func(left, right Descriptor) int {
		if left.Kind < right.Kind {
			return -1
		}
		if left.Kind > right.Kind {
			return 1
		}
		return 0
	})
	return descriptors
}

type localEndpoint struct {
	plugin     Plugin
	capability string
}

func (endpoint localEndpoint) Invoke(ctx context.Context, call plugin.Call) (plugin.Result, error) {
	if call.Capability != endpoint.capability {
		return plugin.Result{}, fmt.Errorf("unknown kind capability %s", call.Capability)
	}
	switch call.Method {
	case "validate":
		validated, report, err := endpoint.plugin.Validate(ctx, Document{Content: call.Payload, MediaType: call.ContentType})
		_ = validated
		payload, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, err
	case "normalize":
		validated := ValidatedDocument{Document: Document{Content: call.Payload, MediaType: call.ContentType}}
		canonical, artifacts, err := endpoint.plugin.Normalize(ctx, validated)
		if err != nil {
			return plugin.Result{}, err
		}
		payload, marshalErr := json.Marshal(struct {
			Document  Document    `json:"document"`
			Version   string      `json:"version"`
			Artifacts ArtifactSet `json:"artifacts"`
		}{canonical.Document, canonical.Version, artifacts})
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, nil
	case "extract":
		canonical := CanonicalDocument{Document: Document{Content: call.Payload, MediaType: call.ContentType}, Version: call.Metadata["canonical-version"]}
		var provenance ProvenanceMap
		if encoded := call.Metadata["provenance"]; encoded != "" {
			if err := json.Unmarshal([]byte(encoded), &provenance); err != nil {
				return plugin.Result{}, err
			}
		}
		items, err := endpoint.plugin.Extract(ctx, canonical, provenance)
		if err != nil {
			return plugin.Result{}, err
		}
		payload, marshalErr := json.Marshal(struct {
			Items []Item `json:"items"`
		}{items})
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, nil
	case "diff":
		var request struct {
			Left  CanonicalDocument `json:"left"`
			Right CanonicalDocument `json:"right"`
			Rules RuleSet           `json:"rules"`
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
		actions, err := endpoint.plugin.CompileOverlay(ctx, call.Metadata["dialect"], call.Payload)
		if err != nil {
			return plugin.Result{}, err
		}
		payload, marshalErr := json.Marshal(actions)
		if marshalErr != nil {
			return plugin.Result{}, marshalErr
		}
		return plugin.Result{ContentType: "application/json", Payload: payload}, nil
	default:
		return plugin.Result{}, fmt.Errorf("unknown kind method %s", call.Method)
	}
}

func (localEndpoint) Close(context.Context) error { return nil }
