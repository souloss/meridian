package task

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type recordingExecutionStore struct {
	started  []StartInput
	stages   []StageInput
	finished []FinishInput
}

func (store *recordingExecutionStore) StartJob(_ context.Context, input StartInput) (ClaimResult, error) {
	store.started = append(store.started, input)
	return ClaimResult{Claimed: true, Attempt: input.ExpectedAttempt}, nil
}

func (store *recordingExecutionStore) SetJobStage(_ context.Context, input StageInput) error {
	store.stages = append(store.stages, input)
	return nil
}

func (store *recordingExecutionStore) FinishJob(_ context.Context, input FinishInput) error {
	store.finished = append(store.finished, input)
	return nil
}

type recordingSyncRunner struct {
	err error
}

func (runner recordingSyncRunner) Run(context.Context, CredentialSyncArgs) error { return runner.err }

func TestCredentialSyncWorkerRecordsAllPipelineStages(t *testing.T) {
	store := &recordingExecutionStore{}
	worker := NewCredentialSyncWorker(store, recordingSyncRunner{})
	worker.now = func() time.Time { return time.Date(2026, 9, 5, 1, 2, 3, 0, time.UTC) }
	args := CredentialSyncArgs{TenantID: uuid.NewV7(), JobID: uuid.NewV7(), RepositoryID: uuid.NewV7(), RefName: "main"}
	job := &river.Job[CredentialSyncArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: args}

	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("worker execution: %v", err)
	}
	if len(store.started) != 1 || store.started[0].Stage != StageResolve {
		t.Fatalf("started events = %#v, want one resolve event", store.started)
	}
	wantStages := []Stage{StageDiscover, StageExtract, StageMerge, StageNormalize, StageIndex}
	if len(store.stages) != len(wantStages) {
		t.Fatalf("stage events = %#v, want %d events", store.stages, len(wantStages))
	}
	for index, want := range wantStages {
		if store.stages[index].Stage != want || store.stages[index].Level != "info" {
			t.Fatalf("stage event %d = %#v, want stage %q", index, store.stages[index], want)
		}
	}
	if len(store.finished) != 1 || store.finished[0].Status != "succeeded" || !store.finished[0].Terminal {
		t.Fatalf("finished events = %#v, want terminal success", store.finished)
	}
	if string(store.finished[0].Result) == "" || store.finished[0].Error != nil {
		t.Fatalf("finished result/error = %s/%s, want result and no error", store.finished[0].Result, store.finished[0].Error)
	}
}

func TestCredentialSyncWorkerRecordsUnsupportedProducerAsTerminalFailure(t *testing.T) {
	store := &recordingExecutionStore{}
	worker := NewCredentialSyncWorker(store, recordingSyncRunner{err: ErrRepositorySyncUnavailable})
	args := CredentialSyncArgs{TenantID: uuid.NewV7(), JobID: uuid.NewV7(), RepositoryID: uuid.NewV7(), RefName: "main"}
	job := &river.Job[CredentialSyncArgs]{JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3}, Args: args}

	if err := worker.Work(t.Context(), job); err != nil {
		t.Fatalf("worker failure handling: %v", err)
	}
	if len(store.stages) != 0 {
		t.Fatalf("stage events = %#v, want no post-resolve stages", store.stages)
	}
	if len(store.finished) != 1 || store.finished[0].Status != "failed" || !store.finished[0].Terminal || store.finished[0].ExpectedAttempt != 1 {
		t.Fatalf("finished events = %#v, want terminal failure", store.finished)
	}
	if store.finished[0].RepositoryID != args.RepositoryID || store.finished[0].ErrorCode != "internal_error" {
		t.Fatalf("failure repository/code = %s/%q, want %s/internal_error", store.finished[0].RepositoryID, store.finished[0].ErrorCode, args.RepositoryID)
	}
	if string(store.finished[0].Error) != `{"code":"internal_error"}` {
		t.Fatalf("failure payload = %s, want redacted stable payload", store.finished[0].Error)
	}
}
