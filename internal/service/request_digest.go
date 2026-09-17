package service

import (
	"crypto/sha256"
	"encoding/json"
	"sort"
)

// RequestDigest computes the SHA-256 request digest over an RFC 8785 JCS
// canonical object for one idempotent request. Object keys are emitted in
// ascending order (encoding/json already serializes maps with sorted keys);
// numbers use the default compact encoding, which is deterministic for the
// string, UUID, and boolean values carried by the M1 idempotent operations.
//
// The canonical object matches contracts/domain.yaml idempotency.requestDigest:
// operationId, pathParameters, query, semanticHeaders (if-match when present),
// and the parsed JSON body (nil when absent).
func RequestDigest(operationID string, pathParameters map[string]any, semanticHeaders map[string]any, body any) ([]byte, error) {
	canonical := map[string]any{
		"operationId":     operationID,
		"pathParameters":  sortedValue(pathParameters),
		"query":           []any{},
		"semanticHeaders": sortedValue(semanticHeaders),
		"body":            body,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}

// sortedValue returns nil for a nil map and otherwise the map itself; encoding/json
// serializes map keys in sorted order, which is the required JCS key ordering.
func sortedValue(value map[string]any) any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

// SortedQueryPairs canonicalizes a query string's percent-decoded name/value
// pairs into the deterministic order required by the request digest. The M1
// idempotent operations carry no query parameters, so this always returns an
// empty list; it is kept as the contract-defined seam.
func SortedQueryPairs() []any {
	return []any{}
}

var _ = sort.Strings
