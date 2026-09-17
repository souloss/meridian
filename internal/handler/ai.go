package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	layer "github.com/meridian-labs/meridian/internal/generated/api/layer"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// GenerateMissingAssetWithAi enqueues one AI generation for a missing asset.
func (s *Server) GenerateMissingAssetWithAi(ctx context.Context, request asset.GenerateMissingAssetWithAiRequestObject) (asset.GenerateMissingAssetWithAiResponseObject, error) {
	if s.aiWorkflow == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input := service.AiGenerateInput{
		Kind: string(request.Body.Kind), Name: string(request.Body.Name),
		IdempotencyKey: serviceUUID(api.Uuid(request.Params.IdempotencyKey)),
		PrincipalType:  string(principal.Kind), PrincipalID: rotationPrincipalID(principal),
	}
	if request.Body.RefType != nil {
		input.RefType = string(*request.Body.RefType)
	}
	if request.Body.Ref != nil {
		input.Ref = string(*request.Body.Ref)
	}
	if request.Body.Hint.IsSpecified() && !request.Body.Hint.IsNull() {
		input.Hint = request.Body.Hint.MustGet()
	}
	if request.Body.ProducerProfileId != nil {
		input.ProducerProfileID = serviceUUID(*request.Body.ProducerProfileId)
	}
	requestHash, err := service.RequestDigest("generateMissingAssetWithAi", map[string]any{
		"tenantSlug": string(request.TenantSlug), "serviceSlug": string(request.ServiceSlug),
	}, map[string]any{}, request.Body)
	if err != nil {
		return nil, err
	}
	input.RequestHash = requestHash

	accepted, err := s.aiWorkflow.GenerateMissingAsset(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug), input)
	if err != nil {
		return nil, err
	}
	return asset.GenerateMissingAssetWithAi202JSONResponse(api.AiGenerationAccepted{
		AssetId: api.Uuid(accepted.AssetID), SourceId: api.Uuid(accepted.SourceID),
		JobId: api.Uuid(accepted.JobID), Deduplicated: accepted.Deduplicated,
	}), nil
}

// GetReviewContext returns the candidate, current effective revision, and author.
func (s *Server) GetReviewContext(ctx context.Context, request layer.GetReviewContextRequestObject) (layer.GetReviewContextResponseObject, error) {
	if s.aiWorkflow == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.aiWorkflow.GetReviewContext(ctx, principal, string(request.TenantSlug), serviceUUID(request.RevisionId))
	if err != nil {
		return nil, err
	}
	current := nullable.NewNullNullable[api.LayerRevision]()
	if result.CurrentEffectiveRevision != nil {
		current = nullable.NewNullableWithValue(layerRevisionResponse(*result.CurrentEffectiveRevision))
	}
	author := nullable.NewNullNullable[api.User]()
	if result.Author != nil {
		author = nullable.NewNullableWithValue(userResponse(*result.Author))
	}
	return layer.GetReviewContext200JSONResponse(api.ReviewContext{
		Revision:                layerRevisionResponse(result.Revision),
		CurrentEffectiveRevision: current,
		Diff:                    emptyDiffResult(),
		Author:                  author,
	}), nil
}

// ApproveLayerRevision approves one pending candidate and enqueues a merge job.
func (s *Server) ApproveLayerRevision(ctx context.Context, request layer.ApproveLayerRevisionRequestObject) (layer.ApproveLayerRevisionResponseObject, error) {
	if s.aiWorkflow == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var comment *string
	if request.Body.Comment.IsSpecified() && !request.Body.Comment.IsNull() {
		comment = new(request.Body.Comment.MustGet())
	}
	decision, err := s.aiWorkflow.ApproveLayerRevision(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RevisionId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), comment,
	)
	if err != nil {
		return nil, err
	}
	return layer.ApproveLayerRevision200JSONResponse(reviewResultResponse(decision)), nil
}

// RejectLayerRevision rejects one pending candidate without advancing the head.
func (s *Server) RejectLayerRevision(ctx context.Context, request layer.RejectLayerRevisionRequestObject) (layer.RejectLayerRevisionResponseObject, error) {
	if s.aiWorkflow == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	decision, err := s.aiWorkflow.RejectLayerRevision(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RevisionId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), request.Body.Comment,
	)
	if err != nil {
		return nil, err
	}
	return layer.RejectLayerRevision200JSONResponse(reviewResultResponse(decision)), nil
}

