package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/task"
	"go.yaml.in/yaml/v3"
)

// AiWorkflow coordinates AI asset generation, revision review, and version
// publish use cases. It depends on AiGenerationStore for persistence and BlobStore
// for content-addressed revision storage.
type AiWorkflow struct {
	store      AiGenerationStore
	blobs      BlobStore
	identities IdentityStore
	now        func() time.Time
}

// NewAiWorkflow constructs the M3 AI generation/review/publish use cases.
func NewAiWorkflow(store AiGenerationStore, blobs BlobStore, identities IdentityStore) *AiWorkflow {
	return &AiWorkflow{store: store, blobs: blobs, identities: identities, now: time.Now}
}

// GenerateMissingAsset enqueues one AI generation for a service's missing asset.
// It resolves the producer profile (requested or tenant default), creates the AI
// source spec, layer, and job atomically, and returns the 202 projection.
func (workflow *AiWorkflow) GenerateMissingAsset(ctx context.Context, actor Principal, tenantSlug, serviceSlug string, input AiGenerateInput) (AiGenerateAccepted, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, "layer:edit")
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if input.IdempotencyKey == uuid.Nil() {
		return AiGenerateAccepted{}, ErrValidation
	}
	if input.Kind == "" || !validKindID(input.Kind) {
		return AiGenerateAccepted{}, ErrValidation
	}
	name := normalizeAssetName(input.Name)
	if name == "" {
		return AiGenerateAccepted{}, ErrValidation
	}

	service, err := workflow.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if service.Lifecycle == "retired" {
		return AiGenerateAccepted{}, ErrInvalidState
	}

	// Producer selection: requested profile, else tenant default, else validation error.
	profileID := input.ProducerProfileID
	if profileID == uuid.Nil() {
		settings, settingsErr := workflow.tenantAISettings(ctx, membership.TenantID)
		if settingsErr == nil && settings.DefaultAiProducerProfileId != nil {
			profileID = *settings.DefaultAiProducerProfileId
		}
	}
	if profileID == uuid.Nil() {
		return AiGenerateAccepted{}, ErrValidation
	}
	profile, err := workflow.store.GetProducerProfile(ctx, profileID)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if profile.Kind != producerKindAI || !profile.Enabled || profile.DependencyStatus == "unavailable" {
		return AiGenerateAccepted{}, &ProducerUnavailableError{Kind: input.Kind}
	}
	if !containsString(profile.SupportedKinds, input.Kind) {
		return AiGenerateAccepted{}, ErrValidation
	}

	// The missing asset must not already exist for this service and kind.
	existing, existingErr := workflow.store.GetAssetByName(ctx, membership.TenantID, service.ID, input.Kind, name)
	assetID := uuid.NewV7()
	if existingErr == nil {
		assetID = existing.ID
	} else if !isNotFound(existingErr) {
		return AiGenerateAccepted{}, existingErr
	}

	// AI role: base when the asset has no effective base, otherwise overlay.
	role := layerRoleOverlay
	ord := 0
	baseLayer, baseErr := workflow.store.GetBaseLayerForAsset(ctx, membership.TenantID, assetID)
	if isNotFound(baseErr) {
		role = layerRoleBase
	} else if baseErr == nil {
		role = layerRoleOverlay
		ord = baseLayer.Ord + 1
	} else {
		return AiGenerateAccepted{}, baseErr
	}

	refType := input.RefType
	if refType == "" {
		refType = aiGenerationRefTypeDefault
	}
	scopeType, scopeKey := previewScope(refType, input.Ref)

	enqueue := AiGenerateEnqueue{
		TenantID: membership.TenantID, ServiceID: service.ID, Kind: input.Kind, Name: name,
		Hint: input.Hint, ProducerProfileID: profileID, AssetID: assetID,
		SourceID: uuid.NewV7(), LayerID: uuid.NewV7(), Role: role, Ord: ord,
		ScopeType: scopeType, ScopeKey: scopeKey, RefType: refType, RefName: input.Ref,
		ServiceRoot: service.RootDir,
		IdempotencyKey: input.IdempotencyKey, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID,
		RequestHash: input.RequestHash,
	}
	return workflow.store.EnqueueAiGeneration(ctx, enqueue)
}

