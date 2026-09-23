package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	collaboration "github.com/meridian-labs/meridian/internal/generated/api/collaboration"
	diff "github.com/meridian-labs/meridian/internal/generated/api/diff"
	view "github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// diffSelectorType* 是 DocumentSelector 判别值对应的服务层选择器类型。
const (
	// diffSelectorTypeVersion 表示按资产版本选择文档。
	diffSelectorTypeVersion = "version"
	// diffSelectorTypeRef 表示按引用（branch/tag）选择文档。
	diffSelectorTypeRef = "ref"
	// diffSelectorTypeUpload 表示按上传文档选择。
	diffSelectorTypeUpload = "upload"
)

// RunDiff 在两个文档选择器之间执行一次结构化差异对比。
func (s *Server) RunDiff(ctx context.Context, request diff.RunDiffRequestObject) (diff.RunDiffResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	left, err := diffSelector(request.Body.Left)
	if err != nil {
		return nil, err
	}
	right, err := diffSelector(request.Body.Right)
	if err != nil {
		return nil, err
	}
	persist := true
	if request.Body.Persist != nil {
		persist = *request.Body.Persist
	}
	var ruleSetID *uuid.UUID
	if request.Body.RuleSetId.IsSpecified() && !request.Body.RuleSetId.IsNull() {
		value := serviceUUID(request.Body.RuleSetId.MustGet())
		ruleSetID = &value
	}
	outcome, err := s.diffService.RunDiff(ctx, principal, string(request.TenantSlug), service.DiffRunInput{
		Left: left, Right: right, RuleSetID: ruleSetID, Persist: persist,
	})
	if err != nil {
		return nil, err
	}
	return diff.RunDiff200JSONResponse(diffResultResponse(outcome)), nil
}

// CreateDiffSnapshotShareLink 为冻结快照铸造一个分享令牌。
func (s *Server) CreateDiffSnapshotShareLink(ctx context.Context, request diff.CreateDiffSnapshotShareLinkRequestObject) (diff.CreateDiffSnapshotShareLinkResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.diffService.CreateDiffSnapshotShareLink(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.SnapshotId), request.Body.ExpiresInSeconds,
	)
	if err != nil {
		return nil, err
	}
	return diff.CreateDiffSnapshotShareLink201JSONResponse(shareLinkCreatedResponse(created)), nil
}

// GetSharedView 将匿名分享令牌解析为其冻结快照。
func (s *Server) GetSharedView(ctx context.Context, request view.GetSharedViewRequestObject) (view.GetSharedViewResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	result, err := s.diffService.GetSharedView(ctx, string(request.ShareToken))
	if err != nil {
		return nil, err
	}
	var resolution api.SharedView_Resolution
	if result.Resolution != nil {
		if err := resolution.FromViewResolution(viewResolutionResponse(*result.Resolution)); err != nil {
			return nil, err
		}
	} else {
		if err := resolution.FromDiffSnapshot(diffSnapshotResponse(result)); err != nil {
			return nil, err
		}
	}
	return view.GetSharedView200JSONResponse(api.SharedView{
		ResourceType: api.SharedViewResourceType(result.ResourceType),
		ExpiresAt:    result.ExpiresAt,
		Resolution:   resolution,
	}), nil
}

// ListBreakingTodos 分页返回租户的破坏性待办。
func (s *Server) ListBreakingTodos(ctx context.Context, request collaboration.ListBreakingTodosRequestObject) (collaboration.ListBreakingTodosResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	status := ""
	if request.Params.Status != nil {
		status = string(*request.Params.Status)
	}
	items, total, err := s.diffService.ListBreakingTodos(ctx, principal, string(request.TenantSlug), status, page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.BreakingTodo, 0, len(items))
	for _, item := range items {
		responses = append(responses, breakingTodoResponse(item))
	}
	return collaboration.ListBreakingTodos200JSONResponse(api.BreakingTodoPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: responses,
	}), nil
}

// AcknowledgeBreakingTodo 确认一条未关闭的待办。
func (s *Server) AcknowledgeBreakingTodo(ctx context.Context, request collaboration.AcknowledgeBreakingTodoRequestObject) (collaboration.AcknowledgeBreakingTodoResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var comment *string
	if request.Body.Comment.IsSpecified() && !request.Body.Comment.IsNull() {
		value := request.Body.Comment.MustGet()
		comment = &value
	}
	record, err := s.diffService.AcknowledgeBreakingTodo(ctx, principal, string(request.TenantSlug), serviceUUID(request.TodoId), comment)
	if err != nil {
		return nil, err
	}
	return collaboration.AcknowledgeBreakingTodo200JSONResponse(breakingTodoResponse(record)), nil
}

// breakingTodoResponse 将破坏性待办记录投影为 API 形状。
func breakingTodoResponse(record service.TodoRecord) api.BreakingTodo {
	ackedBy := nullable.NewNullNullable[api.Uuid]()
	if record.AckedBy != nil {
		ackedBy = nullable.NewNullableWithValue(api.Uuid(*record.AckedBy))
	}
	ackedAt := nullable.NewNullNullable[api.Timestamp]()
	if record.AckedAt != nil {
		ackedAt = nullable.NewNullableWithValue(*record.AckedAt)
	}
	comment := nullable.NewNullNullable[string]()
	if record.Comment != nil {
		comment = nullable.NewNullableWithValue(*record.Comment)
	}
	return api.BreakingTodo{
		Id: api.Uuid(record.ID), AssetVersionId: api.Uuid(record.AssetVersionID), ServiceId: api.Uuid(record.ServiceID),
		Status: api.TodoStatus(record.Status), Summary: api.DiffCounts{}, AckedBy: ackedBy, Comment: comment,
		CreatedAt: record.CreatedAt, AcknowledgedAt: ackedAt,
	}
}

