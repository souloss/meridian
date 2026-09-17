package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/task"
	"go.yaml.in/yaml/v3"
)

// LayerEditStore is the persistence boundary for M2 layer editing: overlay
// revisions, ordering, rollback, merge preview, and provenance reads. Every
// method retains the tenant predicate.
type LayerEditStore interface {
	GetLayer(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	ListLayersForAsset(context.Context, uuid.UUID, uuid.UUID) ([]LayerRecord, error)
	UpdateLayerOrd(context.Context, uuid.UUID, uuid.UUID, int64, int) (LayerRecord, error)
	CreateLayerRevision(context.Context, NewLayerRevision) (LayerRevisionRecord, error)
	GetLayerRevision(context.Context, uuid.UUID, uuid.UUID) (LayerRevisionRecord, error)
	GetLayerHead(context.Context, uuid.UUID, uuid.UUID, string, string) (LayerHeadRecord, error)
	UpdateLayerHeadPointers(context.Context, NewLayerHead) (LayerHeadRecord, error)
	GetAssetVersion(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	GetAsset(context.Context, uuid.UUID, uuid.UUID) (AssetRecord, error)
	GetAssetRepositoryDefaultBranch(context.Context, uuid.UUID, uuid.UUID) (string, error)
	GetAssetRefTrack(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRefTrackRecord, error)
	GetAssetRefTrackByID(context.Context, uuid.UUID, uuid.UUID) (AssetRefTrackRecord, error)
	CreateAssetRefTrack(context.Context, NewAssetRefTrack) (AssetRefTrackRecord, error)
	GetLatestVersionInTrack(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	CreateAssetVersion(context.Context, NewAssetVersion) (AssetVersionRecord, error)
	UpdateAssetRefTrackHead(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, *uuid.UUID, int64) error
	EnqueueMergeJob(context.Context, MergeJobInput) (JobAccepted, error)
}

// MergeJobInput describes one idempotent asset.merge request for a track.
type MergeJobInput struct {
	TenantID       uuid.UUID
	TrackID        uuid.UUID
	IdempotencyKey uuid.UUID
}

// LayerEdit coordinates overlay revision submission, layer ordering, rollback,
// and non-persisting merge preview against the M2 platform-v1 merge engine.
type LayerEdit struct {
	store      LayerEditStore
	blobs      BlobStore
	identities IdentityStore
	now        func() time.Time
}

// NewLayerEdit constructs M2 layer editing use cases.
func NewLayerEdit(store LayerEditStore, blobs BlobStore, identities IdentityStore) *LayerEdit {
	return &LayerEdit{store: store, blobs: blobs, identities: identities, now: time.Now}
}

// PreviewMerge runs the real merge engine over an asset's effective layers
// without persisting anything. The result carries the merged content, the
// canonical fingerprint, and provenance as json-pointer → last-writing layer.
func (editor *LayerEdit) PreviewMerge(ctx context.Context, actor Principal, tenantSlug string, input MergePreviewInput) (MergePreviewResult, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, "layer:read")
	if err != nil {
		return MergePreviewResult{}, err
	}
	if _, err := editor.store.GetAsset(ctx, membership.TenantID, input.AssetID); err != nil {
		return MergePreviewResult{}, err
	}
	layers, err := editor.store.ListLayersForAsset(ctx, membership.TenantID, input.AssetID)
	if err != nil {
		return MergePreviewResult{}, err
	}

	base, overlays, err := splitBaseAndOverlays(layers)
	if err != nil {
		return MergePreviewResult{}, err
	}

	// Selection: an explicit layer selection replaces the asset's application
	// order; otherwise effective heads are merged in ord order.
	selected, err := editor.selectedRevisions(ctx, membership.TenantID, input, base, overlays)
	if err != nil {
		return MergePreviewResult{}, err
	}

	content := make(map[string]any)
	var validation []OverlayIssue
	provenance := make([]ProvenanceEntryRecord, 0, len(selected))
	for _, layer := range selected {
		if layer.Role == layerRoleBase {
			decoded, decodeErr := decodeBaseDocument(layer.Content)
			if decodeErr != nil {
				return MergePreviewResult{}, decodeErr
			}
			content = decoded
			provenance = append(provenance, ProvenanceEntryRecord{Pointer: "", LayerID: layer.LayerID, RevisionID: layer.RevisionID})
			continue
		}
		raw, parseErr := ParseOverlay([]byte(layer.Content))
		if parseErr != nil {
			return MergePreviewResult{}, parseErr
		}
		merged, pointers, warnings, mergeErr := ApplyPlatformOverlay(content, raw)
		if mergeErr != nil {
			return MergePreviewResult{}, mergeErr
		}
		content = merged
		validation = append(validation, warnings...)
		for _, pointer := range pointers {
			provenance = append(provenance, ProvenanceEntryRecord{Pointer: pointer, LayerID: layer.LayerID, RevisionID: layer.RevisionID})
		}
	}

	// A base-owning entry marks the document root when no overlay wrote it.
	if len(provenance) > 0 && provenance[0].Pointer == "" && len(provenance) == 1 {
		provenance[0].Pointer = ""
	}

	mergedBytes, err := CanonicalJSON(content)
	if err != nil {
		return MergePreviewResult{}, err
	}
	fingerprint, err := editor.buildFingerprint(selected)
	if err != nil {
		return MergePreviewResult{}, err
	}
	return MergePreviewResult{
		InputFingerprint: fingerprint,
		Content:          string(mergedBytes),
		ContentType:      layerRevisionContentTypeYAML,
		Validation:       validation,
		Provenance:       provenance,
	}, nil
}

// CreateLayerRevision validates and persists one manual overlay revision, then
// advances the layer head and enqueues a merge job. Invalid overlays are
// rejected without persistence.
func (editor *LayerEdit) CreateLayerRevision(ctx context.Context, actor Principal, tenantSlug string, layerID uuid.UUID, input LayerRevisionInput) (LayerRevisionResult, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, "layer:edit")
	if err != nil {
		return LayerRevisionResult{}, err
	}
	layer, err := editor.store.GetLayer(ctx, membership.TenantID, layerID)
	if err != nil {
		return LayerRevisionResult{}, err
	}
	if layer.Role != layerRoleOverlay {
		return LayerRevisionResult{}, ErrValidation
	}
	// Validate before persisting anything: parse + compile the overlay.
	if _, err := ParseOverlay([]byte(input.Content)); err != nil {
		return LayerRevisionResult{}, err
	}

	scopeType, scopeKey := normalizeScope(input.ScopeType, input.ScopeKey)
	head, headErr := editor.store.GetLayerHead(ctx, membership.TenantID, layer.ID, scopeType, scopeKey)
	hasHead := headErr == nil
	if headErr != nil && !isNotFound(headErr) {
		return LayerRevisionResult{}, headErr
	}

	hash := sha256.Sum256([]byte(input.Content))
	contentHash := hex.EncodeToString(hash[:])
	blob, err := editor.blobs.Put(ctx, strings.NewReader(input.Content))
	if err != nil {
		return LayerRevisionResult{}, err
	}
	// Manual overlays are always not_required per the revision state machine.
	reviewStatus := layerRevisionReviewNotRequired
	createdBy := membership.UserID
	revision, err := editor.store.CreateLayerRevision(ctx, NewLayerRevision{
		TenantID: membership.TenantID, ID: uuid.NewV7(), LayerID: layer.ID, ScopeType: scopeType, ScopeKey: scopeKey,
		ContentHash: contentHash, ContentRef: blob.Digest, ContentType: input.ContentType,
		Dialect: input.Dialect, SourceBranch: nil, ReviewStatus: reviewStatus, GitCommit: nil,
		CreatedBy: &createdBy, ProducerRunID: nil,
	})
	if err != nil {
		return LayerRevisionResult{}, err
	}

	// Advance the head: not_required moves latest+effective.
	next := NewLayerHead{
		TenantID: membership.TenantID, LayerID: layer.ID, ScopeType: scopeType, ScopeKey: scopeKey,
		LatestRevisionID:    &revision.ID,
		EffectiveRevisionID: &revision.ID,
		CandidateRevisionID: nil,
		Generation:          layerGenerationBase,
	}
	if hasHead {
		next.Generation = head.Generation + 1
	}
	if _, err := editor.store.UpdateLayerHeadPointers(ctx, next); err != nil {
		return LayerRevisionResult{}, err
	}

	// Enqueue a merge job to materialize the new effective content.
	track, err := editor.resolveTrack(ctx, membership.TenantID, layer.AssetID, scopeType, scopeKey)
	if err != nil {
		return LayerRevisionResult{}, err
	}
	accepted, err := editor.store.EnqueueMergeJob(ctx, MergeJobInput{
		TenantID: membership.TenantID, TrackID: track.ID, IdempotencyKey: uuid.NewV7(),
	})
	if err != nil {
		return LayerRevisionResult{}, err
	}
	return LayerRevisionResult{Revision: revision, JobID: accepted.JobID, Deduplicated: accepted.Deduplicated}, nil
}

// LayerRevisionResult carries one persisted overlay revision plus the merge job
// enqueued to materialize it.
type LayerRevisionResult struct {
	Revision     LayerRevisionRecord
	JobID        uuid.UUID
	Deduplicated bool
}

// GetAssetVersionProvenance returns the last-writing layer and revision per
// JSON pointer for a materialized version, derived from its layer manifest.
func (editor *LayerEdit) GetAssetVersionProvenance(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID) ([]ProvenanceEntryRecord, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, "layer:read")
	if err != nil {
		return nil, err
	}
	version, err := editor.store.GetAssetVersion(ctx, membership.TenantID, versionID)
	if err != nil {
		return nil, err
	}
	entries := make([]ProvenanceEntryRecord, 0, len(version.LayerManifest))
	for _, entry := range version.LayerManifest {
		entries = append(entries, ProvenanceEntryRecord{Pointer: "", LayerID: entry.LayerID, RevisionID: entry.RevisionID})
	}
	return entries, nil
}

