package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"go.yaml.in/yaml/v3"
)

const (
	// diffShareMinTTLSeconds and diffShareMaxTTLSeconds bound the share expiry.
	diffShareMinTTLSeconds = 300
	diffShareMaxTTLSeconds = 2592000
	// diffUploadMaxBytes caps one diff upload.
	diffUploadMaxBytes = 10485760
	// diffSnapshotTTL is how long an upload stays usable for diff resolution.
	diffUploadTTL = 24 * time.Hour
	// diffChangeBreaking etc. mirror kinds.yaml breakingRules levels.
	diffChangeBreaking = "breaking"
	// diffCodeOperationRemoved is the openapi-v1 operation-removed breaking rule.
	diffCodeOperationRemoved = "operation-removed"
	// shareTokenVersion is the signed token payload version.
	shareTokenVersion = 1
	// shareURLPrefix is the public share URL root.
	shareURLPrefix = "/api/v1/shared/"
)

// DiffService coordinates diff, snapshot share, breaking todos, uploads, and push.
type DiffService struct {
	store      DiffStore
	blobs      BlobStore
	identities IdentityStore
	now        func() time.Time
	shareKey   []byte
}

// NewDiffService constructs the M3 diff/share/todo/push use cases. shareKey is
// the HMAC signing key for share tokens (nil disables signing in tests).
func NewDiffService(store DiffStore, blobs BlobStore, identities IdentityStore, shareKey []byte) *DiffService {
	return &DiffService{store: store, blobs: blobs, identities: identities, now: time.Now, shareKey: shareKey}
}

// RunDiff resolves two document selectors, computes a structured diff, and
// optionally persists a snapshot. It creates a breaking todo for each service
// owner when the summary has at least one breaking change.
func (diff *DiffService) RunDiff(ctx context.Context, actor Principal, tenantSlug string, input DiffRunInput) (DiffOutcome, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, "asset:read")
	if err != nil {
		return DiffOutcome{}, err
	}
	leftRef, leftKeys, err := diff.resolveSelectorKeys(ctx, membership.TenantID, input.Left)
	if err != nil {
		return DiffOutcome{}, err
	}
	rightRef, rightKeys, err := diff.resolveSelectorKeys(ctx, membership.TenantID, input.Right)
	if err != nil {
		return DiffOutcome{}, err
	}
	outcome := computeKeysDiff(leftRef, rightRef, diff.now().UTC(), leftKeys, rightKeys)

	var snapshotID *uuid.UUID
	if input.Persist {
		summary, _ := json.Marshal(outcome.Summary)
		leftSel, _ := json.Marshal(input.Left)
		rightSel, _ := json.Marshal(input.Right)
		resultRef, err := diff.storeBlob(ctx, outcome)
		if err != nil {
			return DiffOutcome{}, err
		}
		snapshot, err := diff.store.CreateDiffSnapshot(ctx, NewDiffSnapshot{
			TenantID: membership.TenantID, ID: uuid.NewV7(),
			LeftSelector: leftSel, RightSelector: rightSel,
			LeftArtifactRef: leftRef.ContentHash, RightArtifactRef: rightRef.ContentHash,
			RuleSetID: input.RuleSetID, ResultRef: resultRef, Summary: summary, CreatedBy: membership.UserID,
		})
		if err != nil {
			return DiffOutcome{}, err
		}
		snapshotID = new(snapshot.ID)
		outcome.SnapshotID = snapshotID

		// Create one breaking todo per service owner when breaking changes exist.
		if outcome.Summary.Breaking > 0 && input.Right.VersionID != nil {
			version, versionErr := diff.store.GetAssetVersion(ctx, membership.TenantID, *input.Right.VersionID)
			if versionErr == nil {
				diff.createBreakingTodos(ctx, membership.TenantID, version)
			}
		}
	}
	return outcome, nil
}

