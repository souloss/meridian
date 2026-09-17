package service

import (
	"strings"
	"testing"
)

// TestRepositoryConfigRootDotNormalization verifies that the `.` service root
// is normalized to the empty string before preview/apply, per SMK-038.
func TestRepositoryConfigRootDotNormalization(t *testing.T) {
	t.Parallel()

	content := "version: 1\nservices:\n  - name: root\n    root: .\n    assets:\n      - kind: openapi\n        base: {mode: builtin, path: openapi.yaml}\n"
	config, err := ParseRepositoryConfig([]byte(content))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if len(config.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(config.Services))
	}
	if config.Services[0].Root != "" {
		t.Fatalf("root = %q, want empty string after normalization", config.Services[0].Root)
	}
}

// TestRepositoryConfigDigestDeterminism verifies that the config digest is
// deterministic and that equivalent YAML documents produce the same digest.
func TestRepositoryConfigDigestDeterminism(t *testing.T) {
	t.Parallel()

	first := "version: 1\nservices:\n  - name: a\n    root: api\n    assets:\n      - kind: openapi\n        base: {mode: builtin, path: openapi.yaml}\n"
	second := "version: 1\nservices:\n  - name: a\n    root: api\n    assets:\n      - kind: openapi\n        base:\n          mode: builtin\n          path: openapi.yaml\n"

	d1, c1, err := ConfigDigestFromBytes([]byte(first))
	if err != nil {
		t.Fatalf("digest first: %v", err)
	}
	d2, c2, err := ConfigDigestFromBytes([]byte(second))
	if err != nil {
		t.Fatalf("digest second: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("equivalent configs produced different digests: %s != %s", d1, d2)
	}
	_ = c1
	_ = c2
}

// TestRepositoryConfigInvalidRejected verifies that structurally invalid
// configurations are rejected.
func TestRepositoryConfigInvalidRejected(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"version: 2\nservices: []\n", // wrong version
		"version: 1\nservices:\n  - name: a\n    root: .\n    assets: []\n  - name: a\n    root: other\n    assets: []\n",      // duplicate service name
		"version: 1\nservices:\n  - name: a\n    root: .\n    assets:\n      - kind: openapi\n        base: {mode: builtin}\n", // builtin missing path
	} {
		if _, err := ParseRepositoryConfig([]byte(content)); err == nil {
			t.Fatalf("invalid config %q must be rejected", content)
		}
	}
}

var _ = strings.TrimSpace
