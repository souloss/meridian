package contracttest

import (
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

type agentWorkItem struct {
	ID           string   `yaml:"id"`
	Milestone    string   `yaml:"milestone"`
	Status       string   `yaml:"status"`
	DependsOn    []string `yaml:"dependsOn"`
	OperationIDs []string `yaml:"operationIds"`
	Stories      []string `yaml:"stories"`
	Assertions   []string `yaml:"assertions"`
	Verify       []string `yaml:"verify"`
}

type agentQueue struct {
	Selection struct {
		OnlyStatuses                      []string `yaml:"onlyStatuses"`
		RequireEarlierMilestonesCompleted bool     `yaml:"requireEarlierMilestonesCompleted"`
	} `yaml:"selection"`
	MilestoneCheckpointPolicy struct {
		Statuses                                  []string `yaml:"statuses"`
		CompletionAuthority                       string   `yaml:"completionAuthority"`
		AllMilestoneItemsMustPassBeforeCompletion bool     `yaml:"allMilestoneItemsMustPassBeforeCompletion"`
		CompletedRequires                         []string `yaml:"completedRequires"`
		NextMilestoneRequiresPreviousCompleted    bool     `yaml:"nextMilestoneRequiresPreviousCompleted"`
		StaleSourceOrContractReturnsTo            string   `yaml:"staleSourceOrContractReturnsTo"`
	} `yaml:"milestoneCheckpointPolicy"`
	MilestoneCheckpoints map[string]struct {
		Status          string `yaml:"status"`
		ReportPath      string `yaml:"reportPath"`
		CompletedBy     string `yaml:"completedBy"`
		CompletedAt     string `yaml:"completedAt"`
		CompletedCommit string `yaml:"completedCommit"`
		ContractDigest  string `yaml:"contractDigest"`
	} `yaml:"milestoneCheckpoints"`
	Items []agentWorkItem `yaml:"items"`
}

type milestoneCompletionReport struct {
	Milestone      string `json:"milestone"`
	Status         string `json:"status"`
	Commit         string `json:"commit"`
	ContractDigest string `json:"contractDigest"`
	ReportPath     string `json:"reportPath"`
	CompletedBy    string `json:"completedBy"`
	CompletedAt    string `json:"completedAt"`
	WorkItems      []struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Evidence string `json:"evidence"`
	} `json:"workItems"`
	Stories []struct {
		ID       string   `json:"id"`
		Status   string   `json:"status"`
		Evidence []string `json:"evidence"`
	} `json:"stories"`
	Assertions []struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Evidence string `json:"evidence"`
	} `json:"assertions"`
	Commands []struct {
		Command  string `json:"command"`
		ExitCode int    `json:"exitCode"`
		Evidence string `json:"evidence"`
	} `json:"commands"`
	Failures []any `json:"failures"`
	Deferred []any `json:"deferred"`
}

type workItemEvidenceReport struct {
	WorkItem string `json:"workItem"`
	Status   string `json:"status"`
}

type smokeEvidenceReport struct {
	Result         string `json:"result"`
	Commit         string `json:"commit"`
	ContractDigest string `json:"contractDigest"`
}

type evidenceReference struct {
	Status   string
	Evidence string
}