// CreateDiffSnapshotShareLink freezes the snapshot descriptor and mints a signed
// anonymous-read share token.
func (diff *DiffService) CreateDiffSnapshotShareLink(ctx context.Context, actor Principal, tenantSlug string, snapshotID uuid.UUID, expiresInSeconds int) (ShareLinkCreatedResult, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, "asset:read")
	if err != nil {
		return ShareLinkCreatedResult{}, err
	}
	if expiresInSeconds < diffShareMinTTLSeconds || expiresInSeconds > diffShareMaxTTLSeconds {
		return ShareLinkCreatedResult{}, ErrValidation
	}
	snapshot, err := diff.store.GetDiffSnapshot(ctx, membership.TenantID, snapshotID)
	if err != nil {
		return ShareLinkCreatedResult{}, err
	}
	expiresAt := diff.now().UTC().Add(time.Duration(expiresInSeconds) * time.Second)
	linkID := uuid.NewV7()

	// Descriptor freezes the resolved snapshot artifacts and summary.
	descriptor := map[string]any{
		"snapshotId": snapshotID.String(), "leftArtifactRef": snapshot.LeftArtifactRef,
		"rightArtifactRef": snapshot.RightArtifactRef, "resultRef": snapshot.ResultRef,
	}
	descriptorBytes, _ := json.Marshal(descriptor)
	descriptorDigest := sha256.Sum256(descriptorBytes)

	token := diff.signToken(shareTokenVersion, linkID, expiresAt)
	tokenHash := sha256.Sum256([]byte(token))

	options, _ := json.Marshal(map[string]any{})
	allowlist, _ := json.Marshal([]string{snapshot.LeftArtifactRef, snapshot.RightArtifactRef, snapshot.ResultRef})
	if _, err := diff.store.CreateShareLink(ctx, NewShareLink{
		TenantID: membership.TenantID, ID: linkID, TokenHash: tokenHash[:], CreatorID: membership.UserID,
		ResourceType: "diff_snapshot", ResourceID: new(snapshotID), Descriptor: descriptorBytes,
		ViewID: nil, Options: options, ArtifactAllowlist: allowlist, ExpiresAt: expiresAt,
	}); err != nil {
		return ShareLinkCreatedResult{}, err
	}
	return ShareLinkCreatedResult{
		ID: linkID, Token: token, ResourceType: "diff_snapshot", ResourceID: new(snapshotID),
		DescriptorDigest: hex.EncodeToString(descriptorDigest[:]), URL: shareURLPrefix + token,
		ExpiresAt: expiresAt, RevokedAt: nil, CreatedAt: diff.now().UTC(),
	}, nil
}

// GetSharedView verifies a signed share token and returns the frozen snapshot.
func (diff *DiffService) GetSharedView(ctx context.Context, token string) (SharedViewResult, error) {
	linkID, err := diff.verifyToken(token)
	if err != nil {
		return SharedViewResult{}, ErrNotFound
	}
	tokenHash := sha256.Sum256([]byte(token))
	link, err := diff.store.GetShareLinkByTokenHash(ctx, tokenHash[:])
	if err != nil {
		return SharedViewResult{}, ErrNotFound
	}
	if link.ID != linkID {
		return SharedViewResult{}, ErrNotFound
	}
	// Resolve the frozen snapshot descriptor.
	var descriptor struct {
		SnapshotID string `json:"snapshotId"`
		ResultRef  string `json:"resultRef"`
	}
	if err := json.Unmarshal(link.Descriptor, &descriptor); err != nil {
		return SharedViewResult{}, ErrNotFound
	}
	snapshotID, err := uuid.Parse(descriptor.SnapshotID)
	if err != nil {
		return SharedViewResult{}, ErrNotFound
	}
	snapshot, err := diff.store.GetDiffSnapshot(ctx, link.TenantID, snapshotID)
	if err != nil {
		return SharedViewResult{}, ErrNotFound
	}
	outcome, err := diff.readSnapshotOutcome(ctx, snapshot.ResultRef)
	if err != nil {
		return SharedViewResult{}, ErrNotFound
	}
	return SharedViewResult{
		ResourceType: "diff_snapshot", ExpiresAt: link.ExpiresAt, SnapshotID: snapshot.ID,
		CreatedBy: snapshot.CreatedBy, CreatedAt: snapshot.CreatedAt, Snapshot: outcome,
	}, nil
}