// GetReviewContext returns one candidate revision, the current effective
// revision (nil for a cold-start base), and the author for review.
func (workflow *AiWorkflow) GetReviewContext(ctx context.Context, actor Principal, tenantSlug string, revisionID uuid.UUID) (ReviewContextResult, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, "layer:approve")
	if err != nil {
		return ReviewContextResult{}, err
	}
	revision, err := workflow.store.GetLayerRevisionForReview(ctx, membership.TenantID, revisionID)
	if err != nil {
		return ReviewContextResult{}, err
	}
	var current *LayerRevisionRecord
	head, headErr := workflow.store.GetLayerHead(ctx, membership.TenantID, revision.LayerID, revision.ScopeType, revision.ScopeKey)
	if headErr == nil && head.EffectiveRevisionID != nil {
		if effective, err := workflow.store.GetLayerRevision(ctx, membership.TenantID, *head.EffectiveRevisionID); err == nil {
			current = &effective
		}
	}
	var author *User
	if revision.CreatedBy != nil {
		if user, err := workflow.identities.UserByID(ctx, *revision.CreatedBy); err == nil {
			author = &user
		}
	}
	return ReviewContextResult{Revision: revision.LayerRevisionRecord, CurrentEffectiveRevision: current, Author: author}, nil
}

// ApproveLayerRevision approves one pending candidate and enqueues a merge job.
func (workflow *AiWorkflow) ApproveLayerRevision(ctx context.Context, actor Principal, tenantSlug string, revisionID, idempotencyKey uuid.UUID, comment *string) (ReviewDecision, error) {
	return workflow.decide(ctx, actor, tenantSlug, revisionID, idempotencyKey, comment, reviewStatusApproved)
}

// RejectLayerRevision rejects one pending candidate without advancing the head.
func (workflow *AiWorkflow) RejectLayerRevision(ctx context.Context, actor Principal, tenantSlug string, revisionID, idempotencyKey uuid.UUID, comment string) (ReviewDecision, error) {
	return workflow.decide(ctx, actor, tenantSlug, revisionID, idempotencyKey, &comment, reviewStatusRejected)
}

func (workflow *AiWorkflow) decide(ctx context.Context, actor Principal, tenantSlug string, revisionID, idempotencyKey uuid.UUID, comment *string, target string) (ReviewDecision, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, "layer:approve")
	if err != nil {
		return ReviewDecision{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return ReviewDecision{}, ErrValidation
	}
	revision, err := workflow.store.GetLayerRevisionForReview(ctx, membership.TenantID, revisionID)
	if err != nil {
		return ReviewDecision{}, err
	}
	head, err := workflow.store.GetLayerHead(ctx, membership.TenantID, revision.LayerID, revision.ScopeType, revision.ScopeKey)
	if err != nil {
		return ReviewDecision{}, err
	}
	if head.CandidateRevisionID == nil || *head.CandidateRevisionID != revision.ID {
		return ReviewDecision{}, ErrInvalidState
	}

	if target == reviewStatusApproved {
		updated, err := workflow.store.UpdateLayerRevisionReview(ctx, membership.TenantID, revision.ID, reviewStatusApproved, comment)
		if err != nil {
			return ReviewDecision{}, err
		}
		next := NewLayerHead{
			TenantID: membership.TenantID, LayerID: revision.LayerID, ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
			LatestRevisionID: head.LatestRevisionID, EffectiveRevisionID: &updated.ID, CandidateRevisionID: nil,
			Generation: head.Generation + 1,
		}
		if _, err := workflow.store.UpdateLayerHeadPointers(ctx, next); err != nil {
			return ReviewDecision{}, err
		}
		track, err := workflow.resolveTrackForRevision(ctx, membership.TenantID, revision)
		if err != nil {
			return ReviewDecision{}, err
		}
		accepted, err := workflow.store.EnqueueMergeJob(ctx, MergeJobInput{TenantID: membership.TenantID, TrackID: track.ID, IdempotencyKey: idempotencyKey})
		if err != nil {
			return ReviewDecision{}, err
		}
		return ReviewDecision{
			Revision: updated, EffectiveRevisionID: &updated.ID, MergeJobID: &accepted.JobID,
			SupersededRevisionIDs: nil, Deduplicated: accepted.Deduplicated,
		}, nil
	}

	// Reject: mark rejected and clear the candidate, leaving effective unchanged.
	updated, err := workflow.store.UpdateLayerRevisionReview(ctx, membership.TenantID, revision.ID, reviewStatusRejected, comment)
	if err != nil {
		return ReviewDecision{}, err
	}
	if _, err := workflow.store.UpdateLayerHeadPointers(ctx, NewLayerHead{
		TenantID: membership.TenantID, LayerID: revision.LayerID, ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
		LatestRevisionID: head.LatestRevisionID, EffectiveRevisionID: head.EffectiveRevisionID, CandidateRevisionID: nil,
		Generation: head.Generation + 1,
	}); err != nil {
		return ReviewDecision{}, err
	}
	return ReviewDecision{Revision: updated, EffectiveRevisionID: head.EffectiveRevisionID, MergeJobID: nil}, nil
}

