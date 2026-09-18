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

// LayerEditStore 是 M2 层编辑的持久化边界：overlay 修订、排序、回滚、合并预览与溯源读取。
// 每个方法都保留租户谓词。
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
	CreateAssetItem(context.Context, NewAssetItem) (AssetItemRecord, error)
	MarkAssetVersionIndexed(context.Context, uuid.UUID, uuid.UUID) error
	EnqueueMergeJob(context.Context, MergeJobInput) (JobAccepted, error)
}

// MergeJobInput 描述某轨道的一次幂等 asset.merge 请求。
type MergeJobInput struct {
	TenantID       uuid.UUID
	TrackID        uuid.UUID
	IdempotencyKey uuid.UUID
}

// LayerEdit 面向 M2 platform-v1 合并引擎协调 overlay 修订提交、层排序、回滚与
// 非持久化合并预览。
type LayerEdit struct {
	store      LayerEditStore
	blobs      BlobStore
	identities IdentityStore
	now        func() time.Time
}

// NewLayerEdit 构造 M2 层编辑用例。
func NewLayerEdit(store LayerEditStore, blobs BlobStore, identities IdentityStore) *LayerEdit {
	return &LayerEdit{store: store, blobs: blobs, identities: identities, now: time.Now}
}

// PreviewMerge 对资产的生效层运行真实合并引擎而不持久化任何内容。结果携带合并内容、
// 规范指纹与溯源（json-pointer → 最后写入层）。
func (editor *LayerEdit) PreviewMerge(ctx context.Context, actor Principal, tenantSlug string, input MergePreviewInput) (MergePreviewResult, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerRead)
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

	// 选择：显式层选择替换资产的应用顺序；否则按 ord 顺序合并生效层头。
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

	// 无 overlay 写入文档根时，拥有 base 的条目标记文档根。
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