type smokeRequirement struct {
	Milestone            string   `yaml:"milestone"`
	RequiredOperationIDs []string `yaml:"requiredOperationIds"`
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

func readSmokeRequirements(t *testing.T) map[string]smokeRequirement {
	t.Helper()
	contents, err := os.ReadFile("../../contracts/acceptance.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var acceptance struct {
		SmokeSuite struct {
			Cases map[string]smokeRequirement `yaml:"cases"`
		} `yaml:"smokeSuite"`
	}
	if err := yaml.Unmarshal(contents, &acceptance); err != nil {
		t.Fatal(err)
	}
	if len(acceptance.SmokeSuite.Cases) == 0 {
		t.Fatal("acceptance Smoke suite is empty")
	}
	return acceptance.SmokeSuite.Cases
}

func currentContractDigest(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir("../../contracts")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") || name == "work-items.yaml" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join("../../contracts", name))
		if err != nil {
			t.Fatal(err)
		}
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(contents)
		hash.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

func localEvidencePath(t *testing.T, path, prefix string) string {
	t.Helper()
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if !filepath.IsLocal(path) || !strings.HasPrefix(cleaned, prefix) || cleaned == strings.TrimSuffix(prefix, "/") {
		t.Errorf("evidence %q must be a local file below %s", path, prefix)
		return ""
	}
	fullPath := filepath.Join("../..", filepath.FromSlash(cleaned))
	if info, err := os.Stat(fullPath); err != nil || info.IsDir() {
		t.Errorf("evidence %q is not a readable file: %v", path, err)
		return ""
	}
	return fullPath
}

func TestAgentQueueReferencesAndMilestoneCheckpoints(t *testing.T) {
	queue := readAgentQueue(t)
	smokes := readSmokeRequirements(t)
	if !slices.Equal(queue.Selection.OnlyStatuses, []string{"ready", "needs_retry"}) || !queue.Selection.RequireEarlierMilestonesCompleted {
		t.Fatal("selection must exclude active leases and require earlier milestone completion")
	}
	policy := queue.MilestoneCheckpointPolicy
	if !slices.Equal(policy.Statuses, []string{"pending", "completed"}) ||
		policy.CompletionAuthority != "coding-agent" ||
		!policy.AllMilestoneItemsMustPassBeforeCompletion ||
		!policy.NextMilestoneRequiresPreviousCompleted ||
		policy.StaleSourceOrContractReturnsTo != "pending" ||
		!slices.Equal(policy.CompletedRequires, []string{"completedBy", "completedAt", "reportPath", "completedCommit", "contractDigest"}) {
		t.Fatal("milestone checkpoint policy must require evidence-backed coding-agent completion")
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
		if _, exists := queue.MilestoneCheckpoints[item.Milestone]; !exists {
			t.Errorf("%s has no milestone checkpoint", item.ID)
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
			smoke, exists := smokes[assertion]
			if !exists {
				t.Errorf("%s references unknown Smoke %s", item.ID, assertion)
			}
			if smoke.Milestone == item.Milestone {
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
	for _, item := range queue.Items {
		available := make(map[string]bool)
		var collect func(string)
		collect = func(id string) {
			for _, operation := range items[id].OperationIDs {
				available[operation] = true
			}
			for _, dependency := range items[id].DependsOn {
				collect(dependency)
			}
		}
		collect(item.ID)
		for _, assertion := range item.Assertions {
			for _, operation := range smokes[assertion].RequiredOperationIDs {
				if !operations[operation] {
					t.Errorf("Smoke %s references unknown required operation %s", assertion, operation)
				} else if !available[operation] {
					t.Errorf("%s cannot complete %s: operation %s is not owned by the item or its dependencies", item.ID, assertion, operation)
				}
			}
		}
	}
	milestones := []string{"M0", "M1", "M2", "M3", "M4", "M5", "M6"}
	commitPattern := regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	activeStatuses := []string{"claimed", "in_progress", "verifying"}
	currentDigest := currentContractDigest(t)
	for index, milestone := range milestones {
		checkpoint, exists := queue.MilestoneCheckpoints[milestone]
		if !exists || !slices.Contains(policy.Statuses, checkpoint.Status) {
			t.Errorf("missing or invalid milestone checkpoint %s", milestone)
			continue
		}
		if checkpoint.Status == "completed" {
			if checkpoint.CompletedBy != policy.CompletionAuthority {
				t.Errorf("%s checkpoint must be completed by %s", milestone, policy.CompletionAuthority)
			}
			if _, err := time.Parse(time.RFC3339, checkpoint.CompletedAt); err != nil {
				t.Errorf("%s checkpoint has invalid completedAt: %v", milestone, err)
			}
			if !commitPattern.MatchString(checkpoint.CompletedCommit) || !digestPattern.MatchString(checkpoint.ContractDigest) {
				t.Errorf("%s checkpoint has invalid commit or contract digest", milestone)
			} else if checkpoint.ContractDigest != currentDigest {
				t.Errorf("%s checkpoint contract digest is stale", milestone)
			} else {
				command := exec.CommandContext(t.Context(), "git", "merge-base", "--is-ancestor", checkpoint.CompletedCommit, "HEAD")
				command.Dir = "../.."
				if output, err := command.CombinedOutput(); err != nil {
					t.Errorf("%s checkpoint commit is not in the current history: %v: %s", milestone, err, output)
				}
			}
			expectedPrefix := "artifacts/agent/milestones/" + milestone + "/"
			reportFile := localEvidencePath(t, checkpoint.ReportPath, expectedPrefix)
			if reportFile != "" {
				contents, err := os.ReadFile(reportFile)
				if err != nil {
					t.Errorf("%s checkpoint report: %v", milestone, err)
				} else {
					var report milestoneCompletionReport
					if err := json.Unmarshal(contents, &report); err != nil {
						t.Errorf("%s checkpoint report: %v", milestone, err)
					} else if report.Milestone != milestone || report.Status != "completed" ||
						report.Commit != checkpoint.CompletedCommit || report.ContractDigest != checkpoint.ContractDigest ||
						report.ReportPath != checkpoint.ReportPath || report.CompletedBy != checkpoint.CompletedBy ||
						report.CompletedAt != checkpoint.CompletedAt {
						t.Errorf("%s checkpoint report does not match queue metadata", milestone)
					} else {
						validateMilestoneCompletionEvidence(t, queue, smokes, report, currentDigest)
					}
				}
			}
			for _, item := range queue.Items {
				if item.Milestone == milestone && !slices.Contains([]string{"passed", "superseded", "cancelled"}, item.Status) {
					t.Errorf("%s cannot be completed while %s is %s", milestone, item.ID, item.Status)
				}
			}
			for _, earlier := range milestones[:index] {
				if queue.MilestoneCheckpoints[earlier].Status != "completed" {
					t.Errorf("%s cannot be completed before %s", milestone, earlier)
				}
			}
		}
		for _, item := range queue.Items {
			if item.Milestone != milestone || !slices.Contains(activeStatuses, item.Status) {
				continue
			}
			for _, earlier := range milestones[:index] {
				if queue.MilestoneCheckpoints[earlier].Status != "completed" {
					t.Errorf("%s cannot be %s before %s is completed", item.ID, item.Status, earlier)
				}
			}
		}
	}
}

func validateMilestoneCompletionEvidence(t *testing.T, queue agentQueue, smokes map[string]smokeRequirement, report milestoneCompletionReport, currentDigest string) {
	t.Helper()
	// 里程碑若在 acceptance.yaml 声明了 story，则完成报告必须为每个 story 提供通过证据；
	// 无 story 的里程碑（如 M6 契约覆盖收口）不强制 story 列表，但仍须有命令证据且零失败。
	hasStories := false
	for _, item := range queue.Items {
		if item.Milestone == report.Milestone && len(item.Stories) > 0 {
			hasStories = true
		}
	}
	if (hasStories && len(report.Stories) == 0) || len(report.Commands) == 0 || report.Failures == nil || len(report.Failures) != 0 || report.Deferred == nil {
		t.Errorf("%s completion report must include passed stories and commands with no failures", report.Milestone)
	}
	workItemEvidence := make(map[string]evidenceReference)
	for _, entry := range report.WorkItems {
		if _, duplicate := workItemEvidence[entry.ID]; duplicate || entry.ID == "" {
			t.Errorf("%s completion report contains duplicate or empty work item evidence", report.Milestone)
		}
		workItemEvidence[entry.ID] = evidenceReference{entry.Status, entry.Evidence}
	}
	for _, item := range queue.Items {
		if item.Milestone != report.Milestone || slices.Contains([]string{"superseded", "cancelled"}, item.Status) {
			continue
		}
		entry, exists := workItemEvidence[item.ID]
		if !exists || entry.Status != "passed" {
			t.Errorf("%s completion report is missing passed work item %s", report.Milestone, item.ID)
			continue
		}
		file := localEvidencePath(t, entry.Evidence, "artifacts/agent/"+item.ID+"/")
		if file == "" {
			continue
		}
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Error(err)
			continue
		}
		var evidence workItemEvidenceReport
		if err := json.Unmarshal(contents, &evidence); err != nil || evidence.WorkItem != item.ID || evidence.Status != "passed" {
			t.Errorf("%s work item evidence is not a matching passed report: %v", item.ID, err)
		}
	}
	assertionEvidence := make(map[string]evidenceReference)
	for _, entry := range report.Assertions {
		if _, duplicate := assertionEvidence[entry.ID]; duplicate || entry.ID == "" {
			t.Errorf("%s completion report contains duplicate or empty assertion evidence", report.Milestone)
		}
		assertionEvidence[entry.ID] = evidenceReference{entry.Status, entry.Evidence}
	}
	for id, requirement := range smokes {
		if requirement.Milestone != report.Milestone {
			continue
		}
		entry, exists := assertionEvidence[id]
		if !exists || entry.Status != "passed" {
			t.Errorf("%s completion report is missing passed assertion %s", report.Milestone, id)
			continue
		}
		file := localEvidencePath(t, entry.Evidence, "artifacts/smoke/")
		if file == "" {
			continue
		}
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Error(err)
			continue
		}
		var evidence smokeEvidenceReport
		if err := json.Unmarshal(contents, &evidence); err != nil || evidence.Result != "pass" ||
			evidence.Commit != report.Commit || evidence.ContractDigest != currentDigest {
			t.Errorf("%s assertion evidence is stale or did not pass: %v", id, err)
		}
	}
	for _, story := range report.Stories {
		if story.ID == "" || story.Status != "passed" || len(story.Evidence) == 0 {
			t.Errorf("%s completion report contains incomplete story evidence", report.Milestone)
		}
		for _, assertion := range story.Evidence {
			if entry, exists := assertionEvidence[assertion]; !exists || entry.Status != "passed" {
				t.Errorf("%s story %s references missing or failed assertion %s", report.Milestone, story.ID, assertion)
			}
		}
	}
	for _, command := range report.Commands {
		if command.Command == "" || command.ExitCode != 0 || localEvidencePath(t, command.Evidence, "artifacts/") == "" {
			t.Errorf("%s completion report contains invalid command evidence", report.Milestone)
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
	acceptance := readSmokeRequirements(t)
	if len(catalog) != len(acceptance) {
		t.Fatal("Smoke catalog must include every acceptance case, including unimplemented fixtures")
	}
	for id, entry := range catalog {
		if acceptance[id].Milestone != entry.Milestone {
			t.Errorf("Smoke %s has wrong or unknown milestone %s", id, entry.Milestone)
		}
		if entry.Test != "" {
			if _, err := os.Stat(filepath.Join("../..", entry.Fixture)); err != nil {
				t.Errorf("registered Smoke %s fixture: %v", id, err)
			}
		}
	}
}

// TestEveryOpenAPIOperationIsOwnedByAWorkItem asserts that every operation declared
// in contracts/openapi.yaml is referenced by at least one work item's operationIds.
// This prevents the coverage gap where an operation ships as an unimplemented 501
// stub because the work-item queue never dispatched it.
func TestEveryOpenAPIOperationIsOwnedByAWorkItem(t *testing.T) {
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
	owned := make(map[string]bool)
	for _, item := range readAgentQueue(t).Items {
		for _, operation := range item.OperationIDs {
			owned[operation] = true
		}
	}
	for operation := range operations {
		if !owned[operation] {
			t.Errorf("operation %s is declared in openapi.yaml but not owned by any work item", operation)
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
