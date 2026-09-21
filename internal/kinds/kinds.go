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

type OverlayAction struct {
	Operation string `json:"operation"`
	Target    string `json:"target,omitempty"`
	Value     any    `json:"value,omitempty"`
}

// Plugin is the typed host contract for a kind capability. Its methods are
// pure domain operations; lifecycle and transport are owned by plugin.Host.
type Plugin interface {
	Descriptor() Descriptor
	Validate(context.Context, Document) (ValidatedDocument, ValidationReport, error)
	Normalize(context.Context, ValidatedDocument) (CanonicalDocument, ArtifactSet, error)
	Extract(context.Context, CanonicalDocument, ProvenanceMap) ([]Item, error)
	Diff(context.Context, CanonicalDocument, CanonicalDocument, RuleSet) (DiffResult, error)
	CompileOverlay(context.Context, string, []byte) ([]OverlayAction, error)
}

// EndpointPlugin is a transport-neutral endpoint binding. A process-local
// implementation and an RPC proxy both satisfy the same registry entry.
type EndpointPlugin struct {
	descriptor Descriptor
	endpoint   plugin.Endpoint
}

func NewEndpointPlugin(descriptor Descriptor, endpoint plugin.Endpoint) (*EndpointPlugin, error) {
	if endpoint == nil || descriptor.Kind == "" || descriptor.PluginVersion == "" || descriptor.ContractVersion == "" {
		return nil, ErrInvalidKindPlugin
	}
	return &EndpointPlugin{descriptor: descriptor, endpoint: endpoint}, nil
}

func (binding *EndpointPlugin) Descriptor() Descriptor { return binding.descriptor }

func (binding *EndpointPlugin) Validate(ctx context.Context, input Document) (ValidatedDocument, ValidationReport, error) {
	result, err := binding.endpoint.Invoke(ctx, plugin.Call{Capability: capabilityID(binding.descriptor), Method: "validate", ContentType: input.MediaType, Payload: input.Content})
	if err != nil {
		return ValidatedDocument{}, ValidationReport{}, err
	}
	var report ValidationReport
	if err := json.Unmarshal(result.Payload, &report); err != nil {
		return ValidatedDocument{}, ValidationReport{}, err
	}
	if !report.Valid {
		return ValidatedDocument{}, report, errors.New("kind validation failed")
	}
	return ValidatedDocument{Document: input}, report, nil
}

func (binding *EndpointPlugin) Normalize(ctx context.Context, input ValidatedDocument) (CanonicalDocument, ArtifactSet, error) {
	result, err := binding.endpoint.Invoke(ctx, plugin.Call{Capability: capabilityID(binding.descriptor), Method: "normalize", ContentType: input.Document.MediaType, Payload: input.Document.Content})
	if err != nil {
		return CanonicalDocument{}, nil, err
	}
	var response struct {
		Document  Document    `json:"document"`
		Version   string      `json:"version"`
		Artifacts ArtifactSet `json:"artifacts"`
	}
	if err := json.Unmarshal(result.Payload, &response); err != nil {
		return CanonicalDocument{}, nil, err
	}
	return CanonicalDocument{Document: response.Document, Version: response.Version}, response.Artifacts, nil
}

