package contracttest

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestSourceExpansionOwnsItsSemanticStructure(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile("../../contracts/domain.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		SourceExpansion struct {
			ConfiguredEntity   string         `yaml:"configuredEntity"`
			MaterializedEntity string         `yaml:"materializedEntity"`
			Expansion          map[string]any `yaml:"expansion"`
		} `yaml:"sourceExpansion"`
	}
	if err := yaml.Unmarshal(contents, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.SourceExpansion.ConfiguredEntity != "source_specs" {
		t.Errorf("sourceExpansion.configuredEntity = %q, want source_specs", contract.SourceExpansion.ConfiguredEntity)
	}
	if contract.SourceExpansion.MaterializedEntity != "source_bindings" {
		t.Errorf("sourceExpansion.materializedEntity = %q, want source_bindings", contract.SourceExpansion.MaterializedEntity)
	}
	if len(contract.SourceExpansion.Expansion) == 0 {
		t.Error("sourceExpansion.expansion must define materialization behavior")
	}
}
