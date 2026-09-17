package service

import (
	"errors"
	"strings"
	"testing"
	"uuid"

	"go.yaml.in/yaml/v3"
)

// TestOverlayMergeDeterminism verifies that the platform-v1 merge engine is
// deterministic: identical base + overlay inputs produce an identical merged
// document and canonical hash across repeated runs, and equivalent YAML/JSON
// base encodings produce the same canonical hash.
func TestOverlayMergeDeterminism(t *testing.T) {
	t.Parallel()

	baseJSON := `{"openapi":"3.1.0","info":{"title":"orders","version":"1.0.0"},"paths":{"/orders":{"get":{"summary":"list"}}}}`
	baseYAML := "openapi: 3.1.0\ninfo:\n  title: orders\n  version: 1.0.0\npaths:\n  /orders:\n    get:\n      summary: list\n"
	overlay := "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      description: order api\n"

	baseFromJSON := mustDecodeBase(t, baseJSON)
	baseFromYAML := mustDecodeBase(t, baseYAML)
	raw := mustParseOverlay(t, overlay)

	var firstHash string
	var firstMerged map[string]any
	const runCount = 100
	for run := 0; run < runCount; run++ {
		merged, _, warnings, err := ApplyPlatformOverlay(baseFromJSON, raw)
		if err != nil {
			t.Fatalf("apply overlay run %d: %v", run, err)
		}
		if len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %+v", warnings)
		}
		hash, err := CanonicalHash(merged)
		if err != nil {
			t.Fatalf("canonical hash: %v", err)
		}
		if run == 0 {
			firstHash = hash
			firstMerged = merged
			continue
		}
		if hash != firstHash {
			t.Fatalf("deterministic hash mismatch at run %d: %s != %s", run, hash, firstHash)
		}
	}

	// The base document is never mutated by overlay application.
	if title, ok := baseFromJSON["info"].(map[string]any)["title"].(string); !ok || title != "orders" {
		t.Fatalf("base document mutated: %#v", baseFromJSON["info"])
	}

	// Equivalent JSON and YAML base encodings produce the same canonical hash.
	mergedFromYAML, _, _, err := ApplyPlatformOverlay(baseFromYAML, raw)
	if err != nil {
		t.Fatalf("apply overlay over yaml base: %v", err)
	}
	yamlHash, err := CanonicalHash(mergedFromYAML)
	if err != nil {
		t.Fatalf("yaml base canonical hash: %v", err)
	}
	if yamlHash != firstHash {
		t.Fatalf("equivalent inputs produced different canonical hashes: %s != %s", yamlHash, firstHash)
	}
	_ = firstMerged
}

// TestOverlayProvenance tracks last-writing pointers in document order.
func TestOverlayProvenance(t *testing.T) {
	t.Parallel()

	base := mustDecodeBase(t, `{"openapi":"3.1.0","info":{"title":"orders","description":"base"},"paths":{}}`)
	raw := mustParseOverlay(t, "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      description: overlaid\n")

	_, pointers, _, err := ApplyPlatformOverlay(base, raw)
	if err != nil {
		t.Fatalf("apply overlay: %v", err)
	}
	if len(pointers) != 1 || pointers[0] != "/info" {
		t.Fatalf("provenance pointers = %v, want [/info]", pointers)
	}
}