func (binding *EndpointPlugin) Extract(ctx context.Context, input CanonicalDocument, provenance ProvenanceMap) ([]Item, error) {
	metadata := map[string]string{"canonical-version": input.Version}
	if provenance != nil {
		encoded, err := json.Marshal(provenance)
		if err != nil {
			return nil, err
		}
		metadata["provenance"] = string(encoded)
	}
	result, err := binding.endpoint.Invoke(ctx, plugin.Call{Capability: capabilityID(binding.descriptor), Method: "extract", ContentType: input.Document.MediaType, Payload: input.Document.Content, Metadata: metadata})
	if err != nil {
		return nil, err
	}
	var response struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(result.Payload, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (binding *EndpointPlugin) Diff(ctx context.Context, left CanonicalDocument, right CanonicalDocument, rules RuleSet) (DiffResult, error) {
	payload, err := json.Marshal(struct {
		Left  CanonicalDocument `json:"left"`
		Right CanonicalDocument `json:"right"`
		Rules RuleSet           `json:"rules"`
	}{left, right, rules})
	if err != nil {
		return DiffResult{}, err
	}
	result, err := binding.endpoint.Invoke(ctx, plugin.Call{Capability: capabilityID(binding.descriptor), Method: "diff", ContentType: "application/json", Payload: payload})
	if err != nil {
		return DiffResult{}, err
	}
	var response DiffResult
	if err := json.Unmarshal(result.Payload, &response); err != nil {
		return DiffResult{}, err
	}
	return response, nil
}

func (binding *EndpointPlugin) CompileOverlay(ctx context.Context, dialect string, content []byte) ([]OverlayAction, error) {
	result, err := binding.endpoint.Invoke(ctx, plugin.Call{Capability: capabilityID(binding.descriptor), Method: "overlay", ContentType: "application/octet-stream", Payload: content, Metadata: map[string]string{"dialect": dialect}})
	if err != nil {
		return nil, err
	}
	var response []OverlayAction
	if err := json.Unmarshal(result.Payload, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func capabilityID(descriptor Descriptor) string { return "kind/" + descriptor.Kind }

// NewLocalEndpoint adapts a typed Kind plugin to the transport-neutral
// endpoint. It is useful for built-in adapters; external runtimes provide
// their own plugin.Endpoint implementation instead.
func NewLocalEndpoint(kindPlugin Plugin) plugin.Endpoint {
	return localEndpoint{plugin: kindPlugin, capability: capabilityID(kindPlugin.Descriptor())}
}

type Registry struct {
	mu          sync.RWMutex
	plugins     map[string]Plugin
	endpoints   map[string]plugin.Endpoint
	descriptors map[string]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin), endpoints: make(map[string]plugin.Endpoint), descriptors: make(map[string]Descriptor)}
}

func (registry *Registry) Register(plugin Plugin) error {
	if plugin == nil {
		return ErrInvalidKindPlugin
	}
	descriptor := plugin.Descriptor()
	if descriptor.Kind == "" || descriptor.PluginVersion == "" || descriptor.ContractVersion == "" {
		return ErrInvalidKindPlugin
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.endpoints[descriptor.Kind]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateKind, descriptor.Kind)
	}
	registry.plugins[descriptor.Kind] = plugin
	registry.endpoints[descriptor.Kind] = localEndpoint{plugin: plugin, capability: capabilityID(descriptor)}
	descriptor.Capabilities = slices.Clone(descriptor.Capabilities)
	registry.descriptors[descriptor.Kind] = descriptor
	return nil
}

// RegisterEndpoint binds a descriptor to any runtime endpoint, including a
// future RPC/process proxy. The registry does not inspect or own transport
// details beyond the endpoint lifecycle owned by the plugin host.
func (registry *Registry) RegisterEndpoint(descriptor Descriptor, endpoint plugin.Endpoint) error {
	if endpoint == nil || descriptor.Kind == "" || descriptor.PluginVersion == "" || descriptor.ContractVersion == "" {
		return ErrInvalidKindPlugin
	}
	binding, err := NewEndpointPlugin(descriptor, endpoint)
	if err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.endpoints[descriptor.Kind]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateKind, descriptor.Kind)
	}
	registry.plugins[descriptor.Kind] = binding
	registry.endpoints[descriptor.Kind] = endpoint
	descriptor.Capabilities = slices.Clone(descriptor.Capabilities)
	registry.descriptors[descriptor.Kind] = descriptor
	return nil
}

func (registry *Registry) Lookup(kind string) (Plugin, error) {
	registry.mu.RLock()
	plugin, ok := registry.plugins[kind]
	registry.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrKindUnavailable, kind)
	}
	return plugin, nil
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
