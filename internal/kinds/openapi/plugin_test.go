package openapi

import (
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
