package kinds

import (
	"context"
	"encoding/json/v2"
	"slices"
	"testing"

	"github.com/meridian-labs/meridian/internal/plugin"
)

type remoteEndpoint struct{}

func (remoteEndpoint) Invoke(context.Context, plugin.Call) (plugin.Result, error) {
	return plugin.Result{Payload: []byte(`{"items":[]}`)}, nil
}

func (remoteEndpoint) Close(context.Context) error { return nil }

func TestRegistryAcceptsTransportNeutralEndpoint(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	descriptor := Descriptor{Kind: "custom", PluginVersion: "7", ContractVersion: "meridian-asset-kinds-v1"}
	if err := registry.RegisterEndpoint(descriptor, remoteEndpoint{}); err != nil {
		t.Fatal(err)
	}
	got, endpoint, err := registry.LookupEndpoint("custom")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != descriptor.Kind || got.PluginVersion != descriptor.PluginVersion || got.ContractVersion != descriptor.ContractVersion || !slices.Equal(got.Capabilities, descriptor.Capabilities) || endpoint == nil {
		t.Fatalf("unexpected endpoint binding: %#v %v", got, endpoint)
	}
	if typed, err := registry.Lookup("custom"); err != nil || typed.Descriptor().Kind != "custom" {
		t.Fatalf("expected endpoint-backed typed proxy, got %v", err)
	}
}

type contractPlugin struct {
	provenance ProvenanceMap
}

func (kindPlugin *contractPlugin) Descriptor() Descriptor {
	return Descriptor{Kind: "contract", PluginVersion: "1", ContractVersion: "meridian-asset-kinds-v1"}
}

func (*contractPlugin) Validate(context.Context, Document) (ValidatedDocument, ValidationReport, error) {
	return ValidatedDocument{}, ValidationReport{Valid: true}, nil
}

func (*contractPlugin) Normalize(context.Context, ValidatedDocument) (CanonicalDocument, ArtifactSet, error) {
	return CanonicalDocument{}, nil, nil
}

func (kindPlugin *contractPlugin) Extract(_ context.Context, _ CanonicalDocument, provenance ProvenanceMap) ([]Item, error) {
	kindPlugin.provenance = provenance
	return []Item{{ItemType: "test", Key: "one"}}, nil
}

func (*contractPlugin) Diff(context.Context, CanonicalDocument, CanonicalDocument, RuleSet) (DiffResult, error) {
	return DiffResult{Breaking: true}, nil
}

func (*contractPlugin) CompileOverlay(context.Context, string, []byte) ([]OverlayAction, error) {
	return nil, nil
}

func TestLocalEndpointPreservesProvenanceAndDiff(t *testing.T) {
	t.Parallel()
	kindPlugin := &contractPlugin{}
	endpoint := NewLocalEndpoint(kindPlugin)
	provenance := ProvenanceMap{"source": "test"}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint.Invoke(t.Context(), plugin.Call{
		Capability: "kind/contract", Method: "extract", Payload: []byte("content"),
		Metadata: map[string]string{"provenance": string(encoded), "canonical-version": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if kindPlugin.provenance["source"] != "test" || len(result.Payload) == 0 {
		t.Fatalf("provenance was not preserved: %#v", kindPlugin.provenance)
	}
	diffPayload, err := json.Marshal(struct {
		Left  CanonicalDocument `json:"left"`
		Right CanonicalDocument `json:"right"`
		Rules RuleSet           `json:"rules"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	result, err = endpoint.Invoke(t.Context(), plugin.Call{Capability: "kind/contract", Method: "diff", Payload: diffPayload})
	if err != nil {
		t.Fatal(err)
	}
	var diff DiffResult
	if err := json.Unmarshal(result.Payload, &diff); err != nil {
		t.Fatal(err)
	}
	if !diff.Breaking {
		t.Fatalf("expected diff result from typed plugin: %#v", diff)
	}
}
