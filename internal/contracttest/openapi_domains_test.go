package contracttest

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

type operationContract struct {
	OperationID string `yaml:"operationId"`
}

type openAPIContract struct {
	Paths map[string]map[string]operationContract `yaml:"paths"`
}

func TestOpenAPIOperationsArePartitionedByDomain(t *testing.T) {
	t.Parallel()

	canonical := loadOpenAPIContract(t, filepath.Join("..", "..", "contracts", "openapi.yaml"))
	canonicalIDs := operationIDs(canonical)
	if len(canonicalIDs) == 0 {
		t.Fatal("canonical OpenAPI contract contains no operations")
	}
	for operationID, occurrences := range operationIDOccurrences(canonical) {
		if occurrences != 1 {
			t.Fatalf("operationId %q appears %d times in canonical contract", operationID, occurrences)
		}
	}

	sourceIDs := make(map[string]string, len(canonicalIDs))
	sourcePaths := make(map[string]string)
	domainFiles, err := filepath.Glob(filepath.Join("..", "..", "contracts", "api", "domains", "*", "routes.tsp"))
	if err != nil {
		t.Fatalf("find domain OpenAPI contracts: %v", err)
	}
	if len(domainFiles) == 0 {
		t.Fatal("no domain OpenAPI contracts found")
	}
	for _, filename := range domainFiles {
		domain := filepath.Base(filepath.Dir(filename))
		contract := loadOpenAPIContract(t, filepath.Join("..", "..", "build", "contracts", "api", "domains", domain+".yaml"))
		for path := range contract.Paths {
			if previous, exists := sourcePaths[path]; exists {
				t.Fatalf("path %q appears in both %s and %s", path, previous, domain)
			}
			sourcePaths[path] = domain
		}
		for operationID, occurrences := range operationIDOccurrences(contract) {
			if occurrences != 1 {
				t.Fatalf("operationId %q appears %d times in %s", operationID, occurrences, domain)
			}
			if previous, exists := sourceIDs[operationID]; exists {
				t.Fatalf("operationId %q appears in both %s and %s", operationID, previous, domain)
			}
			sourceIDs[operationID] = domain
		}
	}

	if len(sourceIDs) != len(canonicalIDs) {
		t.Fatalf("source operation count = %d, canonical operation count = %d", len(sourceIDs), len(canonicalIDs))
	}
	for operationID := range canonicalIDs {
		if _, exists := sourceIDs[operationID]; !exists {
			t.Errorf("canonical operationId %q is missing from domain contracts", operationID)
		}
	}

	if _, err := os.Stat(filepath.Join("..", "..", "contracts", "api", "domains")); err != nil {
		t.Fatalf("contracts/api/domains is required: %v", err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "contracts", "api", "openapi.yaml")); !os.IsNotExist(err) {
		t.Fatal("contracts/api/openapi.yaml must be generated outside the source tree")
	}
	if _, err := os.Stat(filepath.Join("..", "..", "contracts", "api", "paths")); !os.IsNotExist(err) {
		t.Fatal("contracts/api/paths must not exist; domains are the only operation source")
	}
}

func loadOpenAPIContract(t *testing.T, filename string) openAPIContract {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	var contract openAPIContract
	if err := yaml.Unmarshal(contents, &contract); err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	return contract
}

func operationIDs(contract openAPIContract) map[string]struct{} {
	ids := make(map[string]struct{})
	for operationID := range operationIDOccurrences(contract) {
		ids[operationID] = struct{}{}
	}
	return ids
}

func operationIDOccurrences(contract openAPIContract) map[string]int {
	ids := make(map[string]int)
	for _, pathItem := range contract.Paths {
		for method, operation := range pathItem {
			switch method {
			case "get", "put", "post", "delete", "options", "head", "patch", "trace":
				if operation.OperationID != "" {
					ids[operation.OperationID]++
				}
			}
		}
	}
	return ids
}
