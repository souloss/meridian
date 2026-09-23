package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/meridian-labs/meridian/internal/storage"
)

const (
	// maxDiffRuleSetNameRunes 是差异规则集名称的最大字符数。
	maxDiffRuleSetNameRunes = 128
)

// ListDiffRuleSets 返回租户内全部差异规则集。
func (diff *DiffService) ListDiffRuleSets(ctx context.Context, actor Principal, tenantSlug string) ([]DiffRuleSetRecord, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, err
	}
	return diff.store.ListDiffRuleSets(ctx, membership.TenantID)
}

// CreateDiffRuleSet 校验并创建一条差异规则集。
func (diff *DiffService) CreateDiffRuleSet(ctx context.Context, actor Principal, tenantSlug string, input NewDiffRuleSet) (DiffRuleSetRecord, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return DiffRuleSetRecord{}, err
	}
	if err := validateDiffRuleSetName(input.Name); err != nil {
		return DiffRuleSetRecord{}, err
	}
	if !validKindID(input.Kind) {
		return DiffRuleSetRecord{}, ErrValidation
	}
	input.TenantID = membership.TenantID
	input.ID = uuid.NewV7()
	return diff.store.CreateDiffRuleSet(ctx, input)
}

// UpdateDiffRuleSet 在 If-Match 下更新一条差异规则集。
func (diff *DiffService) UpdateDiffRuleSet(ctx context.Context, actor Principal, tenantSlug string, ruleSetID uuid.UUID, etag string, patch DiffRuleSetPatch) (DiffRuleSetRecord, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return DiffRuleSetRecord{}, err
	}
	current, err := diff.store.GetDiffRuleSet(ctx, membership.TenantID, ruleSetID)
	if err != nil {
		return DiffRuleSetRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "diff-rule-set", ruleSetID)
	if err != nil {
		return DiffRuleSetRecord{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return DiffRuleSetRecord{}, ErrPrecondition
	}
	if patch.Name != nil {
		if err := validateDiffRuleSetName(*patch.Name); err != nil {
			return DiffRuleSetRecord{}, err
		}
	}
	return diff.store.UpdateDiffRuleSet(ctx, membership.TenantID, ruleSetID, expectedRevision, patch)
}

// DeleteDiffRuleSet 在 If-Match 下删除一条差异规则集。
func (diff *DiffService) DeleteDiffRuleSet(ctx context.Context, actor Principal, tenantSlug string, ruleSetID uuid.UUID, etag string) error {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return err
	}
	current, err := diff.store.GetDiffRuleSet(ctx, membership.TenantID, ruleSetID)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "diff-rule-set", ruleSetID)
	if err != nil {
		return ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return ErrPrecondition
	}
	return diff.store.DeleteDiffRuleSet(ctx, membership.TenantID, ruleSetID, expectedRevision)
}

// GetDiffSnapshot 返回一条冻结的差异快照。
func (diff *DiffService) GetDiffSnapshot(ctx context.Context, actor Principal, tenantSlug string, snapshotID uuid.UUID) (DiffSnapshotRecord, DiffOutcome, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return DiffSnapshotRecord{}, DiffOutcome{}, err
	}
	record, err := diff.store.GetDiffSnapshot(ctx, membership.TenantID, snapshotID)
	if err != nil {
		return DiffSnapshotRecord{}, DiffOutcome{}, err
	}
	outcome, err := diff.readSnapshotOutcome(ctx, record.ResultRef)
	if err != nil {
		return DiffSnapshotRecord{}, DiffOutcome{}, err
	}
	return record, outcome, nil
}

// ListDiffSnapshots 返回租户内一页差异快照。
func (diff *DiffService) ListDiffSnapshots(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]DiffSnapshotRecord, int64, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return diff.store.ListDiffSnapshots(ctx, membership.TenantID, int32(pageSize), int32((page-1)*pageSize))
}

