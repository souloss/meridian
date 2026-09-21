// Package ai defines the provider capability used by the unified plugin host.
// It intentionally contains no persistence or tenant implementation details.
package ai

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
	ErrInvalidProvider     = errors.New("invalid AI provider")
	ErrDuplicateProvider   = errors.New("duplicate AI provider")
	ErrProviderUnavailable = errors.New("AI provider unavailable")
)

type Descriptor struct {
	ID             string   `json:"id"`
	Version        string   `json:"version"`
	SupportedKinds []string `json:"supportedKinds,omitempty"`
}

type Request struct {
	Kind     string            `json:"kind"`
	Name     string            `json:"name"`
	Hint     string            `json:"hint,omitempty"`
	RefType  string            `json:"refType,omitempty"`
	Ref      string            `json:"ref,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Config   map[string]any    `json:"config,omitempty"`
}

type Result struct {
	Content     []byte         `json:"content"`
	ContentType string         `json:"contentType"`
	Manifest    map[string]any `json:"manifest,omitempty"`
	Stage       string         `json:"stage,omitempty"`
	ErrorCode   string         `json:"errorCode,omitempty"`
}

type Provider interface {
	Descriptor() Descriptor
	Generate(context.Context, Request) (Result, error)
}

// EndpointProvider adapts the transport-neutral plugin endpoint to the typed
// AI capability contract. A process/RPC runtime can therefore be used without
// changing the workflow-facing Provider interface.
type EndpointProvider struct {
	descriptor Descriptor
	endpoint   plugin.Endpoint
}

func NewEndpointProvider(descriptor Descriptor, endpoint plugin.Endpoint) (*EndpointProvider, error) {
	if endpoint == nil || descriptor.ID == "" || descriptor.Version == "" {
		return nil, ErrInvalidProvider
	}
	return &EndpointProvider{descriptor: descriptor, endpoint: endpoint}, nil
}

func (provider *EndpointProvider) Descriptor() Descriptor { return provider.descriptor }

func (provider *EndpointProvider) Generate(ctx context.Context, request Request) (Result, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return Result{}, err
	}
	response, invokeErr := provider.endpoint.Invoke(ctx, plugin.Call{
		Capability: "ai/" + provider.descriptor.ID, Method: "generate",
		ContentType: "application/json", Payload: payload,
	})
	var result Result
	if len(response.Payload) > 0 {
		if err := json.Unmarshal(response.Payload, &result); err != nil {
			return Result{}, err
		}
	}
	if invokeErr != nil {
		return result, invokeErr
	}
	if len(response.Payload) == 0 {
		return Result{}, errors.New("AI provider returned an empty response")
	}
	return result, nil
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (registry *Registry) Register(provider Provider) error {
	if provider == nil {
		return ErrInvalidProvider
	}
	descriptor := provider.Descriptor()
	if descriptor.ID == "" || descriptor.Version == "" {
		return ErrInvalidProvider
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.providers[descriptor.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateProvider, descriptor.ID)
	}
	registry.providers[descriptor.ID] = provider
	return nil
}

// Replace binds a provider ID to a new implementation. It is used when a
// typed in-process provider is upgraded to an endpoint-backed runtime proxy;
// callers observe one atomic provider selection.
func (registry *Registry) Replace(provider Provider) error {
	if provider == nil {
		return ErrInvalidProvider
	}
	descriptor := provider.Descriptor()
	if descriptor.ID == "" || descriptor.Version == "" {
		return ErrInvalidProvider
	}
	registry.mu.Lock()
	registry.providers[descriptor.ID] = provider
	registry.mu.Unlock()
	return nil
}

func (registry *Registry) Lookup(id string) (Provider, error) {
	registry.mu.RLock()
	provider, ok := registry.providers[id]
	registry.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, id)
	}
	return provider, nil
}

func (registry *Registry) Descriptors() []Descriptor {
	registry.mu.RLock()
	descriptors := make([]Descriptor, 0, len(registry.providers))
	for _, provider := range registry.providers {
		descriptor := provider.Descriptor()
		descriptor.SupportedKinds = slices.Clone(descriptor.SupportedKinds)
		descriptors = append(descriptors, descriptor)
	}
	registry.mu.RUnlock()
	slices.SortFunc(descriptors, func(left, right Descriptor) int {
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return 0
	})
	return descriptors
}

// Plugin adapts a typed provider to the transport-neutral plugin endpoint.
// A future RPC runtime can replace this adapter without changing Provider.
type Plugin struct{ provider Provider }

func NewPlugin(provider Provider) (*Plugin, error) {
	if provider == nil || provider.Descriptor().ID == "" || provider.Descriptor().Version == "" {
		return nil, ErrInvalidProvider
	}
	return &Plugin{provider: provider}, nil
}

func (adapter *Plugin) Manifest() plugin.Manifest {
	descriptor := adapter.provider.Descriptor()
	return plugin.Manifest{
		ID: "meridian.ai." + descriptor.ID, Version: descriptor.Version,
		ProtocolVersion: plugin.CurrentProtocolVersion,
		Capabilities:    []plugin.Capability{{ID: "ai/" + descriptor.ID, Version: descriptor.Version}},
	}
}

func (adapter *Plugin) Open(context.Context) (plugin.Endpoint, error) {
	return providerEndpoint{provider: adapter.provider, capability: "ai/" + adapter.provider.Descriptor().ID}, nil
}

type providerEndpoint struct {
	provider   Provider
	capability string
}

func (endpoint providerEndpoint) Invoke(ctx context.Context, call plugin.Call) (plugin.Result, error) {
	if call.Capability != endpoint.capability || call.Method != "generate" {
		return plugin.Result{}, fmt.Errorf("unknown AI call %s/%s", call.Capability, call.Method)
	}
	var request Request
	if err := json.Unmarshal(call.Payload, &request); err != nil {
		return plugin.Result{}, err
	}
	result, providerErr := endpoint.provider.Generate(ctx, request)
	payload, err := json.Marshal(result)
	if err != nil {
		return plugin.Result{}, err
	}
	return plugin.Result{ContentType: "application/json", Payload: payload}, providerErr
}

func (providerEndpoint) Close(context.Context) error { return nil }

var _ plugin.Factory = (*Plugin)(nil)