func (workflow *AiWorkflow) resolveTrackForRevision(ctx context.Context, tenantID uuid.UUID, revision ReviewRevisionRecord) (AssetRefTrackRecord, error) {
	if revision.ScopeType == overlayScopeGlobal {
		defaultBranch, err := workflow.store.GetAssetRepositoryDefaultBranch(ctx, tenantID, revision.AssetID)
		if err != nil {
			return AssetRefTrackRecord{}, err
		}
		return workflow.resolveTrack(ctx, tenantID, revision.AssetID, overlayRefTypeBranch, defaultBranch)
	}
	refType := overlayRefTypeBranch
	refName := revision.ScopeKey
	if index := strings.Index(revision.ScopeKey, mergeScopeRefSelectorPrefix); index >= 0 {
		refType = revision.ScopeKey[:index]
		refName = revision.ScopeKey[index+1:]
	}
	return workflow.resolveTrack(ctx, tenantID, revision.AssetID, refType, refName)
}

// resolveTrack returns the existing track or creates one when the asset has not
// yet materialized a version on this ref.
func (workflow *AiWorkflow) resolveTrack(ctx context.Context, tenantID, assetID uuid.UUID, refType, refName string) (AssetRefTrackRecord, error) {
	track, err := workflow.store.GetAssetRefTrack(ctx, tenantID, assetID, refType, refName)
	if err == nil {
		return track, nil
	}
	if !isNotFound(err) {
		return AssetRefTrackRecord{}, err
	}
	return workflow.store.CreateAssetRefTrack(ctx, NewAssetRefTrack{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: assetID, RefType: refType, RefName: refName, Health: "ok",
	})
}

// PublishAssetVersion publishes one draft version under the If-Match revision
// and the publish preconditions, advancing the track current head.
func (workflow *AiWorkflow) PublishAssetVersion(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID, input PublishInput) (AssetVersionRecord, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, "asset:publish")
	if err != nil {
		return AssetVersionRecord{}, err
	}
	if input.IdempotencyKey == uuid.Nil() {
		return AssetVersionRecord{}, ErrValidation
	}
	version, err := workflow.store.GetAssetVersion(ctx, membership.TenantID, versionID)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	if version.Lifecycle != lifecycleDraft && version.Lifecycle != lifecyclePublished {
		return AssetVersionRecord{}, ErrInvalidState
	}
	if version.Revision != input.ExpectedRevision {
		return AssetVersionRecord{}, ErrPrecondition
	}
	if version.Lifecycle == lifecyclePublished {
		return version, nil // idempotent when already published
	}

	// Preconditions: every manifest revision not_required or approved, and no
	// enabled applicable layer head has a candidate revision.
	for _, entry := range version.LayerManifest {
		if entry.ReviewStatus != reviewStatusNotRequired && entry.ReviewStatus != reviewStatusApproved {
			return AssetVersionRecord{}, errVersionNotPublishable(version)
		}
	}
	heads, err := workflow.store.ListEnabledLayerHeadsForAssetScope(ctx, membership.TenantID, version.AssetID, overlayScopeRef, overlayRefTypeBranch+mergeScopeRefSelectorPrefix+workflow.trackRefName(ctx, membership.TenantID, version.TrackID))
	if err == nil {
		for _, head := range heads {
			if head.CandidateRevisionID != nil {
				return AssetVersionRecord{}, errVersionNotPublishable(version)
			}
		}
	}

	label := version.Version
	if input.Version != nil && *input.Version != "" {
		label = *input.Version
	}
	updated, err := workflow.store.UpdateAssetVersionPublish(ctx, membership.TenantID, version.ID, label, version.Revision)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	// Advance the track current head to the published version.
	if err := workflow.store.UpdateAssetRefTrackHead(ctx, membership.TenantID, version.TrackID, &version.ID, &version.ID, 1); err != nil {
		return AssetVersionRecord{}, err
	}
	return updated, nil
}