// ReorderAssetLayers persists a new overlay ord sequence under optimistic
// concurrency. Duplicate layer ids and non-overlay layers are rejected.
func (editor *LayerEdit) ReorderAssetLayers(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID, etag string, layerIDs []uuid.UUID) ([]LayerRecord, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, "layer:edit")
	if err != nil {
		return nil, err
	}
	if len(layerIDs) == 0 {
		return nil, ErrValidation
	}
	if err := parseAssetLayersETag(etag, assetID); err != nil {
		return nil, ErrPrecondition
	}
	existing, err := editor.store.ListLayersForAsset(ctx, membership.TenantID, assetID)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]LayerRecord, len(existing))
	overlayCount := 0
	for _, layer := range existing {
		byID[layer.ID] = layer
		if layer.Role == layerRoleOverlay {
			overlayCount++
		}
	}
	if len(layerIDs) != overlayCount {
		return nil, ErrValidation
	}
	seen := make(map[uuid.UUID]bool, len(layerIDs))
	ordered := make([]LayerRecord, 0, len(layerIDs))
	for ord, id := range layerIDs {
		layer, ok := byID[id]
		if !ok || layer.Role != layerRoleOverlay || seen[id] {
			return nil, ErrValidation
		}
		seen[id] = true
		updated, err := editor.store.UpdateLayerOrd(ctx, membership.TenantID, id, layer.Revision, ord)
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, updated)
	}
	return ordered, nil
}

