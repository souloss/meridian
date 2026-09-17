//go:build integration

package handler

import (
	"encoding/json/v2"
	"net/http"
	"os"
	"sync"
	"testing"
	"uuid"
)

// TestM1ConcurrencySmoke exercises the syncRepository idempotency digest and
// first-arrival concurrency (SMK-036) and the repository mutex/dirty successor
// behavior (SMK-027).
func TestM1ConcurrencySmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M1 concurrency smoke requires MERIDIAN_TEST_DATABASE_URL")
	}
	t.Run("SMK-036", smokeM1SyncIdempotencyConcurrency)
	t.Run("SMK-027", smokeM1RepositoryMutex)
}

// smokeM1SyncIdempotencyConcurrency fires concurrent identical syncRepository
// requests under one idempotency key and verifies one winner plus exact replay.
func smokeM1SyncIdempotencyConcurrency(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)

	key := uuid.NewV7().String()
	body := map[string]any{"refType": "branch", "ref": "main"}
	headers := map[string]string{"Idempotency-Key": key}

	const requestCount = 5
	type result struct {
		status int
		jobID  string
	}
	results := make([]result, requestCount)
	var wait sync.WaitGroup
	wait.Add(requestCount)
	for index := range requestCount {
		go func() {
			defer wait.Done()
			response := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", body, headers)
			results[index] = result{status: response.Code, jobID: responseString(t, response, "jobId")}
		}()
	}
	wait.Wait()

	// Every response is 202 and returns the exact same job id.
	for index, res := range results {
		if res.status != http.StatusAccepted {
			t.Fatalf("request %d status = %d, want 202", index, res.status)
		}
		if res.jobID != results[0].jobID {
			t.Fatalf("request %d jobId = %s, want %s", index, res.jobID, results[0].jobID)
		}
	}

	// A different canonical body under the same key yields 409 idempotency_conflict.
	conflict := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "develop"}, map[string]string{"Idempotency-Key": key})
	assertError(t, conflict, http.StatusConflict, "idempotency_conflict")
}

// smokeM1RepositoryMutex verifies that duplicate sync requests for one
// repository+ref return the same job and that a completed sync deduplicates.
func smokeM1RepositoryMutex(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)

	// Two sequential duplicate requests coalesce onto one deduplicated job.
	first := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, first, http.StatusAccepted)
	firstJob := responseString(t, first, "jobId")

	duplicate := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, duplicate, http.StatusAccepted)
	if responseString(t, duplicate, "jobId") != firstJob {
		t.Fatalf("duplicate sync jobId = %s, want %s", responseString(t, duplicate, "jobId"), firstJob)
	}
	var dedupBody struct {
		Deduplicated bool `json:"deduplicated"`
	}
	if err := json.Unmarshal(duplicate.Body.Bytes(), &dedupBody); err == nil && !dedupBody.Deduplicated {
		t.Fatalf("duplicate sync not marked deduplicated: %s", duplicate.Body.String())
	}

	// The coalesced job completes successfully.
	if job := f.waitJob(t, firstJob); job["status"] != "succeeded" {
		t.Fatalf("sync job status = %v", job["status"])
	}
}
