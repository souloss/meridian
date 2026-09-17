package service

import (
	"encoding/json"
	"testing"
)

func TestRequestDigestDeterministicAcrossKeyOrder(t *testing.T) {
	body := map[string]any{"refType": "branch", "ref": "main"}
	first, err := RequestDigest("syncRepository", map[string]any{"tenantSlug": "acme", "repositoryId": "11111111-1111-4111-8111-111111111111"}, map[string]any{}, body)
	if err != nil {
		t.Fatalf("digest first: %v", err)
	}
	second, err := RequestDigest("syncRepository", map[string]any{"repositoryId": "11111111-1111-4111-8111-111111111111", "tenantSlug": "acme"}, map[string]any{}, body)
	if err != nil {
		t.Fatalf("digest second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("digest differs across path-parameter insertion order")
	}
	if len(first) != 32 {
		t.Fatalf("digest length = %d, want 32", len(first))
	}
}

func TestRequestDigestDiffersForDifferentBody(t *testing.T) {
	left, _ := RequestDigest("syncRepository", map[string]any{"tenantSlug": "acme"}, map[string]any{}, map[string]any{"ref": "main"})
	right, _ := RequestDigest("syncRepository", map[string]any{"tenantSlug": "acme"}, map[string]any{}, map[string]any{"ref": "develop"})
	if string(left) == string(right) {
		t.Fatalf("digest did not differ across bodies")
	}
}

var _ = json.Marshal