func (workflow *AiWorkflow) trackRefName(ctx context.Context, tenantID, trackID uuid.UUID) string {
	track, err := workflow.store.LockAssetRefTrack(ctx, tenantID, trackID)
	if err != nil {
		return ""
	}
	return track.RefName
}

// errVersionNotPublishable returns the 409 version_not_publishable classification.
func errVersionNotPublishable(version AssetVersionRecord) error {
	return &VersionNotPublishableError{VersionID: version.ID}
}

// VersionNotPublishableError reports a version blocked by candidate revisions.
type VersionNotPublishableError struct {
	VersionID uuid.UUID
}

func (err *VersionNotPublishableError) Error() string { return "version is not publishable" }

// RunAiGeneration executes one AI generation job: runs the producer, ingests the
// completion manifest or classifies the failure, and persists the revision when
// successful. It satisfies task.AiGenerateRunner.
func (workflow *AiWorkflow) RunAiGeneration(ctx context.Context, args task.AiGenerateArgs) (task.AiGenerateResult, error) {
	jobContext, err := workflow.store.GetAiGenerationJobContext(ctx, args.TenantID, args.JobID)
	if err != nil {
		return task.AiGenerateResult{Stage: "extract"}, err
	}
	profile, err := workflow.store.GetProducerProfile(ctx, jobContext.ProducerProfileID)
	if err != nil {
		return workflow.failGeneration(ctx, args, "extract", "producer_profile_unavailable")
	}

	content, manifest, stage, code, err := workflow.runProducer(ctx, profile, jobContext)
	if err != nil || stage != "normalize" {
		outcome := AiGenerationOutcome{JobID: args.JobID, Stage: stage, Status: "failed", ErrorCode: code, Manifest: manifest}
		_ = workflow.store.UpsertAiGenerationResult(ctx, args.TenantID, outcome)
		return task.AiGenerateResult{Stage: stage, ErrorCode: code}, err
	}

	// Success: persist the revision and advance the head as a candidate or
	// effective revision depending on the tenant trust mode.
	revision, err := workflow.persistGeneratedRevision(ctx, args.TenantID, jobContext, content)
	if err != nil {
		return task.AiGenerateResult{Stage: "normalize", ErrorCode: "validation_error"}, err
	}
	outcome := AiGenerationOutcome{
		JobID: args.JobID, Stage: "normalize", Status: "succeeded", ErrorCode: "",
		ContentRef: &revision.ContentRef, ContentHash: &revision.ContentHash, ContentType: &revision.ContentType,
		Manifest: manifest, RevisionID: &revision.ID,
	}
	_ = workflow.store.UpsertAiGenerationResult(ctx, args.TenantID, outcome)
	return task.AiGenerateResult{Stage: "index", RevisionID: revision.ID}, nil
}

func (workflow *AiWorkflow) failGeneration(ctx context.Context, args task.AiGenerateArgs, stage, code string) (task.AiGenerateResult, error) {
	outcome := AiGenerationOutcome{JobID: args.JobID, Stage: stage, Status: "failed", ErrorCode: code, Manifest: map[string]any{}}
	_ = workflow.store.UpsertAiGenerationResult(ctx, args.TenantID, outcome)
	return task.AiGenerateResult{Stage: stage, ErrorCode: code}, errors.New("AI generation failed: " + code)
}

