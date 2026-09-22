package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/meridian-labs/meridian/internal/plugin"
)

type testProvider struct{}

func (testProvider) Descriptor() Descriptor {
	return Descriptor{ID: "test", Version: "1", SupportedKinds: []string{"openapi"}}
}

func (testProvider) Generate(_ context.Context, request Request) (Result, error) {
	return Result{Content: []byte(request.Kind + ":" + request.Name), ContentType: "text/plain", Manifest: map[string]any{"ok": true}}, nil
}

func TestProviderCanUseGenericPluginEndpoint(t *testing.T) {
	t.Parallel()
	adapter, err := NewPlugin(testProvider{})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := adapter.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"kind":"openapi","name":"service"}`)
	result, err := endpoint.Invoke(t.Context(), plugin.Call{Capability: "ai/test", Method: "generate", ContentType: "application/json", Payload: request})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentType != "application/json" || len(result.Payload) == 0 {
		t.Fatalf("unexpected provider response: %#v", result)
	}
}

func TestRegistryReplacesProvider(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	provider := testProvider{}
	if err := registry.Replace(provider); err != nil {
		t.Fatal(err)
	}
	lookedUp, err := registry.Lookup("test")
	if err != nil {
		t.Fatal(err)
	}
	if lookedUp.Descriptor().ID != "test" {
		t.Fatalf("unexpected provider: %#v", lookedUp.Descriptor())
	}
}

type endpointProvider struct{}

func (endpointProvider) Invoke(_ context.Context, call plugin.Call) (plugin.Result, error) {
	return plugin.Result{ContentType: "application/json", Payload: []byte(`{"content":"b2s=","contentType":"text/plain"}`)}, nil
}

func (endpointProvider) Close(context.Context) error { return nil }

func TestEndpointProviderAdaptsRemoteRuntime(t *testing.T) {
	t.Parallel()
	provider, err := NewEndpointProvider(Descriptor{ID: "remote", Version: "1"}, endpointProvider{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Generate(t.Context(), Request{Kind: "openapi", Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Content) != "ok" || result.ContentType != "text/plain" {
		t.Fatalf("unexpected endpoint provider result: %#v", result)
	}
}

type failingProvider struct{}

func (failingProvider) Descriptor() Descriptor { return Descriptor{ID: "failing", Version: "1"} }

func (failingProvider) Generate(context.Context, Request) (Result, error) {
	return Result{Stage: "extract", ErrorCode: "producer_unavailable"}, errors.New("provider failed")
}

func TestEndpointProviderPreservesProviderResultOnError(t *testing.T) {
	t.Parallel()
	adapter, err := NewPlugin(failingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := adapter.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewEndpointProvider(failingProvider{}.Descriptor(), endpoint)
	if err != nil {
		t.Fatal(err)
	}
	result, err := proxy.Generate(t.Context(), Request{Kind: "openapi", Name: "orders"})
	if err == nil || result.Stage != "extract" || result.ErrorCode != "producer_unavailable" {
		t.Fatalf("provider result was not preserved: %#v %v", result, err)
	}
}