// shareLinkCreatedResponse 将分享令牌创建结果投影为 API 形状。
func shareLinkCreatedResponse(result service.ShareLinkCreatedResult) api.ShareLinkCreated {
	resourceID := nullable.NewNullNullable[api.Uuid]()
	if result.ResourceID != nil {
		resourceID = nullable.NewNullableWithValue(api.Uuid(*result.ResourceID))
	}
	revokedAt := nullable.NewNullNullable[api.Timestamp]()
	return api.ShareLinkCreated{
		Id: api.Uuid(result.ID), Token: result.Token, ResourceType: api.ShareLinkCreatedResourceType(result.ResourceType),
		ResourceId: resourceID, DescriptorDigest: result.DescriptorDigest, Url: result.URL,
		ExpiresAt: result.ExpiresAt, RevokedAt: revokedAt, CreatedAt: result.CreatedAt,
	}
}

// diffSnapshotResponse 将分享视图快照投影为 API 形状。
func diffSnapshotResponse(result service.SharedViewResult) api.DiffSnapshot {
	return api.DiffSnapshot{
		Id:        api.Uuid(result.SnapshotID),
		Result:    diffResultResponse(result.Snapshot),
		CreatedBy: api.Uuid(result.CreatedBy),
		CreatedAt: result.CreatedAt,
	}
}

// diffResultResponse 将差异结果投影为 API 形状。
func diffResultResponse(outcome service.DiffOutcome) api.DiffResult {
	changes := make([]api.DiffChange, 0, len(outcome.Changes))
	for _, change := range outcome.Changes {
		changes = append(changes, api.DiffChange{
			Id: change.ID, Level: api.DiffLevel(change.Level), Code: change.Code,
			Path: change.Path, Summary: change.Summary, Before: change.Before, After: change.After,
		})
	}
	snapshotID := nullable.NewNullNullable[api.Uuid]()
	if outcome.SnapshotID != nil {
		snapshotID = nullable.NewNullableWithValue(api.Uuid(*outcome.SnapshotID))
	}
	return api.DiffResult{
		Kind: api.KindId(outcome.Kind), Left: resolvedRefResponse(outcome.Left), Right: resolvedRefResponse(outcome.Right),
		SnapshotId: snapshotID, Summary: diffCountsResponse(outcome.Summary), Changes: changes, GeneratedAt: outcome.GeneratedAt,
	}
}

// resolvedRefResponse 将已解析文档引用投影为 API 形状。
func resolvedRefResponse(ref service.ResolvedDocRef) api.ResolvedDocumentRef {
	assetID := nullable.NewNullNullable[api.Uuid]()
	if ref.AssetID != nil {
		assetID = nullable.NewNullableWithValue(api.Uuid(*ref.AssetID))
	}
	versionID := nullable.NewNullNullable[api.Uuid]()
	if ref.VersionID != nil {
		versionID = nullable.NewNullableWithValue(api.Uuid(*ref.VersionID))
	}
	uploadID := nullable.NewNullNullable[api.Uuid]()
	if ref.UploadID != nil {
		uploadID = nullable.NewNullableWithValue(api.Uuid(*ref.UploadID))
	}
	refType := nullable.NewNullNullable[api.RefType]()
	if ref.RequestedRefType != nil {
		refType = nullable.NewNullableWithValue(api.RefType(*ref.RequestedRefType))
	}
	refName := nullable.NewNullNullable[api.RefName]()
	if ref.RequestedRef != nil {
		refName = nullable.NewNullableWithValue(api.RefName(*ref.RequestedRef))
	}
	return api.ResolvedDocumentRef{
		SourceType: api.ResolvedDocumentRefSourceType(ref.SourceType), Kind: api.KindId(ref.Kind), ContentHash: ref.ContentHash,
		AssetId: assetID, VersionId: versionID, UploadId: uploadID, RequestedRefType: refType, RequestedRef: refName,
	}
}

// diffCountsResponse 将差异计数投影为 API 形状。
func diffCountsResponse(summary service.DiffCountsSummary) api.DiffCounts {
	return api.DiffCounts{
		Added: summary.Added, Removed: summary.Removed, Modified: summary.Modified, Breaking: summary.Breaking,
		Risky: summary.Risky, NonBreaking: summary.NonBreaking, Informational: summary.Informational,
	}
}

// diffSelector 将生成的 DocumentSelector 联合体转换为服务层形状。
func diffSelector(selector api.DocumentSelector) (service.DiffSelector, error) {
	value, err := selector.ValueByDiscriminator()
	if err != nil {
		return service.DiffSelector{}, service.ErrValidation
	}
	switch typed := value.(type) {
	case api.VersionSelector:
		versionID := serviceUUID(typed.VersionId)
		return service.DiffSelector{Type: diffSelectorTypeVersion, VersionID: &versionID}, nil
	case api.RefSelector:
		assetID := serviceUUID(typed.AssetId)
		refType := string(typed.RefType)
		refName := string(typed.Ref)
		return service.DiffSelector{Type: diffSelectorTypeRef, AssetID: &assetID, RefType: &refType, RefName: &refName}, nil
	case api.UploadSelector:
		uploadID := serviceUUID(typed.UploadId)
		return service.DiffSelector{Type: diffSelectorTypeUpload, UploadID: &uploadID}, nil
	default:
		return service.DiffSelector{}, service.ErrValidation
	}
}