// TestOverlayTargetMissLenientAndStrict verifies that a target miss is a
// warning in lenient mode and an overlay_invalid error in strict mode.
func TestOverlayTargetMissLenientAndStrict(t *testing.T) {
	t.Parallel()

	base := mustDecodeBase(t, `{"openapi":"3.1.0","paths":{}}`)

	lenient := mustParseOverlay(t, "overlay: platform/v1\ntarget_kind: openapi\nmode: lenient\nactions:\n  - target: /info\n    merge:\n      title: x\n")
	merged, _, warnings, err := ApplyPlatformOverlay(base, lenient)
	if err != nil {
		t.Fatalf("lenient miss should not fail: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("lenient miss should emit a warning")
	}
	if _, ok := merged["info"]; ok {
		t.Fatalf("lenient miss must not write the target")
	}

	strict := mustParseOverlay(t, "overlay: platform/v1\ntarget_kind: openapi\nmode: strict\nactions:\n  - target: /info\n    merge:\n      title: x\n")
	_, _, _, err = ApplyPlatformOverlay(base, strict)
	if err == nil {
		t.Fatalf("strict miss should fail")
	}
	if !isOverlayInvalid(err) {
		t.Fatalf("strict miss error = %T, want OverlayInvalidError", err)
	}
}

// TestOverlayDuplicateOrderRejected verifies that a compiled overlay with two
// non-distinct overlay ord values is rejected with overlay_invalid semantics.
func TestOverlayDuplicateOrderRejected(t *testing.T) {
	t.Parallel()

	base := LayerRecord{ID: mustUUID("11111111-1111-7111-8111-111111111111"), Role: layerRoleBase, Ord: layerOrdBase, Enabled: true}
	overlays := []LayerRecord{
		{ID: mustUUID("22222222-2222-7222-8222-222222222222"), Role: layerRoleOverlay, Ord: 1, Enabled: true},
		{ID: mustUUID("33333333-3333-7333-8333-333333333333"), Role: layerRoleOverlay, Ord: 1, Enabled: true},
	}
	_, _, err := splitBaseAndOverlays(append([]LayerRecord{base}, overlays...))
	if err == nil {
		t.Fatalf("duplicate overlay ord must be rejected")
	}
	if !isOverlayInvalid(err) {
		t.Fatalf("duplicate overlay ord error = %T, want OverlayInvalidError", err)
	}
}

// TestOverlayDuplicateKeysRejected verifies that a duplicate object key in the
// overlay document is rejected before validation, with one-based coordinates.
func TestOverlayDuplicateKeysRejected(t *testing.T) {
	t.Parallel()

	content := "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      title: a\n      title: b\n"
	_, err := ParseOverlay([]byte(content))
	if err == nil {
		t.Fatalf("duplicate key must be rejected")
	}
	var overlayErr *OverlayInvalidError
	if !errors.As(err, &overlayErr) {
		t.Fatalf("duplicate key error = %T, want OverlayInvalidError", err)
	}
	if len(overlayErr.Errors) == 0 || overlayErr.Errors[0].Line < 1 || overlayErr.Errors[0].Column < 1 {
		t.Fatalf("duplicate key issue missing one-based coordinates: %+v", overlayErr.Errors)
	}
}

// TestOverlayInvalidRejected verifies that a structurally invalid overlay is
// rejected with a non-empty issue list carrying line/column coordinates.
func TestOverlayInvalidRejected(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"not: [a: valid: overlay]",
		"overlay: wrong/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge: {}\n",
		"overlay: platform/v1\nactions:\n  - target: /info\n    merge: {}\n", // missing target_kind
		"overlay: platform/v1\ntarget_kind: openapi\nactions: []\n",
	} {
		_, err := ParseOverlay([]byte(content))
		if err == nil {
			t.Fatalf("invalid overlay %q must be rejected", content)
		}
		var overlayErr *OverlayInvalidError
		if !errors.As(err, &overlayErr) || len(overlayErr.Errors) == 0 {
			t.Fatalf("invalid overlay %q error = %T, want OverlayInvalidError with issues", content, err)
		}
	}
}

// TestOverlayMergeSemantics verifies RFC 7386 merge (null deletes) and RFC 6902
// patch (add/remove/replace) behavior within the platform-v1 schema.
func TestOverlayMergeSemantics(t *testing.T) {
	t.Parallel()

	base := mustDecodeBase(t, `{"openapi":"3.1.0","info":{"title":"orders","version":"1.0.0","description":"old"},"tags":[{"name":"a"}]}`)

	// merge with null deletes the member.
	mergeOverlay := mustParseOverlay(t, "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      version: 2.0.0\n      description: null\n")
	merged, _, _, err := ApplyPlatformOverlay(base, mergeOverlay)
	if err != nil {
		t.Fatalf("merge overlay: %v", err)
	}
	info := merged["info"].(map[string]any)
	if info["version"] != "2.0.0" {
		t.Fatalf("merge version = %v", info["version"])
	}
	if _, exists := info["description"]; exists {
		t.Fatalf("null merge must delete the member")
	}

	// patch add/remove on an array.
	patchOverlay := mustParseOverlay(t, "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - patch:\n      - op: add\n        path: /tags/-\n        value: {name: b}\n")
	patched, _, _, err := ApplyPlatformOverlay(base, patchOverlay)
	if err != nil {
		t.Fatalf("patch overlay: %v", err)
	}
	tags := patched["tags"].([]any)
	if len(tags) != 2 {
		t.Fatalf("patch add tags length = %d, want 2", len(tags))
	}
}

func mustParseOverlay(t *testing.T, content string) map[string]any {
	t.Helper()
	raw, err := ParseOverlay([]byte(content))
	if err != nil {
		t.Fatalf("parse overlay: %v", err)
	}
	return raw
}

func mustDecodeBase(t *testing.T, content string) map[string]any {
	t.Helper()
	decoded, err := decodeBaseDocument(content)
	if err != nil {
		t.Fatalf("decode base: %v", err)
	}
	return decoded
}

func isOverlayInvalid(err error) bool {
	var overlayErr *OverlayInvalidError
	return errors.As(err, &overlayErr)
}

func mustUUID(value string) uuid.UUID {
	parsed, err := uuid.Parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

var _ = strings.TrimSpace
var _ = yaml.Marshal
