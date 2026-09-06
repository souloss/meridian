package contracttest

import (
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

type agentWorkItem struct {
	ID           string   `yaml:"id"`
	Milestone    string   `yaml:"milestone"`
	Status       string   `yaml:"status"`
	DependsOn    []string `yaml:"dependsOn"`
	OperationIDs []string `yaml:"operationIds"`
	Assertions   []string `yaml:"assertions"`
	Verify       []string `yaml:"verify"`
}

type agentQueue struct {
	Selection struct {
		OnlyStatuses                     []string `yaml:"onlyStatuses"`
		RequirePreviousMilestoneAccepted bool     `yaml:"requirePreviousMilestoneAccepted"`
	} `yaml:"selection"`
	MilestoneGates map[string]struct {
		Status         string `yaml:"status"`
		ReportPath     string `yaml:"reportPath"`
		AcceptedBy     string `yaml:"acceptedBy"`
		AcceptedAt     string `yaml:"acceptedAt"`
		AcceptedCommit string `yaml:"acceptedCommit"`
		ContractDigest string `yaml:"contractDigest"`
	} `yaml:"milestoneGates"`
	Items []agentWorkItem `yaml:"items"`
}

func readAgentQueue(t *testing.T) agentQueue {
	t.Helper()
	contents, err := os.ReadFile("../../contracts/work-items.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var queue agentQueue
	if err := yaml.Unmarshal(contents, &queue); err != nil {
		t.Fatal(err)
	}
	return queue
}

func readSmokeMilestones(t *testing.T) map[string]string {
	t.Helper()
	contents, err := os.ReadFile("../../contracts/acceptance.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var acceptance struct {
		SmokeSuite struct {
			Cases map[string]struct{ Milestone string } `yaml:"cases"`
		} `yaml:"smokeSuite"`
	}
	if err := yaml.Unmarshal(contents, &acceptance); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]string)
	for id, smoke := range acceptance.SmokeSuite.Cases {
		result[id] = smoke.Milestone
	}
	if len(result) == 0 {
		t.Fatal("acceptance Smoke suite is empty")
	}
	return result
}

func TestAgentQueueReferencesAndMilestoneGates(t *testing.T) {
	queue := readAgentQueue(t)
	smokes := readSmokeMilestones(t)
	if !slices.Equal(queue.Selection.OnlyStatuses, []string{"ready", "needs_retry"}) || !queue.Selection.RequirePreviousMilestoneAccepted {
		t.Fatal("selection must exclude active leases and require previous-stage acceptance")
	}
	operations := make(map[string]bool)
	for _, pathValue := range mapping(t, readOpenAPI(t), "paths") {
		for _, value := range asMapping(t, pathValue, "path") {
			if operation, ok := value.(map[string]any); ok {
				if id, ok := operation["operationId"].(string); ok {
					operations[id] = true
				}
			}
		}
	}
	items := make(map[string]agentWorkItem)
	covered := make(map[string]bool)
	gateName := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	for _, item := range queue.Items {
		if _, duplicate := items[item.ID]; duplicate || item.ID == "" {
			t.Fatalf("duplicate or empty work item: %q", item.ID)
		}
		items[item.ID] = item
		if _, exists := queue.MilestoneGates[item.Milestone]; !exists {
			t.Errorf("%s has no milestone acceptance record", item.ID)
		}
		if len(item.Verify) == 0 {
			t.Errorf("%s has no gates", item.ID)
		}
		for _, gate := range item.Verify {
			if !gateName.MatchString(gate) {
				t.Errorf("%s gate %q is not a Make target name", item.ID, gate)
			}
		}
		for _, operation := range item.OperationIDs {
			if !operations[operation] {
				t.Errorf("%s references unknown operation %s", item.ID, operation)
			}
		}
		for _, assertion := range item.Assertions {
			milestone, exists := smokes[assertion]
			if !exists {
				t.Errorf("%s references unknown Smoke %s", item.ID, assertion)
			}
			if milestone == item.Milestone {
				covered[assertion] = true
			}
		}
	}
	for id := range smokes {
		if !covered[id] {
			t.Errorf("%s has no owning work item in its acceptance milestone", id)
		}
	}
	visited, active := make(map[string]bool), make(map[string]bool)
	var visit func(string)
	visit = func(id string) {
		if active[id] {
			t.Fatalf("cyclic work-item dependency at %s", id)
		}
		if visited[id] {
			return
		}
		active[id] = true
		for _, dependency := range items[id].DependsOn {
			prior, exists := items[dependency]
			if !exists || prior.Milestone > items[id].Milestone {
				t.Errorf("%s has unknown or later-stage dependency %s", id, dependency)
				continue
			}
			visit(dependency)
		}
		active[id], visited[id] = false, true
	}
	for id := range items {
		visit(id)
	}
	for _, milestone := range []string{"M0", "M1", "M2", "M3", "M4", "M5"} {
		gate, exists := queue.MilestoneGates[milestone]
		if !exists || !slices.Contains([]string{"pending", "needs_human_acceptance", "accepted", "changes_requested"}, gate.Status) {
			t.Errorf("missing or invalid milestone gate %s", milestone)
		}
		if gate.Status == "accepted" && (gate.AcceptedBy == "" || gate.AcceptedAt == "" || gate.ReportPath == "" || gate.AcceptedCommit == "" || gate.ContractDigest == "") {
			t.Errorf("%s acceptance is missing human identity or evidence", milestone)
		}
	}
}

func TestSmokeCatalogMatchesAcceptance(t *testing.T) {
	contents, err := os.ReadFile("../../scripts/smoke-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]struct {
		Milestone string `json:"milestone"`
		Fixture   string `json:"fixture"`
		Test      string `json:"test"`
	}
	if err := json.Unmarshal(contents, &catalog); err != nil {
		t.Fatal(err)
	}
	acceptance := readSmokeMilestones(t)
	if len(catalog) != len(acceptance) {
		t.Fatal("Smoke catalog must include every acceptance case, including unimplemented fixtures")
	}
	for id, entry := range catalog {
		if acceptance[id] != entry.Milestone {
			t.Errorf("Smoke %s has wrong or unknown milestone %s", id, entry.Milestone)
		}
		if entry.Test != "" {
			if _, err := os.Stat(filepath.Join("../..", entry.Fixture)); err != nil {
				t.Errorf("registered Smoke %s fixture: %v", id, err)
			}
		}
	}
}

// Missing future gates are implementation work, not a prerequisite for claiming it.
func TestAgentGateAvailability(t *testing.T) {
	selected := os.Getenv("ITEM")
	for _, item := range readAgentQueue(t).Items {
		if selected != "" && selected != item.ID {
			continue
		}
		for _, gate := range item.Verify {
			command := exec.CommandContext(t.Context(), "make", "-n", gate, "ITEM="+item.ID)
			command.Dir = "../.."
			if output, err := command.CombinedOutput(); err != nil {
				t.Logf("%s/%s: tooling_gap; implement target/fixture in owning item before verification: %s", item.ID, gate, output)
			} else {
				t.Logf("%s/%s: target resolves (not proof of assertion coverage)", item.ID, gate)
			}
		}
	}
}