// DeleteDiffSnapshot 删除一条差异快照。
func (diff *DiffService) DeleteDiffSnapshot(ctx context.Context, actor Principal, tenantSlug string, snapshotID uuid.UUID) error {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return err
	}
	if _, err := diff.store.GetDiffSnapshot(ctx, membership.TenantID, snapshotID); err != nil {
		return err
	}
	return diff.store.DeleteDiffSnapshot(ctx, membership.TenantID, snapshotID)
}

// ListShareLinks 返回租户内一页分享链接。
func (diff *DiffService) ListShareLinks(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]ShareLinkRecord, int64, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return diff.store.ListShareLinks(ctx, membership.TenantID, int32(pageSize), int32((page-1)*pageSize))
}

// RevokeShareLink 幂等撤销一条分享链接。
func (diff *DiffService) RevokeShareLink(ctx context.Context, actor Principal, tenantSlug string, shareLinkID uuid.UUID) error {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return err
	}
	return diff.store.RevokeShareLink(ctx, membership.TenantID, shareLinkID)
}

// ExportDiffSnapshot 将冻结的差异结果导出为带有效期的产物下载链接。
func (diff *DiffService) ExportDiffSnapshot(ctx context.Context, actor Principal, tenantSlug string, snapshotID uuid.UUID, format string) (ArtifactLinkRecord, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return ArtifactLinkRecord{}, err
	}
	record, err := diff.store.GetDiffSnapshot(ctx, membership.TenantID, snapshotID)
	if err != nil {
		return ArtifactLinkRecord{}, err
	}
	outcome, err := diff.readSnapshotOutcome(ctx, record.ResultRef)
	if err != nil {
		return ArtifactLinkRecord{}, err
	}
	payload, err := diff.encodeDiffOutcome(outcome, format)
	if err != nil {
		return ArtifactLinkRecord{}, err
	}
	blob, err := diff.blobs.Put(ctx, strings.NewReader(string(payload)))
	if err != nil {
		return ArtifactLinkRecord{}, err
	}
	expiresAt := diff.now().UTC().Add(diffExportTTL)
	return ArtifactLinkRecord{URL: diff.signContentToken(blob.Digest), ExpiresAt: expiresAt}, nil
}

// ArtifactLinkRecord 是一个带有效期的产物下载链接投影。
type ArtifactLinkRecord struct {
	// URL 是产物下载链接。
	URL string
	// ExpiresAt 是链接过期时间。
	ExpiresAt time.Time
}

// encodeDiffOutcome 按导出格式序列化差异结果（json 或 markdown）。
func (diff *DiffService) encodeDiffOutcome(outcome DiffOutcome, format string) ([]byte, error) {
	switch format {
	case diffExportFormatMarkdown:
		return encodeDiffMarkdown(outcome), nil
	case diffExportFormatJSON, "":
		return json.Marshal(outcome)
	default:
		return nil, ErrValidation
	}
}