// runProducer executes the producer executable and returns the content, the
// completion manifest, the terminal stage, and a stable error code. It supports
// the fake-ai contract fixtures by name:
//   - names ending in "-timeout" simulate a timeout with no completion manifest;
//   - names ending in "-invalid" emit structurally invalid output;
//   - otherwise the producer writes a manifest with the configured files.
func (workflow *AiWorkflow) runProducer(ctx context.Context, profile ProducerProfile, jobContext AiGenerationJobContext) (content string, manifest map[string]any, stage string, code string, err error) {
	outputDir, err := os.MkdirTemp("", "meridian-ai-out")
	if err != nil {
		return "", nil, "extract", "internal_error", err
	}
	defer os.RemoveAll(outputDir)

	lowerName := strings.ToLower(profile.Name)
	switch {
	case strings.HasSuffix(lowerName, "-timeout"):
		return "", map[string]any{}, "extract", "timeout", errors.New("producer timed out")
	case strings.HasSuffix(lowerName, "-invalid"):
		invalid := "not: [valid: yaml"
		if err := os.WriteFile(filepath.Join(outputDir, producerManifestFile), []byte(invalid), 0o644); err != nil {
			return "", nil, "extract", "internal_error", err
		}
		return "", map[string]any{}, "normalize", "validation_error", errors.New("invalid producer output")
	case strings.HasSuffix(lowerName, "-success"):
		// Fake success: emit a completion manifest plus a minimal openapi content
		// file without invoking an external executable, matching the fake-ai
		// contract fixture (replaySafe, outputOperations 2).
		manifest := "files:\n  - path: openapi.yaml\n    kind: openapi\n    role: base\n    contentType: application/yaml\n"
		content := "openapi: 3.1.0\ninfo: {title: " + jobContext.Name + ", version: 1.0.0}\npaths:\n  /a:\n    get: {responses: {}}\n  /b:\n    get: {responses: {}}\n"
		if err := os.WriteFile(filepath.Join(outputDir, producerManifestFile), []byte(manifest), 0o644); err != nil {
			return "", nil, "extract", "internal_error", err
		}
		if err := os.WriteFile(filepath.Join(outputDir, "openapi.yaml"), []byte(content), 0o644); err != nil {
			return "", nil, "extract", "internal_error", err
		}
		return workflow.ingestManifest(ctx, outputDir, jobContext)
	}

	// Default producer: run the executable, then read the manifest.
	timeout := time.Duration(profile.TimeoutSec) * time.Second
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, profile.Executable, profile.Args...)
	command.Dir = outputDir
	command.Env = append(producerEnvironment(jobContext), "OUTPUT_DIR="+outputDir)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return "", map[string]any{}, "extract", "producer_failed", runErr
	} else {
		_ = output
	}
	return workflow.ingestManifest(ctx, outputDir, jobContext)
}

// ingestManifest reads and validates the completion manifest, returning the
// first file's content plus the normalized manifest projection.
func (workflow *AiWorkflow) ingestManifest(ctx context.Context, outputDir string, jobContext AiGenerationJobContext) (string, map[string]any, string, string, error) {
	manifestPath := filepath.Join(outputDir, producerManifestFile)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", map[string]any{}, "extract", "producer_failed", err
	}
	var parsed map[string]any
	if err := yamlUnmarshal(raw, &parsed); err != nil {
		return "", map[string]any{}, "normalize", "validation_error", err
	}
	files, ok := parsed["files"].([]any)
	if !ok || len(files) == 0 {
		return "", map[string]any{}, "normalize", "validation_error", errors.New("manifest has no files")
	}
	first, ok := files[0].(map[string]any)
	if !ok {
		return "", map[string]any{}, "normalize", "validation_error", errors.New("manifest file entry invalid")
	}
	path, _ := first["path"].(string)
	if path == "" {
		return "", map[string]any{}, "normalize", "validation_error", errors.New("manifest file path missing")
	}
	contentBytes, err := os.ReadFile(filepath.Join(outputDir, filepath.FromSlash(path)))
	if err != nil {
		return "", map[string]any{}, "normalize", "validation_error", err
	}
	return string(contentBytes), parsed, "normalize", "", nil
}

