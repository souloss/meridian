package dbschema

import (
	"testing"

	"github.com/meridian-labs/meridian/internal/kinds"
)

func TestPluginExtractsTablesAndColumns(t *testing.T) {
	t.Parallel()
	content := []byte("schemaVersion: meridian-dbschema-1\ntables:\n  - name: users\n    columns:\n      - name: id\n        dataType: uuid\n        nullable: false\n")
	plugin := NewPlugin()
	validated, report, err := plugin.Validate(t.Context(), document(content))
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
	if len(items) != 2 || items[0].Key != "users" || items[1].Key != "users.id" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestPluginDiffReportsRemovedItems(t *testing.T) {
	t.Parallel()
	plugin := NewPlugin()
	left := document([]byte("schemaVersion: meridian-dbschema-1\ntables:\n  - name: users\n    columns:\n      - name: id\n        dataType: uuid\n  - name: orders\n    columns:\n      - name: id\n        dataType: uuid\n"))
	right := document([]byte("schemaVersion: meridian-dbschema-1\ntables:\n  - name: users\n    columns:\n      - name: id\n        dataType: uuid\n"))
	leftCanonical := kinds.CanonicalDocument{Document: left, Version: SchemaVersion}
	rightCanonical := kinds.CanonicalDocument{Document: right, Version: SchemaVersion}
	diff, err := plugin.Diff(t.Context(), leftCanonical, rightCanonical, kinds.RuleSet{})
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Breaking || len(diff.Changes) != 2 {
		t.Fatalf("expected breaking diff with 2 removed items, got %#v", diff)
	}
}

func document(content []byte) kinds.Document {
	return kinds.Document{Content: content, MediaType: "application/yaml"}
}