// encodeDiffMarkdown 将差异结果渲染为 Markdown 变更清单。
func encodeDiffMarkdown(outcome DiffOutcome) []byte {
	var builder strings.Builder
	builder.WriteString("# Diff Report\n\n")
	for _, change := range outcome.Changes {
		builder.WriteString("- **")
		builder.WriteString(change.Code)
		builder.WriteString("** ")
		builder.WriteString(change.Path)
		builder.WriteString(": ")
		builder.WriteString(change.Summary)
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

// signContentToken 为 blob 摘要铸造一个签名内容令牌，供 downloadSignedContent 匿名下载。
func (diff *DiffService) signContentToken(digest string) string {
	expiresAt := diff.now().UTC().Add(diffExportTTL)
	payload := fmt.Sprintf(`{"version":%d,"digest":%q,"expiresAt":%q}`, shareTokenVersion, digest, expiresAt.UTC().Format(time.RFC3339))
	payloadEncoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	if len(diff.shareKey) == 0 {
		key := make([]byte, shareTokenKeyBytes)
		_, _ = rand.Read(key)
		diff.shareKey = key
	}
	mac := hmac.New(sha256.New, diff.shareKey)
	_, _ = mac.Write([]byte(payloadEncoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadEncoded + "." + signature
}

// ResolveContentToken 校验签名内容令牌并返回其 blob 摘要。
func (diff *DiffService) ResolveContentToken(ctx context.Context, token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", ErrNotFound
	}
	if len(diff.shareKey) == 0 {
		return "", ErrNotFound
	}
	mac := hmac.New(sha256.New, diff.shareKey)
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", ErrNotFound
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrNotFound
	}
	var claim struct {
		Version   int       `json:"version"`
		Digest    string    `json:"digest"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(payload, &claim); err != nil || claim.Digest == "" {
		return "", ErrNotFound
	}
	if claim.ExpiresAt.Before(diff.now()) {
		return "", ErrNotFound
	}
	return claim.Digest, nil
}

// CreateDiffUpload 接收 multipart 表单并持久化一个差异上传。
func (diff *DiffService) CreateDiffUpload(ctx context.Context, actor Principal, tenantSlug string, reader *multipart.Reader) (UploadRecord, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return UploadRecord{}, err
	}
	var kind, contentType string
	var content []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return UploadRecord{}, ErrValidation
		}
		switch part.FormName() {
		case "kind":
			text, err := io.ReadAll(part)
			if err != nil {
				return UploadRecord{}, ErrValidation
			}
			kind = strings.TrimSpace(string(text))
		case "contentType":
			text, err := io.ReadAll(part)
			if err != nil {
				return UploadRecord{}, ErrValidation
			}
			contentType = strings.TrimSpace(string(text))
		case "file":
			content, err = io.ReadAll(part)
			if err != nil {
				return UploadRecord{}, ErrValidation
			}
		}
	}
	if kind == "" || len(content) == 0 {
		return UploadRecord{}, ErrValidation
	}
	if len(content) > diffUploadMaxBytes {
		return UploadRecord{}, ErrValidation
	}
	blob, err := diff.blobs.Put(ctx, strings.NewReader(string(content)))
	if err != nil {
		return UploadRecord{}, err
	}
	return diff.store.CreateUpload(ctx, NewUpload{
		TenantID: membership.TenantID, ID: uuid.NewV7(), BlobDigest: blob.Digest, Kind: kind,
		ContentType: contentType, SizeBytes: int64(len(content)), ExpiresAt: diff.now().UTC().Add(diffUploadTTL),
		CreatedBy: new(membership.UserID),
	})
}

// CreateShareLink 冻结视图选择器并铸造一个签名匿名分享令牌（createShareLink）。
func (diff *DiffService) CreateShareLink(ctx context.Context, actor Principal, tenantSlug string, input NewViewShareLink) (ShareLinkCreatedResult, error) {
	membership, err := diff.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return ShareLinkCreatedResult{}, err
	}
	if input.ExpiresInSeconds < diffShareMinTTLSeconds || input.ExpiresInSeconds > diffShareMaxTTLSeconds {
		return ShareLinkCreatedResult{}, ErrValidation
	}
	if input.ViewID == "" {
		return ShareLinkCreatedResult{}, ErrValidation
	}
	// 冻结描述符：视图 id + 输入选择器 + 选项。
	descriptor := map[string]any{
		"viewId":  input.ViewID,
		"inputs":  input.Inputs,
		"options": input.Options,
	}
	descriptorBytes, err := json.Marshal(descriptor)
	if err != nil {
		return ShareLinkCreatedResult{}, err
	}
	descriptorDigest := sha256.Sum256(descriptorBytes)
	expiresAt := diff.now().UTC().Add(time.Duration(input.ExpiresInSeconds) * time.Second)
	linkID := uuid.NewV7()
	token := diff.signToken(shareTokenVersion, linkID, expiresAt)
	tokenHash := sha256.Sum256([]byte(token))
	viewID := input.ViewID
	options, _ := json.Marshal(input.Options)
	if _, err := diff.store.CreateShareLink(ctx, NewShareLink{
		TenantID: membership.TenantID, ID: linkID, TokenHash: tokenHash[:], CreatorID: membership.UserID,
		ResourceType: shareResourceTypeView, ResourceID: nil, Descriptor: descriptorBytes,
		ViewID: &viewID, Options: options, ArtifactAllowlist: []byte("[]"), ExpiresAt: expiresAt,
	}); err != nil {
		return ShareLinkCreatedResult{}, err
	}
	return ShareLinkCreatedResult{
		ID: linkID, Token: token, ResourceType: shareResourceTypeView, ResourceID: nil,
		DescriptorDigest: hex.EncodeToString(descriptorDigest[:]), URL: shareURLPrefix + token,
		ExpiresAt: expiresAt, RevokedAt: nil, CreatedAt: diff.now().UTC(),
	}, nil
}

// GetSharedViewResolution 校验视图分享令牌并解析冻结视图（供公开视图解析）。
// 冻结描述符记录 viewId 与输入选择器，此处按单文档视图语义重建文档解析结果，
// 使 getSharedView 能返回与 resolveView 同构的 ViewResolution（Document 非空）。
func (diff *DiffService) GetSharedViewResolution(ctx context.Context, token string) (ViewResolution, error) {
	linkID, err := diff.verifyToken(token)
	if err != nil {
		return ViewResolution{}, ErrNotFound
	}
	tokenHash := sha256.Sum256([]byte(token))
	link, err := diff.store.GetShareLinkByTokenHash(ctx, tokenHash[:])
	if err != nil || link.ID != linkID {
		return ViewResolution{}, ErrNotFound
	}
	if link.ResourceType != shareResourceTypeView {
		return ViewResolution{}, ErrNotFound
	}
	var descriptor struct {
		ViewID  string           `json:"viewId"`
		Inputs  []jsontext.Value `json:"inputs"`
		Options map[string]any   `json:"options"`
	}
	_ = json.Unmarshal(link.Descriptor, &descriptor)
	resolution := ViewResolution{
		Kind:     viewResolutionKindDocument,
		View:     ViewDefinition{ID: descriptor.ViewID},
		Document: &DocumentResolution{MediaType: contentTypeYAML},
	}
	for _, input := range descriptor.Inputs {
		var selector struct {
			Type      string `json:"type"`
			VersionID string `json:"versionId"`
			UploadID  string `json:"uploadId"`
		}
		if err := json.Unmarshal(input, &selector); err != nil {
			continue
		}
		switch selector.Type {
		case diffSelectorTypeVersion:
			if versionID, parseErr := uuid.Parse(selector.VersionID); parseErr == nil {
				if version, versionErr := diff.store.GetAssetVersion(ctx, link.TenantID, versionID); versionErr == nil {
					resolution.Document.VersionID = version.ID
				}
			}
		}
	}
	return resolution, nil
}

// OpenBlob 打开一个 blob 供流式下载。
func (diff *DiffService) OpenBlob(ctx context.Context, digest string) (*os.File, storage.Blob, error) {
	return diff.blobs.Open(digest)
}

// NewViewShareLink 承载一次视图分享链接创建请求。
type NewViewShareLink struct {
	// ViewID 是被分享的视图标识。
	ViewID string
	// Inputs 是冻结的文档选择器列表。
	Inputs []any
	// Options 是冻结的视图选项。
	Options map[string]any
	// ExpiresInSeconds 是分享有效期（秒）。
	ExpiresInSeconds int
}

func validateDiffRuleSetName(name string) error {
	if name == "" || utf8.RuneCountInString(name) > maxDiffRuleSetNameRunes {
		return ErrValidation
	}
	return nil
}