// PublishAssetVersion publishes one draft version and advances the track head.
func (s *Server) PublishAssetVersion(ctx context.Context, request asset.PublishAssetVersionRequestObject) (asset.PublishAssetVersionResponseObject, error) {
	if s.aiWorkflow == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseVersionETag(request.Params.IfMatch, serviceUUID(request.VersionId))
	if err != nil {
		return nil, service.ErrPrecondition
	}
	var versionLabel *string
	if request.Body.Version.IsSpecified() && !request.Body.Version.IsNull() {
		value := request.Body.Version.MustGet()
		versionLabel = &value
	}
	labels := make(map[string]string)
	if request.Body.Labels != nil {
		for name, value := range *request.Body.Labels {
			if text, ok := value.(string); ok {
				labels[name] = text
			}
		}
	}
	requestHash, err := service.RequestDigest("publishAssetVersion", map[string]any{
		"tenantSlug": string(request.TenantSlug), "versionId": request.VersionId.String(),
	}, map[string]any{"if-match": request.Params.IfMatch}, request.Body)
	if err != nil {
		return nil, err
	}
	updated, err := s.aiWorkflow.PublishAssetVersion(ctx, principal, string(request.TenantSlug), serviceUUID(request.VersionId), service.PublishInput{
		VersionID: serviceUUID(request.VersionId), ExpectedRevision: expectedRevision,
		Version: versionLabel, Labels: labels,
		IdempotencyKey: serviceUUID(api.Uuid(request.Params.IdempotencyKey)),
		PrincipalType:  string(principal.Kind), PrincipalID: rotationPrincipalID(principal), RequestHash: requestHash,
	})
	if err != nil {
		return nil, err
	}
	body := assetVersionResponse(updated)
	return asset.PublishAssetVersion200JSONResponse{
		Body: body, Headers: asset.PublishAssetVersion200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// reviewResultResponse projects one approve/reject decision onto the API shape.
func reviewResultResponse(decision service.ReviewDecision) api.RevisionReviewResult {
	effective := nullable.NewNullNullable[api.Uuid]()
	if decision.EffectiveRevisionID != nil {
		effective = nullable.NewNullableWithValue(api.Uuid(*decision.EffectiveRevisionID))
	}
	mergeJob := nullable.NewNullNullable[api.Uuid]()
	if decision.MergeJobID != nil {
		mergeJob = nullable.NewNullableWithValue(api.Uuid(*decision.MergeJobID))
	}
	superseded := make([]api.Uuid, 0, len(decision.SupersededRevisionIDs))
	for _, id := range decision.SupersededRevisionIDs {
		superseded = append(superseded, api.Uuid(id))
	}
	return api.RevisionReviewResult{
		Revision:              layerRevisionResponse(decision.Revision),
		EffectiveRevisionId:   effective,
		MergeJobId:            mergeJob,
		SupersededRevisionIds: superseded,
		Deduplicated:          decision.Deduplicated,
	}
}

// emptyDiffResult returns an empty diff for a cold-start review context.
func emptyDiffResult() api.DiffResult {
	return api.DiffResult{
		Kind: "openapi", Summary: api.DiffCounts{}, Changes: []api.DiffChange{},
		Left:  emptyDocumentRef(), Right: emptyDocumentRef(),
		SnapshotId: nullable.NewNullNullable[api.Uuid](),
	}
}

// emptyDocumentRef returns an empty resolved document ref for the skeleton diff.
func emptyDocumentRef() api.ResolvedDocumentRef {
	return api.ResolvedDocumentRef{
		SourceType: api.ResolvedDocumentRefSourceType("version"), Kind: "openapi", ContentHash: "",
		AssetId: nullable.NewNullNullable[api.Uuid](), VersionId: nullable.NewNullNullable[api.Uuid](),
		UploadId: nullable.NewNullNullable[api.Uuid](),
		RequestedRefType: nullable.NewNullNullable[api.RefType](), RequestedRef: nullable.NewNullNullable[api.RefName](),
	}
}

// rotationPrincipalID returns the stable principal identifier for replay identity.
func rotationPrincipalID(principal service.Principal) uuid.UUID {
	if principal.Kind == service.PrincipalPAT {
		return principal.TokenID
	}
	return principal.User.ID
}

// parseVersionETag validates the opaque If-Match token for a publish request.
func parseVersionETag(etag string, versionID uuid.UUID) (int64, error) {
	prefix := fmt.Sprintf(`"asset-version:%s:`, versionID.String())
	if !strings.HasPrefix(etag, prefix) || !strings.HasSuffix(etag, `"`) {
		return 0, errors.New("invalid entity tag")
	}
	revisionText := strings.TrimSuffix(strings.TrimPrefix(etag, prefix), `"`)
	var revision int64
	if _, err := fmt.Sscan(revisionText, &revision); err != nil || revision < 1 {
		return 0, errors.New("invalid entity tag revision")
	}
	return revision, nil
}

var _ = strings.TrimSpace