// RollbackLayer moves one layer's effective head to a historical revision and
// enqueues a merge job. The revision count is unchanged: no new revision row.
func (editor *LayerEdit) RollbackLayer(ctx context.Context, actor Principal, tenantSlug string, layerID, idempotencyKey uuid.UUID, input LayerRollbackInput) (JobAccepted, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, "layer:edit")
	if err != nil {
		return JobAccepted{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	layer, err := editor.store.GetLayer(ctx, membership.TenantID, layerID)
	if err != nil {
		return JobAccepted{}, err
	}
	target, err := editor.store.GetLayerRevision(ctx, membership.TenantID, input.TargetRevisionID)
	if err != nil {
		return JobAccepted{}, err
	}
	if target.LayerID != layer.ID {
		return JobAccepted{}, ErrNotFound
	}
	if target.ReviewStatus != layerRevisionReviewNotRequired && target.ReviewStatus != layerRevisionReviewApproved {
		return JobAccepted{}, ErrValidation
	}
	scopeType, scopeKey := normalizeScope(input.ScopeType, input.ScopeKey)
	head, err := editor.store.GetLayerHead(ctx, membership.TenantID, layer.ID, scopeType, scopeKey)
	if err != nil {
		return JobAccepted{}, err
	}
	if input.ExpectedEffectiveRevisionID != nil && head.EffectiveRevisionID != nil && *input.ExpectedEffectiveRevisionID != *head.EffectiveRevisionID {
		return JobAccepted{}, ErrPrecondition
	}
	if _, err := editor.store.UpdateLayerHeadPointers(ctx, NewLayerHead{
		TenantID: membership.TenantID, LayerID: layer.ID, ScopeType: scopeType, ScopeKey: scopeKey,
		LatestRevisionID: head.LatestRevisionID, EffectiveRevisionID: &target.ID, CandidateRevisionID: nil,
		Generation: head.Generation + 1,
	}); err != nil {
		return JobAccepted{}, err
	}

	// Resolve the asset's track for this scope and enqueue a merge job. The
	// merge worker materializes the next version (+1) without a new revision.
	track, err := editor.resolveTrack(ctx, membership.TenantID, layer.AssetID, scopeType, scopeKey)
	if err != nil {
		return JobAccepted{}, err
	}
	return editor.store.EnqueueMergeJob(ctx, MergeJobInput{
		TenantID: membership.TenantID, TrackID: track.ID, IdempotencyKey: idempotencyKey,
	})
}

// MaterializeTrack re-merges one track's effective layers into a new version
// and advances the track head. It is invoked by the asset.merge worker after a
// layer head mutation. Re-merging identical input is a no-op.
func (editor *LayerEdit) MaterializeTrack(ctx context.Context, tenantID, trackID uuid.UUID) (AssetVersionRecord, bool, error) {
	track, err := editor.store.GetAssetRefTrackByID(ctx, tenantID, trackID)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	asset := AssetRecord{ID: track.AssetID}
	return editor.materialize(ctx, tenantID, asset, track)
}

// Run adapts the asset.merge worker invocation to the layer editing use case,
// so the LayerEdit service satisfies task.MergeRunner.
func (editor *LayerEdit) Run(ctx context.Context, args task.MergeArgs) (task.MergeResult, error) {
	version, noop, err := editor.MaterializeTrack(ctx, args.TenantID, args.TrackID)
	if err != nil {
		return task.MergeResult{}, err
	}
	return task.MergeResult{VersionID: version.ID, Noop: noop}, nil
}

// buildFingerprint derives the deterministic build fingerprint over the ordered
// layer-revision manifest and the merge-engine version.
func (editor *LayerEdit) buildFingerprint(selected []selectedLayer) (string, error) {
	manifest := make([]LayerManifestEntry, 0, len(selected))
	for _, layer := range selected {
		manifest = append(manifest, LayerManifestEntry{
			LayerID: layer.LayerID, RevisionID: layer.RevisionID, Role: layer.Role, Origin: layer.Origin,
			Ord: layer.Ord, ScopeType: layer.ScopeType, ScopeKey: layer.ScopeKey,
			ReviewStatus: layer.ReviewStatus, ContentHash: layer.ContentHash,
		})
	}
	payload, err := json.Marshal(struct {
		Manifest           []LayerManifestEntry `json:"manifest"`
		MergeEngineVersion string               `json:"mergeEngineVersion"`
	}{Manifest: manifest, MergeEngineVersion: mergeEngineVersion})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// selectedLayer is one layer revision chosen for a merge.
type selectedLayer struct {
	LayerID      uuid.UUID
	RevisionID   uuid.UUID
	Role         string
	Origin       string
	Ord          int
	ScopeType    string
	ScopeKey     string
	ReviewStatus string
	ContentHash  string
	Content      string
}

// effectiveRevision resolves the effective head revision for one layer scope,
// falling back to the global head when an exact-branch head is absent.
func (editor *LayerEdit) effectiveRevision(ctx context.Context, tenantID uuid.UUID, layer LayerRecord, scopeType, scopeKey string) (LayerRevisionRecord, error) {
	head, err := editor.store.GetLayerHead(ctx, tenantID, layer.ID, scopeType, scopeKey)
	if err != nil {
		if scopeType == overlayScopeRef {
			head, err = editor.store.GetLayerHead(ctx, tenantID, layer.ID, overlayScopeGlobal, overlayScopeKeyGlobal)
		}
		if err != nil {
			return LayerRevisionRecord{}, err
		}
	}
	revisionID := head.EffectiveRevisionID
	if revisionID == nil {
		revisionID = head.LatestRevisionID
	}
	if revisionID == nil {
		return LayerRevisionRecord{}, ErrNotFound
	}
	return editor.store.GetLayerRevision(ctx, tenantID, *revisionID)
}

// selectedRevisions materializes the ordered layer selection for a preview.
func (editor *LayerEdit) selectedRevisions(ctx context.Context, tenantID uuid.UUID, input MergePreviewInput, base LayerRecord, overlays []LayerRecord) ([]selectedLayer, error) {
	if len(input.Layers) > 0 {
		// Explicit selection: resolve each (layerId, revisionId) pair directly.
		selected := make([]selectedLayer, 0, len(input.Layers))
		for _, selector := range input.Layers {
			revision, err := editor.store.GetLayerRevision(ctx, tenantID, selector.RevisionID)
			if err != nil {
				return nil, err
			}
			if revision.LayerID != selector.LayerID {
				return nil, ErrValidation
			}
			layer, err := editor.store.GetLayer(ctx, tenantID, selector.LayerID)
			if err != nil {
				return nil, err
			}
			selected = append(selected, selectedLayer{
				LayerID: layer.ID, RevisionID: revision.ID, Role: layer.Role, Origin: layer.Origin, Ord: layer.Ord,
				ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
				ReviewStatus: revision.ReviewStatus, ContentHash: revision.ContentHash,
				Content: revision.ContentRef,
			})
		}
		// Load blob content for each selected revision.
		for index := range selected {
			content, err := editor.blobContent(ctx, selected[index].Content)
			if err != nil {
				return nil, err
			}
			selected[index].Content = content
		}
		return selected, nil
	}

	scopeType, scopeKey := previewScope(input.RefType, input.RefName)
	selected := make([]selectedLayer, 0, len(overlays)+1)
	baseRevision, err := editor.effectiveRevision(ctx, tenantID, base, scopeType, scopeKey)
	if err != nil {
		return nil, err
	}
	baseContent, err := editor.blobContent(ctx, baseRevision.ContentRef)
	if err != nil {
		return nil, err
	}
	selected = append(selected, selectedLayer{
		LayerID: base.ID, RevisionID: baseRevision.ID, Role: base.Role, Origin: base.Origin, Ord: base.Ord,
		ScopeType: baseRevision.ScopeType, ScopeKey: baseRevision.ScopeKey,
		ReviewStatus: baseRevision.ReviewStatus, ContentHash: baseRevision.ContentHash, Content: baseContent,
	})
	for _, overlay := range overlays {
		revision, err := editor.effectiveRevision(ctx, tenantID, overlay, scopeType, scopeKey)
		if err != nil {
			return nil, err
		}
		overlayContent, err := editor.blobContent(ctx, revision.ContentRef)
		if err != nil {
			return nil, err
		}
		selected = append(selected, selectedLayer{
			LayerID: overlay.ID, RevisionID: revision.ID, Role: overlay.Role, Origin: overlay.Origin, Ord: overlay.Ord,
			ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
			ReviewStatus: revision.ReviewStatus, ContentHash: revision.ContentHash, Content: overlayContent,
		})
	}
	return selected, nil
}

// materialize merges a track's effective layers into a new version, or returns
// the existing latest version when the fingerprint is unchanged.
func (editor *LayerEdit) materialize(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, track AssetRefTrackRecord) (AssetVersionRecord, bool, error) {
	layers, err := editor.store.ListLayersForAsset(ctx, tenantID, asset.ID)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	base, overlays, err := splitBaseAndOverlays(layers)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	scopeType, scopeKey := overlayScopeRef, overlayRefTypeBranch+mergeScopeRefSelectorPrefix+track.RefName
	selected := make([]selectedLayer, 0, len(overlays)+1)
	baseRevision, err := editor.effectiveRevision(ctx, tenantID, base, scopeType, scopeKey)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	baseContent, err := editor.blobContent(ctx, baseRevision.ContentRef)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	selected = append(selected, selectedLayer{
		LayerID: base.ID, RevisionID: baseRevision.ID, Role: base.Role, Origin: base.Origin, Ord: base.Ord,
		ScopeType: baseRevision.ScopeType, ScopeKey: baseRevision.ScopeKey,
		ReviewStatus: baseRevision.ReviewStatus, ContentHash: baseRevision.ContentHash, Content: baseContent,
	})
	for _, overlay := range overlays {
		revision, err := editor.effectiveRevision(ctx, tenantID, overlay, scopeType, scopeKey)
		if err != nil {
			return AssetVersionRecord{}, false, err
		}
		overlayContent, err := editor.blobContent(ctx, revision.ContentRef)
		if err != nil {
			return AssetVersionRecord{}, false, err
		}
		selected = append(selected, selectedLayer{
			LayerID: overlay.ID, RevisionID: revision.ID, Role: overlay.Role, Origin: overlay.Origin, Ord: overlay.Ord,
			ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
			ReviewStatus: revision.ReviewStatus, ContentHash: revision.ContentHash, Content: overlayContent,
		})
	}

	content := make(map[string]any)
	for _, layer := range selected {
		if layer.Role == layerRoleBase {
			decoded, decodeErr := decodeBaseDocument(layer.Content)
			if decodeErr != nil {
				return AssetVersionRecord{}, false, decodeErr
			}
			content = decoded
			continue
		}
		raw, parseErr := ParseOverlay([]byte(layer.Content))
		if parseErr != nil {
			return AssetVersionRecord{}, false, parseErr
		}
		merged, _, _, mergeErr := ApplyPlatformOverlay(content, raw)
		if mergeErr != nil {
			return AssetVersionRecord{}, false, mergeErr
		}
		content = merged
	}
	mergedBytes, err := CanonicalJSON(content)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	mergedHash := sha256.Sum256(mergedBytes)
	mergedHashText := hex.EncodeToString(mergedHash[:])

	fingerprint, err := editor.buildFingerprint(selected)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	latest, latestErr := editor.store.GetLatestVersionInTrack(ctx, tenantID, track.ID)
	if latestErr == nil && latest.InputFingerprint == fingerprint {
		return latest, false, nil
	}
	sequence := int64(1)
	if latestErr == nil {
		sequence = latest.SequenceNo + 1
	}
	manifest := make([]LayerManifestEntry, 0, len(selected))
	for _, layer := range selected {
		manifest = append(manifest, LayerManifestEntry{
			LayerID: layer.LayerID, RevisionID: layer.RevisionID, Role: layer.Role, Origin: layer.Origin, Ord: layer.Ord,
			ScopeType: layer.ScopeType, ScopeKey: layer.ScopeKey, ReviewStatus: layer.ReviewStatus, ContentHash: layer.ContentHash,
		})
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	labels, err := json.Marshal(map[string]any{})
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	version, err := editor.store.CreateAssetVersion(ctx, NewAssetVersion{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: asset.ID, TrackID: track.ID, SequenceNo: sequence,
		Version: nextVersionLabel(latest, sequence), Lifecycle: "draft", Revision: 1,
		InputFingerprint: fingerprint, MergeEngineVersion: mergeEngineVersion,
		KindPluginVersion: new(openapiPluginVersion), LayerManifest: manifestBytes,
		MergedHash: new(mergedHashText), SourceCommit: nil, BaselineVersionID: nil,
		Labels: labels, IndexComplete: false,
	})
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	if err := editor.store.UpdateAssetRefTrackHead(ctx, tenantID, track.ID, new(version.ID), nil, 1); err != nil {
		return AssetVersionRecord{}, false, err
	}
	return version, true, nil
}

// resolveTrack returns the existing or newly created track for a layer scope.
func (editor *LayerEdit) resolveTrack(ctx context.Context, tenantID, assetID uuid.UUID, scopeType, scopeKey string) (AssetRefTrackRecord, error) {
	if scopeType == overlayScopeGlobal {
		defaultBranch, err := editor.store.GetAssetRepositoryDefaultBranch(ctx, tenantID, assetID)
		if err != nil {
			return AssetRefTrackRecord{}, err
		}
		return editor.store.GetAssetRefTrack(ctx, tenantID, assetID, overlayRefTypeBranch, defaultBranch)
	}
	refType := overlayRefTypeBranch
	refName := scopeKey
	if index := strings.Index(scopeKey, mergeScopeRefSelectorPrefix); index >= 0 {
		refType = scopeKey[:index]
		refName = scopeKey[index+1:]
	}
	track, err := editor.store.GetAssetRefTrack(ctx, tenantID, assetID, refType, refName)
	if err == nil {
		return track, nil
	}
	if !isNotFound(err) {
		return AssetRefTrackRecord{}, err
	}
	return editor.store.CreateAssetRefTrack(ctx, NewAssetRefTrack{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: assetID, RefType: refType, RefName: refName, Health: "ok",
	})
}

func (editor *LayerEdit) blobContent(ctx context.Context, contentRef string) (string, error) {
	if editor.blobs == nil {
		return "", ErrNotFound
	}
	file, _, err := editor.blobs.Open(contentRef)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (editor *LayerEdit) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, "*")) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || editor.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := editor.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

// splitBaseAndOverlays orders the asset's layers: base first, then overlays by
// ord. A missing base is a validation error; duplicate overlay ord is rejected.
func splitBaseAndOverlays(layers []LayerRecord) (LayerRecord, []LayerRecord, error) {
	var base LayerRecord
	foundBase := false
	overlays := make([]LayerRecord, 0, len(layers))
	for _, layer := range layers {
		if !layer.Enabled {
			continue
		}
		if layer.Role == layerRoleBase {
			if foundBase {
				return LayerRecord{}, nil, ErrValidation
			}
			base = layer
			foundBase = true
			continue
		}
		if layer.Role == layerRoleOverlay {
			overlays = append(overlays, layer)
		}
	}
	if !foundBase {
		return LayerRecord{}, nil, ErrValidation
	}
	slices.SortFunc(overlays, func(a, b LayerRecord) int { return a.Ord - b.Ord })
	for index := 1; index < len(overlays); index++ {
		if overlays[index].Ord == overlays[index-1].Ord {
			return LayerRecord{}, nil, &OverlayInvalidError{Errors: []OverlayIssue{{Severity: "error", Code: overlayInvalidCode, Message: "duplicate overlay ord"}}}
		}
	}
	return base, overlays, nil
}

// decodeBaseDocument parses a base layer document. Base content is JSON or
// YAML; it is normalized to the decoded document form for overlay application.
func decodeBaseDocument(content string) (map[string]any, error) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content), &decoded); err == nil {
		return decoded, nil
	}
	if err := yaml.Unmarshal([]byte(content), &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// normalizeScope fills the scope_type and scope_key defaults for a manual
// submission: an empty scope type defaults to the global scope.
func normalizeScope(scopeType, scopeKey string) (string, string) {
	if scopeType == "" {
		scopeType = overlayScopeGlobal
	}
	if scopeKey == "" {
		scopeKey = overlayScopeKeyGlobal
	}
	return scopeType, scopeKey
}

// previewScope resolves a merge preview's ref into a layer scope.
func previewScope(refType, refName string) (string, string) {
	if refName == "" {
		return overlayScopeGlobal, overlayScopeKeyGlobal
	}
	if refType == "" {
		refType = overlayRefTypeBranch
	}
	return overlayScopeRef, refType + mergeScopeRefSelectorPrefix + refName
}

// parseAssetLayersETag validates the opaque If-Match token for a reorder.
func parseAssetLayersETag(etag string, assetID uuid.UUID) error {
	_, err := parseRevisionETag(etag, "asset-layers", assetID)
	return err
}

// MergePreviewInput describes one non-persisting merge preview request.
type MergePreviewInput struct {
	AssetID uuid.UUID
	RefType string
	RefName string
	Layers  []MergeLayerSelector
}

// MergeLayerSelector selects one layer revision for a preview.
type MergeLayerSelector struct {
	LayerID    uuid.UUID
	RevisionID uuid.UUID
}

// MergePreviewResult carries the non-persisted merge output.
type MergePreviewResult struct {
	InputFingerprint string
	Content          string
	ContentType      string
	Validation       []OverlayIssue
	Provenance       []ProvenanceEntryRecord
}

// LayerRevisionInput describes one manual overlay revision submission.
type LayerRevisionInput struct {
	ScopeType       string
	ScopeKey        string
	Content         string
	ContentType     string
	Dialect         *string
	SubmitForReview bool
}

// LayerRollbackInput describes one rollback request.
type LayerRollbackInput struct {
	ScopeType                   string
	ScopeKey                    string
	ExpectedEffectiveRevisionID *uuid.UUID
	TargetRevisionID            uuid.UUID
}

// ProvenanceEntryRecord is one pointer-to-last-writing-layer provenance entry.
type ProvenanceEntryRecord struct {
	Pointer    string
	LayerID    uuid.UUID
	RevisionID uuid.UUID
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrNotFound)
}
