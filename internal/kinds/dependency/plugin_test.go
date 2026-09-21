package dependency

import (
	"testing"

	"github.com/meridian-labs/meridian/internal/kinds"
)

func TestPluginExtractsServiceEdges(t *testing.T) {
	t.Parallel()
	content := []byte("schemaVersion: meridian-dependency-1\nedges:\n  - from: orders\n    toServiceSlug: billing\n    protocol: http\n")
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
	if len(items) != 1 || items[0].Key != "orders->billing:http:default" {
		t.Fatalf("unexpected items: %#v", items)
	}
}