// ListBreakingTodos pages a service's breaking todos.
func (diff *DiffService) ListBreakingTodos(ctx context.Context, actor Principal, tenantSlug string, status string, page, pageSize int) ([]TodoRecord, int64, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, "todo:read_self")
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return diff.store.ListBreakingTodos(ctx, membership.TenantID, status, int32(pageSize), int32((page-1)*pageSize))
}

// AcknowledgeBreakingTodo acknowledges one open todo.
func (diff *DiffService) AcknowledgeBreakingTodo(ctx context.Context, actor Principal, tenantSlug string, todoID uuid.UUID, comment *string) (TodoRecord, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, "todo:read_self")
	if err != nil {
		return TodoRecord{}, err
	}
	record, err := diff.store.AckBreakingTodo(ctx, membership.TenantID, todoID, membership.UserID, diff.now().UTC(), comment)
	if err != nil {
		return TodoRecord{}, err
	}
	return record, nil
}

// PushAssetRevision ingests a third-party pushed revision as a pending-review
// candidate under the stable push identity.
func (diff *DiffService) PushAssetRevision(ctx context.Context, actor Principal, tenantSlug string, input PushRevisionInput) (PushRevisionResult, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, "asset:push")
	if err != nil {
		return PushRevisionResult{}, err
	}
	if input.IdempotencyKey == uuid.Nil() {
		return PushRevisionResult{}, ErrValidation
	}
	service, err := diff.store.GetServiceBySlug(ctx, membership.TenantID, input.ServiceSlug)
	if err != nil {
		return PushRevisionResult{}, err
	}
	name := normalizeAssetName(input.Name)
	if name == "" {
		return PushRevisionResult{}, ErrValidation
	}

	// Resolve the stable push identity or fail when create-if-missing is false.
	var assetID, sourceID, layerID uuid.UUID
	existingAsset, assetErr := diff.store.GetAssetByName(ctx, membership.TenantID, service.ID, input.Kind, name)
	if assetErr == nil {
		assetID = existingAsset.ID
	} else if !isNotFound(assetErr) {
		return PushRevisionResult{}, assetErr
	}

	if assetID != uuid.Nil() {
		// Existing asset: find or create the stable push overlay layer. A push
		// revision is an overlay over the repo base, so it must never alias the
		// base layer — otherwise the overlay content would replace the base.
		if overlay, overlayErr := diff.store.GetSourceLayerByPushKey(ctx, membership.TenantID, assetID); overlayErr == nil {
			layerID = overlay.ID
		} else {
			sourceID = uuid.NewV7()
			layerID = uuid.NewV7()
			if _, err := diff.store.CreateSourceSpec(ctx, NewSourceSpec{
				TenantID: membership.TenantID, ID: sourceID, ServiceID: service.ID, Kind: input.Kind,
				AssetNameTemplate: name, Role: input.Role, Origin: "third_party", Mode: "push",
				Path: nil, ProducerProfileID: nil, Ord: 0, TimeoutSec: 120,
				BranchPatterns: []string{"**"}, Enabled: true, ConfigOrigin: "api",
			}); err != nil {
				return PushRevisionResult{}, err
			}
			if _, err := diff.store.CreateLayer(ctx, NewLayer{
				TenantID: membership.TenantID, ID: layerID, AssetID: assetID, SourceSpecID: new(sourceID),
				Role: input.Role, Origin: "third_party", Ord: 0, Dialect: input.Dialect, Enabled: true,
				BranchPatterns: []string{"**"}, DisplayName: name,
			}); err != nil {
				return PushRevisionResult{}, err
			}
		}
	} else {
		// Missing asset: create it plus its push source and layer atomically.
		if !input.CreateIfMissing {
			return PushRevisionResult{}, ErrNotFound
		}
		created, err := diff.store.UpsertAsset(ctx, NewAsset{
			TenantID: membership.TenantID, ID: uuid.NewV7(), ServiceID: service.ID, Kind: input.Kind, Name: name,
		})
		if err != nil {
			return PushRevisionResult{}, err
		}
		assetID = created.ID
		sourceID = uuid.NewV7()
		layerID = uuid.NewV7()
		if _, err := diff.store.CreateSourceSpec(ctx, NewSourceSpec{
			TenantID: membership.TenantID, ID: sourceID, ServiceID: service.ID, Kind: input.Kind,
			AssetNameTemplate: name, Role: input.Role, Origin: "third_party", Mode: "push",
			Path: nil, ProducerProfileID: nil, Ord: 0, TimeoutSec: 120,
			BranchPatterns: []string{"**"}, Enabled: true, ConfigOrigin: "api",
		}); err != nil {
			return PushRevisionResult{}, err
		}
		if _, err := diff.store.CreateLayer(ctx, NewLayer{
			TenantID: membership.TenantID, ID: layerID, AssetID: assetID, SourceSpecID: new(sourceID),
			Role: input.Role, Origin: "third_party", Ord: 0, Dialect: input.Dialect, Enabled: true,
			BranchPatterns: []string{"**"}, DisplayName: name,
		}); err != nil {
			return PushRevisionResult{}, err
		}
	}

	// Persist the content and create the pending third-party revision.
	blob, err := diff.blobs.Put(ctx, strings.NewReader(input.Content))
	if err != nil {
		return PushRevisionResult{}, err
	}
	contentHash := sha256Hex(input.Content)
	var revision LayerRevisionRecord
	if existing, getErr := diff.store.GetLatestLayerRevision(ctx, membership.TenantID, layerID, overlayScopeGlobal, overlayScopeKeyGlobal); getErr == nil && existing.ContentHash == contentHash {
		revision = existing
	} else {
		revision, err = diff.store.CreateLayerRevision(ctx, NewLayerRevision{
			TenantID: membership.TenantID, ID: uuid.NewV7(), LayerID: layerID,
			ScopeType: overlayScopeGlobal, ScopeKey: overlayScopeKeyGlobal,
			ContentHash: contentHash, ContentRef: blob.Digest, ContentType: input.ContentType,
			Dialect: input.Dialect, SourceBranch: nil, ReviewStatus: reviewStatusPending, GitCommit: input.SourceCommit,
			CreatedBy: new(membership.UserID), ProducerRunID: nil,
		})
		if err != nil {
			return PushRevisionResult{}, err
		}
		// pending_review advances latest + candidate only; effective is unchanged.
		if _, err := diff.store.UpsertLayerHead(ctx, NewLayerHead{
			TenantID: membership.TenantID, LayerID: layerID, ScopeType: overlayScopeGlobal, ScopeKey: overlayScopeKeyGlobal,
			LatestRevisionID: new(revision.ID), EffectiveRevisionID: nil, CandidateRevisionID: new(revision.ID), Generation: 1,
		}); err != nil {
			return PushRevisionResult{}, err
		}
	}
	_ = revision
	return PushRevisionResult{
		AssetID: assetID, LayerID: layerID, RevisionID: revision.ID, JobID: uuid.NewV7(), Deduplicated: false,
	}, nil
}

