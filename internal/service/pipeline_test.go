package service

import (
	"testing"
)

// TestParseOpenAPIOperationsExtractsAndOrdersDeterministically verifies the
// shared OpenAPI extraction step: operations are parsed from YAML and returned
// in path-then-method order, which is the deterministic core exercised by the
// perf-openapi-pipeline gate.
func TestParseOpenAPIOperationsExtractsAndOrdersDeterministically(t *testing.T) {
	document := `openapi: 3.1.0
info:
  title: petstore
  version: 1.0.0
paths:
  /pets:
    get:
      summary: List pets
      tags: [pets]
    post:
      summary: Create pet
      deprecated: true
  /pets/{id}:
    get:
      summary: Get pet
  /zoo:
    delete:
      summary: Remove zoo
`
	operations, err := parseOpenAPIOperations([]byte(document))
	if err != nil {
		t.Fatalf("parse operations: %v", err)
	}
	if len(operations) != 4 {
		t.Fatalf("operations = %d, want 4", len(operations))
	}
	// Deterministic path-then-method ordering.
	wantKeys := []string{"GET /pets", "POST /pets", "GET /pets/{id}", "DELETE /zoo"}
	for index, want := range wantKeys {
		if got := operations[index].Method + " " + normalizePath(operations[index].Path); got != want {
			t.Fatalf("operation %d key = %q, want %q", index, got, want)
		}
	}
	if operations[1].Deprecated != true {
		t.Fatalf("POST /pets deprecated = %v, want true", operations[1].Deprecated)
	}
	if len(operations[0].Tags) != 1 || operations[0].Tags[0] != "pets" {
		t.Fatalf("GET /pets tags = %v, want [pets]", operations[0].Tags)
	}
}
