package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"

	"github.com/meridian-labs/meridian/internal/generated/api"
	diff "github.com/meridian-labs/meridian/internal/generated/api/diff"
	view "github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListDiffRuleSets 返回租户内全部差异规则集。
func (s *Server) ListDiffRuleSets(ctx context.Context, request diff.ListDiffRuleSetsRequestObject) (diff.ListDiffRuleSetsResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.diffService.ListDiffRuleSets(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.DiffRuleSet, 0, len(records))
	for _, record := range records {
		items = append(items, diffRuleSetResponse(record))
	}
	return diff.ListDiffRuleSets200JSONResponse(api.DiffRuleSetList{Items: items}), nil
}

// CreateDiffRuleSet 校验并创建一条差异规则集。
func (s *Server) CreateDiffRuleSet(ctx context.Context, request diff.CreateDiffRuleSetRequestObject) (diff.CreateDiffRuleSetResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := json.Marshal(request.Body.Rules)
	if err != nil {
		return nil, err
	}
	record, err := s.diffService.CreateDiffRuleSet(ctx, principal, string(request.TenantSlug), service.NewDiffRuleSet{
		Kind: string(request.Body.Kind), Name: request.Body.Name, Version: 1, Rules: rules, Enabled: true,
	})
	if err != nil {
		return nil, err
	}
	body := diffRuleSetResponse(record)
	return diff.CreateDiffRuleSet201JSONResponse{
		Body: body, Headers: diff.CreateDiffRuleSet201ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateDiffRuleSet 在 If-Match 下更新一条差异规则集。
func (s *Server) UpdateDiffRuleSet(ctx context.Context, request diff.UpdateDiffRuleSetRequestObject) (diff.UpdateDiffRuleSetResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var rules []byte
	if request.Body.Rules != nil {
		encoded, err := json.Marshal(*request.Body.Rules)
		if err != nil {
			return nil, err
		}
		rules = encoded
	}
	record, err := s.diffService.UpdateDiffRuleSet(ctx, principal, string(request.TenantSlug), serviceUUID(request.RuleSetId), request.Params.IfMatch, service.DiffRuleSetPatch{
		Name: request.Body.Name, Rules: rules,
	})
	if err != nil {
		return nil, err
	}
	body := diffRuleSetResponse(record)
	return diff.UpdateDiffRuleSet200JSONResponse{
		Body: body, Headers: diff.UpdateDiffRuleSet200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteDiffRuleSet 在 If-Match 下删除一条差异规则集。
func (s *Server) DeleteDiffRuleSet(ctx context.Context, request diff.DeleteDiffRuleSetRequestObject) (diff.DeleteDiffRuleSetResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.diffService.DeleteDiffRuleSet(ctx, principal, string(request.TenantSlug), serviceUUID(request.RuleSetId), request.Params.IfMatch); err != nil {
		return nil, err
	}
	return diff.DeleteDiffRuleSet204Response{}, nil
}

// ListDiffSnapshots 返回租户内一页差异快照。
func (s *Server) ListDiffSnapshots(ctx context.Context, request diff.ListDiffSnapshotsRequestObject) (diff.ListDiffSnapshotsResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.diffService.ListDiffSnapshots(ctx, principal, string(request.TenantSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.DiffSnapshot, 0, len(records))
	for _, record := range records {
		items = append(items, api.DiffSnapshot{
			Id: api.Uuid(record.ID), Result: diffResultResponseFromSummary(record.Summary),
			CreatedBy: api.Uuid(record.CreatedBy), CreatedAt: record.CreatedAt,
		})
	}
	return diff.ListDiffSnapshots200JSONResponse(api.DiffSnapshotPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// GetDiffSnapshot 返回一条冻结的差异快照。
func (s *Server) GetDiffSnapshot(ctx context.Context, request diff.GetDiffSnapshotRequestObject) (diff.GetDiffSnapshotResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, outcome, err := s.diffService.GetDiffSnapshot(ctx, principal, string(request.TenantSlug), serviceUUID(request.SnapshotId))
	if err != nil {
		return nil, err
	}
	return diff.GetDiffSnapshot200JSONResponse(api.DiffSnapshot{
		Id: api.Uuid(record.ID), Result: diffResultResponse(outcome),
		CreatedBy: api.Uuid(record.CreatedBy), CreatedAt: record.CreatedAt,
	}), nil
}

// DeleteDiffSnapshot 删除一条差异快照。
func (s *Server) DeleteDiffSnapshot(ctx context.Context, request diff.DeleteDiffSnapshotRequestObject) (diff.DeleteDiffSnapshotResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.diffService.DeleteDiffSnapshot(ctx, principal, string(request.TenantSlug), serviceUUID(request.SnapshotId)); err != nil {
		return nil, err
	}
	return diff.DeleteDiffSnapshot204Response{}, nil
}

// ExportDiffSnapshot 将冻结的差异结果导出为带有效期的下载链接。
func (s *Server) ExportDiffSnapshot(ctx context.Context, request diff.ExportDiffSnapshotRequestObject) (diff.ExportDiffSnapshotResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	link, err := s.diffService.ExportDiffSnapshot(ctx, principal, string(request.TenantSlug), serviceUUID(request.SnapshotId), string(request.Params.Format))
	if err != nil {
		return nil, err
	}
	return diff.ExportDiffSnapshot200JSONResponse(api.ArtifactLink{Url: link.URL, ExpiresAt: link.ExpiresAt}), nil
}

// ListShareLinks 返回租户内一页分享链接。
func (s *Server) ListShareLinks(ctx context.Context, request view.ListShareLinksRequestObject) (view.ListShareLinksResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.diffService.ListShareLinks(ctx, principal, string(request.TenantSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.ShareLink, 0, len(records))
	for _, record := range records {
		items = append(items, shareLinkResponse(record))
	}
	return view.ListShareLinks200JSONResponse(api.ShareLinkPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// RevokeShareLink 幂等撤销一条分享链接。
func (s *Server) RevokeShareLink(ctx context.Context, request view.RevokeShareLinkRequestObject) (view.RevokeShareLinkResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.diffService.RevokeShareLink(ctx, principal, string(request.TenantSlug), serviceUUID(request.ShareLinkId)); err != nil {
		return nil, err
	}
	return view.RevokeShareLink204Response{}, nil
}

// CreateDiffUpload 接收 multipart 上传并持久化一个差异上传。
func (s *Server) CreateDiffUpload(ctx context.Context, request diff.CreateDiffUploadRequestObject) (diff.CreateDiffUploadResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	upload, err := s.diffService.CreateDiffUpload(ctx, principal, string(request.TenantSlug), request.Body)
	if err != nil {
		return nil, err
	}
	return diff.CreateDiffUpload201JSONResponse(api.Upload{
		Id: api.Uuid(upload.ID), Digest: upload.BlobDigest, Kind: api.KindId(upload.Kind),
		ContentType: api.ContentType(upload.ContentType), SizeBytes: int(upload.SizeBytes), ExpiresAt: upload.ExpiresAt,
	}), nil
}

// diffRuleSetResponse 将差异规则集记录投影为 API 形状。
func diffRuleSetResponse(record service.DiffRuleSetRecord) api.DiffRuleSet {
	rules := make([]api.DiffRule, 0)
	_ = json.Unmarshal(record.Rules, &rules)
	return api.DiffRuleSet{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindDiffRuleSet, record.ID.String(), record.Revision),
		Kind: api.KindId(record.Kind), Name: record.Name, Version: int(record.Version), Rules: rules,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// shareLinkResponse 将分享链接记录投影为 API 形状。
func shareLinkResponse(record service.ShareLinkRecord) api.ShareLink {
	resourceID := nullable.NewNullNullable[api.Uuid]()
	if record.ResourceID != nil {
		resourceID = nullable.NewNullableWithValue(api.Uuid(*record.ResourceID))
	}
	revokedAt := nullable.NewNullNullable[api.Timestamp]()
	if record.RevokedAt != nil {
		revokedAt = nullable.NewNullableWithValue(*record.RevokedAt)
	}
	return api.ShareLink{
		Id: api.Uuid(record.ID), ResourceType: api.ShareLinkResourceType(record.ResourceType), ResourceId: resourceID,
		DescriptorDigest: shareLinkDescriptorDigest(record.Descriptor), ExpiresAt: record.ExpiresAt, RevokedAt: revokedAt,
		Url: "", CreatedAt: record.CreatedAt,
	}
}

// shareLinkDescriptorDigest 计算分享链接描述符的 SHA-256 十六进制摘要。
func shareLinkDescriptorDigest(descriptor []byte) string {
	sum := sha256.Sum256(descriptor)
	return hex.EncodeToString(sum[:])
}

// diffResultResponseFromSummary 从快照摘要 JSON 重建差异结果骨架。
func diffResultResponseFromSummary(summary []byte) api.DiffResult {
	var counts service.DiffCountsSummary
	_ = json.Unmarshal(summary, &counts)
	return api.DiffResult{
		Kind: "", Summary: diffCountsResponse(counts), Changes: []api.DiffChange{},
		Left: emptyDocumentRef(), Right: emptyDocumentRef(),
		SnapshotId: nullable.NewNullNullable[api.Uuid](),
	}
}

const (
	// etagKindDiffRuleSet 是差异规则集 ETag 的实体类型令牌。
	etagKindDiffRuleSet = "diff-rule-set"
)
