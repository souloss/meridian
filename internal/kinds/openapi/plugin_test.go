package openapi

import (
	"encoding/json/v2"
	"testing"

	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/plugin"
)

func TestPluginExtractsDeterministically(t *testing.T) {
	t.Parallel()
	content := []byte("openapi: 3.1.0\npaths:\n  /z:\n    post:\n      summary: create\n    get:\n      summary: list\n  /a:\n    get:\n      tags: [read]\n")
	plugin := NewPlugin()
	validated, report, err := plugin.Validate(t.Context(), kinds.Document{Content: content, MediaType: "application/yaml"})
	if err != nil || !report.Valid {
		t.Fatalf("validation failed: %v %#v", err, report)
	}
	canonical, _, err := plugin.Normalize(t.Context(), validated)
	if err != nil {
		t.Fatal(err)
	}
	items, err := plugin.Extract(t.Context(), canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Key != "GET /a" || items[1].Key != "GET /z" || items[2].Key != "POST /z" {
		t.Fatalf("unexpected extracted items: %#v", items)
	}
}

func TestPluginEndpointCanBeInvokedThroughGenericContract(t *testing.T) {
	t.Parallel()
	endpoint, err := NewPlugin().Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint.Invoke(t.Context(), plugin.Call{Capability: "kind/openapi", Method: "extract", ContentType: "application/yaml", Payload: []byte("openapi: 3.1.0\npaths:\n  /health:\n    get: {}\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Payload) == 0 || result.ContentType != "application/json" {
		t.Fatalf("unexpected endpoint response: %#v", result)
	}
}

func TestPluginDiffReportsRemovedOperations(t *testing.T) {
	t.Parallel()
	endpoint, err := NewPlugin().Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	left := `openapi: 3.1.0
paths:
  /a:
    get: {}
  /b:
    post: {}
`
	right := `openapi: 3.1.0
paths:
  /a:
    get: {}
`
	payload, err := json.Marshal(struct {
		Left  kinds.CanonicalDocument `json:"left"`
		Right kinds.CanonicalDocument `json:"right"`
		Rules kinds.RuleSet           `json:"rules"`
	}{
		Left:  kinds.CanonicalDocument{Document: kinds.Document{Content: []byte(left), MediaType: "application/yaml"}, Version: "openapi-3.1"},
		Right: kinds.CanonicalDocument{Document: kinds.Document{Content: []byte(right), MediaType: "application/yaml"}, Version: "openapi-3.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint.Invoke(t.Context(), plugin.Call{Capability: "kind/openapi", Method: "diff", ContentType: "application/json", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var diff kinds.DiffResult
	if err := json.Unmarshal(result.Payload, &diff); err != nil {
		t.Fatal(err)
	}
	if !diff.Breaking || len(diff.Changes) != 1 || diff.Changes[0][kinds.DiffChangePath] != "POST /b" {
		t.Fatalf("unexpected diff result: %#v", diff)
	}
}
