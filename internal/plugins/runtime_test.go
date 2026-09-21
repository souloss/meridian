package plugins

import (
	"context"
	"testing"

	"github.com/meridian-labs/meridian/internal/ai"
)

func TestRuntimeInstallsCoreKindThroughHost(t *testing.T) {
	runtime, err := New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(t.Context())
	if _, endpoint, err := runtime.Kinds.LookupEndpoint("openapi"); err != nil || endpoint == nil {
		t.Fatalf("openapi endpoint unavailable: %v", err)
	}
	if len(runtime.Host.Manifests()) != 3 || runtime.Host.Manifests()[0].ID != "meridian.kind.dbschema" || runtime.Host.Manifests()[1].ID != "meridian.kind.dependency" || runtime.Host.Manifests()[2].ID != "meridian.kind.openapi" {
		t.Fatalf("unexpected core manifests: %#v", runtime.Host.Manifests())
	}
}

type testAIProvider struct{}

func (testAIProvider) Descriptor() ai.Descriptor { return ai.Descriptor{ID: "test", Version: "1"} }

func (testAIProvider) Generate(context.Context, ai.Request) (ai.Result, error) {
	return ai.Result{Content: []byte("ok"), ContentType: "text/plain"}, nil
}

func TestRuntimeRoutesAIProviderThroughHost(t *testing.T) {
	runtime, err := New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(t.Context())
	if err := runtime.RegisterAIProvider(t.Context(), testAIProvider{}); err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.AI.Lookup("test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Generate(t.Context(), ai.Request{Kind: "openapi", Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Content) != "ok" {
		t.Fatalf("unexpected provider result: %#v", result)
	}
	if len(runtime.Host.Manifests()) != 4 {
		t.Fatalf("expected OpenAPI and AI manifests, got %#v", runtime.Host.Manifests())
	}
}

func TestRuntimeAcceptsExternalAIEndpointFactory(t *testing.T) {
	runtime, err := New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(t.Context())
	factory, err := ai.NewPlugin(testAIProvider{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.RegisterAIEndpoint(t.Context(), factory, testAIProvider{}.Descriptor()); err != nil {
		t.Fatal(err)
	}
	provider, err := runtime.AI.Lookup("test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Generate(t.Context(), ai.Request{Kind: "openapi", Name: "orders"})
	if err != nil || string(result.Content) != "ok" {
		t.Fatalf("unexpected external endpoint result: %#v %v", result, err)
	}
}