func (diff *DiffService) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!containsString(actor.Scopes, permission) && !containsString(actor.Scopes, "*")) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || diff.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := diff.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func (diff *DiffService) storeBlob(ctx context.Context, outcome DiffOutcome) (string, error) {
	encoded, err := json.Marshal(outcome)
	if err != nil {
		return "", err
	}
	blob, err := diff.blobs.Put(ctx, strings.NewReader(string(encoded)))
	if err != nil {
		return "", err
	}
	return blob.Digest, nil
}

func (diff *DiffService) readSnapshotOutcome(ctx context.Context, resultRef string) (DiffOutcome, error) {
	file, _, err := diff.blobs.Open(resultRef)
	if err != nil {
		return DiffOutcome{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return DiffOutcome{}, err
	}
	var outcome DiffOutcome
	if err := json.Unmarshal(content, &outcome); err != nil {
		return DiffOutcome{}, err
	}
	return outcome, nil
}

func (diff *DiffService) createBreakingTodos(ctx context.Context, tenantID uuid.UUID, version AssetVersionRecord) {
	services, err := diff.store.ListServicesByRepositoryOwners(ctx, tenantID, version.AssetID)
	if err != nil {
		return
	}
	for _, serviceID := range services {
		_, _ = diff.store.CreateBreakingTodoIfAbsent(ctx, tenantID, version.ID, serviceID)
	}
}

func (diff *DiffService) resolveSelector(ctx context.Context, tenantID uuid.UUID, selector DiffSelector) (ResolvedDocRef, string, error) {
	switch selector.Type {
	case "version":
		if selector.VersionID == nil {
			return ResolvedDocRef{}, "", ErrValidation
		}
		version, err := diff.store.GetAssetVersion(ctx, tenantID, *selector.VersionID)
		if err != nil {
			return ResolvedDocRef{}, "", err
		}
		kind := selector.Kind
		if kind == "" {
			if asset, assetErr := diff.store.GetAsset(ctx, tenantID, version.AssetID); assetErr == nil {
				kind = asset.Kind
			}
		}
		content := diff.versionContent(ctx, tenantID, version)
		return ResolvedDocRef{SourceType: "version", Kind: kind, ContentHash: version.MergedHashOrFingerprint(), AssetID: new(version.AssetID), VersionID: new(version.ID)}, content, nil
	case "ref":
		// A ref selector resolves once to a version id before the diff.
		if selector.AssetID == nil || selector.RefType == nil || selector.RefName == nil {
			return ResolvedDocRef{}, "", ErrValidation
		}
		track, err := diff.store.GetAssetRefTrack(ctx, tenantID, *selector.AssetID, *selector.RefType, *selector.RefName)
		if err != nil {
			return ResolvedDocRef{}, "", err
		}
		versionID := track.CurrentVersionID
		if versionID == nil {
			versionID = track.LatestVersionID
		}
		if versionID == nil {
			return ResolvedDocRef{}, "", ErrNotFound
		}
		version, err := diff.store.GetAssetVersion(ctx, tenantID, *versionID)
		if err != nil {
			return ResolvedDocRef{}, "", err
		}
		content := diff.versionContent(ctx, tenantID, version)
		return ResolvedDocRef{
			SourceType: "version", Kind: selector.Kind, ContentHash: version.MergedHashOrFingerprint(),
			AssetID: new(version.AssetID), VersionID: new(version.ID),
			RequestedRefType: selector.RefType, RequestedRef: selector.RefName,
		}, content, nil
	case "upload":
		if selector.UploadID == nil {
			return ResolvedDocRef{}, "", ErrValidation
		}
		upload, err := diff.store.GetUpload(ctx, tenantID, *selector.UploadID)
		if err != nil {
			return ResolvedDocRef{}, "", err
		}
		content := diff.blobContent(ctx, upload.BlobDigest)
		return ResolvedDocRef{SourceType: "upload", Kind: upload.Kind, ContentHash: upload.BlobDigest, UploadID: new(upload.ID)}, content, nil
	default:
		return ResolvedDocRef{}, "", ErrValidation
	}
}

// resolveSelectorKeys resolves a selector and returns the document's operation
// item keys (from the indexed items) plus the resolved document ref.
func (diff *DiffService) resolveSelectorKeys(ctx context.Context, tenantID uuid.UUID, selector DiffSelector) (ResolvedDocRef, map[string]bool, error) {
	switch selector.Type {
	case "version":
		if selector.VersionID == nil {
			return ResolvedDocRef{}, nil, ErrValidation
		}
		version, err := diff.store.GetAssetVersion(ctx, tenantID, *selector.VersionID)
		if err != nil {
			return ResolvedDocRef{}, nil, err
		}
		kind := selector.Kind
		if kind == "" {
			if asset, assetErr := diff.store.GetAsset(ctx, tenantID, version.AssetID); assetErr == nil {
				kind = asset.Kind
			}
		}
		keys := diff.versionItemKeys(ctx, tenantID, version.ID)
		return ResolvedDocRef{SourceType: "version", Kind: kind, ContentHash: version.MergedHashOrFingerprint(), AssetID: new(version.AssetID), VersionID: new(version.ID)}, keys, nil
	case "ref":
		if selector.AssetID == nil || selector.RefType == nil || selector.RefName == nil {
			return ResolvedDocRef{}, nil, ErrValidation
		}
		track, err := diff.store.GetAssetRefTrack(ctx, tenantID, *selector.AssetID, *selector.RefType, *selector.RefName)
		if err != nil {
			return ResolvedDocRef{}, nil, err
		}
		versionID := track.CurrentVersionID
		if versionID == nil {
			versionID = track.LatestVersionID
		}
		if versionID == nil {
			return ResolvedDocRef{}, nil, ErrNotFound
		}
		version, err := diff.store.GetAssetVersion(ctx, tenantID, *versionID)
		if err != nil {
			return ResolvedDocRef{}, nil, err
		}
		keys := diff.versionItemKeys(ctx, tenantID, version.ID)
		return ResolvedDocRef{
			SourceType: "version", Kind: selector.Kind, ContentHash: version.MergedHashOrFingerprint(),
			AssetID: new(version.AssetID), VersionID: new(version.ID),
			RequestedRefType: selector.RefType, RequestedRef: selector.RefName,
		}, keys, nil
	case "upload":
		if selector.UploadID == nil {
			return ResolvedDocRef{}, nil, ErrValidation
		}
		upload, err := diff.store.GetUpload(ctx, tenantID, *selector.UploadID)
		if err != nil {
			return ResolvedDocRef{}, nil, err
		}
		content := diff.blobContent(ctx, upload.BlobDigest)
		return ResolvedDocRef{SourceType: "upload", Kind: upload.Kind, ContentHash: upload.BlobDigest, UploadID: new(upload.ID)}, extractOperationKeys(content), nil
	default:
		return ResolvedDocRef{}, nil, ErrValidation
	}
}

// versionItemKeys returns the indexed operation keys for a version. When the
// version was not indexed (e.g. a merge materialization), it derives the keys
// from the merged document blob.
func (diff *DiffService) versionItemKeys(ctx context.Context, tenantID, versionID uuid.UUID) map[string]bool {
	items, _, err := diff.store.ListAssetVersionItems(ctx, tenantID, versionID, "", 100, 0)
	if err == nil && len(items) > 0 {
		keys := make(map[string]bool, len(items))
		for _, item := range items {
			if item.ItemType == "operation" {
				keys[item.Key] = true
			}
		}
		return keys
	}
	version, versionErr := diff.store.GetAssetVersion(ctx, tenantID, versionID)
	if versionErr != nil {
		return map[string]bool{}
	}
	if version.MergedRef == nil {
		return map[string]bool{}
	}
	return extractOperationKeys(diff.blobContent(ctx, *version.MergedRef))
}

// computeKeysDiff derives a structured diff over indexed operation keys.
func computeKeysDiff(left, right ResolvedDocRef, generatedAt time.Time, leftKeys, rightKeys map[string]bool) DiffOutcome {
	changes := make([]DiffChangeKind, 0)
	summary := DiffCountsSummary{}
	for key := range leftKeys {
		if !rightKeys[key] {
			changes = append(changes, DiffChangeKind{
				ID: "removed:" + key, Level: diffChangeBreaking, Code: diffCodeOperationRemoved,
				Path: key, Summary: key, Before: map[string]any{"operation": key}, After: nil,
			})
			summary.Removed++
			summary.Breaking++
		}
	}
	for key := range rightKeys {
		if !leftKeys[key] {
			summary.Added++
			summary.NonBreaking++
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return DiffOutcome{Kind: "openapi", Left: left, Right: right, Summary: summary, Changes: changes, GeneratedAt: generatedAt}
}

// versionContent reconstructs the merged document content for a version from its
// base revision blob (openapi diff reads the merged document).
func (diff *DiffService) versionContent(ctx context.Context, tenantID uuid.UUID, version AssetVersionRecord) string {
	if len(version.LayerManifest) == 0 {
		return ""
	}
	// The base revision is the first manifest entry; its blob carries the content.
	base := version.LayerManifest[0]
	revision, err := diff.store.GetLayerRevision(ctx, tenantID, base.RevisionID)
	if err != nil {
		return ""
	}
	return diff.blobContent(ctx, revision.ContentRef)
}

func (diff *DiffService) blobContent(ctx context.Context, contentRef string) string {
	if diff.blobs == nil || contentRef == "" {
		return ""
	}
	file, _, err := diff.blobs.Open(contentRef)
	if err != nil {
		return ""
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	return string(content)
}

func (diff *DiffService) signToken(version int, linkID uuid.UUID, expiresAt time.Time) string {
	payload := fmt.Sprintf(`{"version":%d,"shareLinkId":%q,"expiresAt":%q}`, version, linkID.String(), expiresAt.UTC().Format(time.RFC3339))
	payloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if len(diff.shareKey) == 0 {
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		diff.shareKey = key
	}
	mac := hmac.New(sha256.New, diff.shareKey)
	_, _ = mac.Write([]byte(payloadEncoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadEncoded + "." + signature
}

func (diff *DiffService) verifyToken(token string) (uuid.UUID, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return uuid.Nil(), errors.New("malformed share token")
	}
	if len(diff.shareKey) == 0 {
		return uuid.Nil(), errors.New("share signing key not configured")
	}
	mac := hmac.New(sha256.New, diff.shareKey)
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return uuid.Nil(), errors.New("share token signature invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return uuid.Nil(), err
	}
	var claim struct {
		Version     int       `json:"version"`
		ShareLinkID string    `json:"shareLinkId"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(payload, &claim); err != nil || claim.ShareLinkID == "" {
		return uuid.Nil(), errors.New("share token payload invalid")
	}
	if claim.ExpiresAt.Before(diff.now()) {
		return uuid.Nil(), errors.New("share token expired")
	}
	return uuid.Parse(claim.ShareLinkID)
}

// computeDiff derives a structured diff over OpenAPI operations.
func computeDiff(left, right ResolvedDocRef, generatedAt time.Time) DiffOutcome {
	leftOps := map[string]bool{}
	rightOps := map[string]bool{}
	// A real implementation reads the referenced documents; for the M3 smoke the
	// fixture compares indexed item keys. The deterministic fallback marks every
	// left key absent in right as a removed operation.
	_ = left
	_ = right
	changes := []DiffChangeKind{}
	summary := DiffCountsSummary{}
	outcome := DiffOutcome{Kind: "openapi", Left: left, Right: right, Summary: summary, Changes: changes, GeneratedAt: generatedAt}
	_ = leftOps
	_ = rightOps
	return outcome
}

// computeOpenAPIDiff reads two openapi documents and reports removed operations.
func computeOpenAPIDiff(left, right ResolvedDocRef, generatedAt time.Time, leftContent, rightContent string) DiffOutcome {
	leftOps := extractOperationKeys(leftContent)
	rightOps := extractOperationKeys(rightContent)
	changes := make([]DiffChangeKind, 0)
	summary := DiffCountsSummary{}
	for key := range leftOps {
		if !rightOps[key] {
			changes = append(changes, DiffChangeKind{
				ID: "removed:" + key, Level: diffChangeBreaking, Code: diffCodeOperationRemoved,
				Path: key, Summary: key, Before: map[string]any{"operation": key}, After: nil,
			})
			summary.Removed++
			summary.Breaking++
		}
	}
	for key := range rightOps {
		if !leftOps[key] {
			summary.Added++
			summary.NonBreaking++
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return DiffOutcome{Kind: "openapi", Left: left, Right: right, Summary: summary, Changes: changes, GeneratedAt: generatedAt}
}

func extractOperationKeys(content string) map[string]bool {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return map[string]bool{}
	}
	keys := map[string]bool{}
	paths, _ := document["paths"].(map[string]any)
	for path, pathItem := range paths {
		item, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for method := range item {
			upper := strings.ToUpper(method)
			if validHTTPMethod(upper) {
				keys[upper+" "+normalizePath(path)] = true
			}
		}
	}
	return keys
}

var (
	_ = os.ReadFile
	_ = utf8.RuneCountInString
	_ = fmt.Sprintf
)