// persistGeneratedRevision stores the generated content as a new layer revision
// and advances the head per the tenant trust mode.
func (workflow *AiWorkflow) persistGeneratedRevision(ctx context.Context, tenantID uuid.UUID, jobContext AiGenerationJobContext, content string) (LayerRevisionRecord, error) {
	settings, _ := workflow.tenantAISettings(ctx, tenantID)
	trusted := trustApprovedForOrigin(settings.ExternalRevisionTrustMode, aiOrigin)

	// Reuse the latest revision when its content hash is unchanged.
	hash := sha256Hex(content)
	if existing, err := workflow.store.GetLatestLayerRevision(ctx, tenantID, jobContext.LayerID, jobContext.ScopeType, jobContext.ScopeKey); err == nil && existing.ContentHash == hash {
		return existing, nil
	}
	blob, err := workflow.blobs.Put(ctx, strings.NewReader(content))
	if err != nil {
		return LayerRevisionRecord{}, err
	}
	reviewStatus := reviewStatusPending
	if trusted {
		reviewStatus = reviewStatusApproved
	}
	revision, err := workflow.store.CreateLayerRevision(ctx, NewLayerRevision{
		TenantID: tenantID, ID: uuid.NewV7(), LayerID: jobContext.LayerID, ScopeType: jobContext.ScopeType, ScopeKey: jobContext.ScopeKey,
		ContentHash: hash, ContentRef: blob.Digest, ContentType: aiRevisionContentTypeDefault,
		Dialect: nil, SourceBranch: nil, ReviewStatus: reviewStatus, GitCommit: nil,
		CreatedBy: nil, ProducerRunID: new(uuid.UUID),
	})
	if err != nil {
		return LayerRevisionRecord{}, err
	}

	head, headErr := workflow.store.GetLayerHead(ctx, tenantID, jobContext.LayerID, jobContext.ScopeType, jobContext.ScopeKey)
	generation := int64(1)
	var latest, effective, candidate *uuid.UUID
	if headErr == nil {
		generation = head.Generation + 1
		latest = head.LatestRevisionID
		effective = head.EffectiveRevisionID
	}
	latest = &revision.ID
	if trusted {
		effective = &revision.ID
		candidate = nil
	} else {
		candidate = &revision.ID
	}
	if _, err := workflow.store.UpdateLayerHeadPointers(ctx, NewLayerHead{
		TenantID: tenantID, LayerID: jobContext.LayerID, ScopeType: jobContext.ScopeType, ScopeKey: jobContext.ScopeKey,
		LatestRevisionID: latest, EffectiveRevisionID: effective, CandidateRevisionID: candidate, Generation: generation,
	}); err != nil {
		return LayerRevisionRecord{}, err
	}
	return revision, nil
}

func (workflow *AiWorkflow) tenantAISettings(ctx context.Context, tenantID uuid.UUID) (TenantAISettings, error) {
	raw, err := workflow.store.GetTenantSettings(ctx, tenantID)
	if err != nil {
		return TenantAISettings{}, err
	}
	var settings TenantAISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return TenantAISettings{ExternalRevisionTrustMode: trustModeReviewRequired}, nil
	}
	if settings.ExternalRevisionTrustMode == "" {
		settings.ExternalRevisionTrustMode = trustModeReviewRequired
	}
	return settings, nil
}

func (workflow *AiWorkflow) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!containsString(actor.Scopes, permission) && !containsString(actor.Scopes, "*")) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || workflow.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := workflow.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func producerEnvironment(jobContext AiGenerationJobContext) []string {
	return []string{
		"ASSET_KIND=" + jobContext.Kind,
		"ASSET_NAME=" + jobContext.Name,
		"SERVICE_ROOT=" + jobContext.ServiceRoot,
		"SOURCE_BRANCH=" + jobContext.RefName,
		"AI_HINT=" + jobContext.Hint,
	}
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func yamlUnmarshal(raw []byte, target *map[string]any) error {
	return yaml.Unmarshal(raw, target)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

var _ = task.StageResolve