// CreateLayerRevision 校验并持久化一个手动 overlay 修订，然后推进层头并入队合并任务。
// 非法 overlay 被拒绝且不持久化。
func (editor *LayerEdit) CreateLayerRevision(ctx context.Context, actor Principal, tenantSlug string, layerID uuid.UUID, input LayerRevisionInput) (LayerRevisionResult, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
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
	// 在持久化任何内容前校验：解析 + 编译 overlay。
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
	// 按修订状态机，手动 overlay 恒为 not_required。
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

	// 推进层头：not_required 移动 latest+effective。
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

	// 入队合并任务以物化新的生效内容。
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

// LayerRevisionResult 承载一个已持久化的 overlay 修订，以及为物化它而入队的合并任务。
type LayerRevisionResult struct {
	Revision     LayerRevisionRecord
	JobID        uuid.UUID
	Deduplicated bool
}

// GetAssetVersionProvenance 从层清单推导物化版本每个 JSON 指针的最后写入层与修订。
func (editor *LayerEdit) GetAssetVersionProvenance(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID) ([]ProvenanceEntryRecord, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerRead)
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

// ReorderAssetLayers 在乐观并发下持久化新的 overlay ord 序列。重复层 id 与非 overlay 层被拒绝。
func (editor *LayerEdit) ReorderAssetLayers(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID, etag string, layerIDs []uuid.UUID) ([]LayerRecord, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
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

// RollbackLayer 将层的生效头移到历史修订并入队合并任务。修订数不变：不产生新修订行。
func (editor *LayerEdit) RollbackLayer(ctx context.Context, actor Principal, tenantSlug string, layerID, idempotencyKey uuid.UUID, input LayerRollbackInput) (JobAccepted, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
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

	// 解析该作用域下资产的轨道并入队合并任务。合并 worker 在无新修订的情况下物化下一版本（+1）。
	track, err := editor.resolveTrack(ctx, membership.TenantID, layer.AssetID, scopeType, scopeKey)
	if err != nil {
		return JobAccepted{}, err
	}
	return editor.store.EnqueueMergeJob(ctx, MergeJobInput{
		TenantID: membership.TenantID, TrackID: track.ID, IdempotencyKey: idempotencyKey,
	})
}

// MaterializeTrack 将轨道的生效层重合并为新版本并推进轨道头。它在层头变更后由
// asset.merge worker 调用。重合并相同输入是无操作。
func (editor *LayerEdit) MaterializeTrack(ctx context.Context, tenantID, trackID uuid.UUID) (AssetVersionRecord, bool, error) {
	track, err := editor.store.GetAssetRefTrackByID(ctx, tenantID, trackID)
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	asset := AssetRecord{ID: track.AssetID}
	return editor.materialize(ctx, tenantID, asset, track)
}

// Run 将 asset.merge worker 调用适配到层编辑用例，使 LayerEdit 服务满足 task.MergeRunner。
func (editor *LayerEdit) Run(ctx context.Context, args task.MergeArgs) (task.MergeResult, error) {
	version, noop, err := editor.MaterializeTrack(ctx, args.TenantID, args.TrackID)
	if err != nil {
		return task.MergeResult{}, err
	}
	return task.MergeResult{VersionID: version.ID, Noop: noop}, nil
}

// buildFingerprint 对有序层修订清单与合并引擎版本派生确定性构建指纹。
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

// selectedLayer 是为合并选中的一个层修订。
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

// effectiveRevision 解析层作用域的生效头修订，在精确分支头缺失时回退到全局头。
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

// selectedRevisions 物化预览的有序层选择。
func (editor *LayerEdit) selectedRevisions(ctx context.Context, tenantID uuid.UUID, input MergePreviewInput, base LayerRecord, overlays []LayerRecord) ([]selectedLayer, error) {
	if len(input.Layers) > 0 {
		// 显式选择：直接解析每个 (layerId, revisionId) 对。
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
		// 为每个选中修订加载 blob 内容。
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

// materialize 将轨道的生效层合并为新版本，或在指纹未变时返回现有最新版本。
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

	// 将合并文档持久化到 blob store 并记录其内容引用，使下游消费者（diff、provenance）
	// 能读取合并文档。
	mergedRef := new(string(""))
	if editor.blobs != nil {
		blob, putErr := editor.blobs.Put(ctx, strings.NewReader(string(mergedBytes)))
		if putErr == nil {
			mergedRef = new(blob.Digest)
		}
	}

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
		Version: nextVersionLabel(latest, sequence), Lifecycle: lifecycleDraft, Revision: 1,
		InputFingerprint: fingerprint, MergeEngineVersion: mergeEngineVersion,
		KindPluginVersion: new(openapiPluginVersion), LayerManifest: manifestBytes,
		MergedHash: new(mergedHashText), MergedRef: mergedRef, SourceCommit: nil, BaselineVersionID: nil,
		Labels: labels, IndexComplete: false,
	})
	if err != nil {
		return AssetVersionRecord{}, false, err
	}
	if err := editor.store.UpdateAssetRefTrackHead(ctx, tenantID, track.ID, new(version.ID), nil, 1); err != nil {
		return AssetVersionRecord{}, false, err
	}
	// 通用类别（dbschema / dependency）从合并文档索引其条目；openapi 路径改为在同步管线中索引。
	if err := editor.indexGenericItems(ctx, tenantID, asset, version, content); err != nil {
		return AssetVersionRecord{}, false, err
	}
	return version, true, nil
}

// indexGenericItems 为通用资产类别索引 table/column/edge 条目，并在类别具备条目模式时
// 将版本标记为已索引。
func (editor *LayerEdit) indexGenericItems(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, version AssetVersionRecord, content map[string]any) error {
	assetRecord, assetErr := editor.store.GetAsset(ctx, tenantID, asset.ID)
	if assetErr != nil {
		return assetErr
	}
	items, err := indexGenericKindItems(assetRecord.Kind, content)
	if err != nil {
		return err
	}
	if items == nil {
		return nil
	}
	serviceID := assetRecord.ServiceID
	for _, item := range items {
		displayBytes, _ := json.Marshal(item.Display)
		if _, err := editor.store.CreateAssetItem(ctx, NewAssetItem{
			TenantID: tenantID, ID: uuid.NewV7(), AssetVersionID: version.ID, AssetID: asset.ID, ServiceID: serviceID,
			Kind: assetRecord.Kind, ItemType: item.ItemType, Key: item.Key, Display: displayBytes,
			SearchText: item.SearchText, SearchRaw: displayBytes, Provenance: []byte("{}"),
		}); err != nil {
			return err
		}
	}
	return editor.store.MarkAssetVersionIndexed(ctx, tenantID, version.ID)
}

// resolveTrack 返回层作用域的现有或新建轨道。
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
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: assetID, RefType: refType, RefName: refName, Health: assetHealthOK,
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
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard)) {
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

// splitBaseAndOverlays 排序资产层：base 在前，随后按 ord 排 overlay。缺失 base 是校验错误；
// 重复 overlay ord 被拒绝。
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

// decodeBaseDocument 解析 base 层文档。base 内容为 JSON 或 YAML；为应用 overlay 规范化
// 为解码文档形式。
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

// normalizeScope 为手动提交填充 scope_type 与 scope_key 默认值：空作用域类型默认全局作用域。
func normalizeScope(scopeType, scopeKey string) (string, string) {
	if scopeType == "" {
		scopeType = overlayScopeGlobal
	}
	if scopeKey == "" {
		scopeKey = overlayScopeKeyGlobal
	}
	return scopeType, scopeKey
}

// previewScope 将合并预览的 ref 解析为层作用域。
func previewScope(refType, refName string) (string, string) {
	if refName == "" {
		return overlayScopeGlobal, overlayScopeKeyGlobal
	}
	if refType == "" {
		refType = overlayRefTypeBranch
	}
	return overlayScopeRef, refType + mergeScopeRefSelectorPrefix + refName
}

// parseAssetLayersETag 校验重排序的不透明 If-Match 令牌。
func parseAssetLayersETag(etag string, assetID uuid.UUID) error {
	_, err := parseRevisionETag(etag, "asset-layers", assetID)
	return err
}

// MergePreviewInput 描述一次非持久化合并预览请求。
type MergePreviewInput struct {
	AssetID uuid.UUID
	RefType string
	RefName string
	Layers  []MergeLayerSelector
}

// MergeLayerSelector 为预览选择一个层修订。
type MergeLayerSelector struct {
	LayerID    uuid.UUID
	RevisionID uuid.UUID
}

// MergePreviewResult 承载非持久化的合并输出。
type MergePreviewResult struct {
	InputFingerprint string
	Content          string
	ContentType      string
	Validation       []OverlayIssue
	Provenance       []ProvenanceEntryRecord
}

// LayerRevisionInput 描述一次手动 overlay 修订提交。
type LayerRevisionInput struct {
	ScopeType       string
	ScopeKey        string
	Content         string
	ContentType     string
	Dialect         *string
	SubmitForReview bool
}

// LayerRollbackInput 描述一次回滚请求。
type LayerRollbackInput struct {
	ScopeType                   string
	ScopeKey                    string
	ExpectedEffectiveRevisionID *uuid.UUID
	TargetRevisionID            uuid.UUID
}

// ProvenanceEntryRecord 是指针到最后写入层的溯源条目。
type ProvenanceEntryRecord struct {
	Pointer    string
	LayerID    uuid.UUID
	RevisionID uuid.UUID
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrNotFound)
}
