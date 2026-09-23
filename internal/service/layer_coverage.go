package service

import (
	"context"
	"slices"
	"uuid"
)

// LayerContentRecord 是资产某层生效内容与元数据的投影，供预览与渲染。
type LayerContentRecord struct {
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// LayerID 是层的标识。
	LayerID uuid.UUID
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源。
	Origin string
	// Ord 是 overlay 层排序号。
	Ord int
	// Content 是生效修订的内容。
	Content string
	// ContentType 是内容媒体类型。
	ContentType string
	// EffectiveRevision 是当前生效修订（可为空）。
	EffectiveRevision *LayerRevisionRecord
}

// ListAssetLayerContents 一次性返回资产每个层的生效内容与元数据。
func (editor *LayerEdit) ListAssetLayerContents(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID) ([]LayerContentRecord, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerRead)
	if err != nil {
		return nil, err
	}
	if _, err := editor.store.GetAsset(ctx, membership.TenantID, assetID); err != nil {
		return nil, err
	}
	layers, err := editor.store.ListLayersForAsset(ctx, membership.TenantID, assetID)
	if err != nil {
		return nil, err
	}
	records := make([]LayerContentRecord, 0, len(layers))
	for _, layer := range layers {
		record := LayerContentRecord{
			AssetID: assetID, LayerID: layer.ID, Role: layer.Role, Origin: layer.Origin,
			Ord: layer.Ord, Content: "", ContentType: layerRevisionContentTypeYAML,
		}
		if revision, err := editor.resolveLayerContentRevision(ctx, membership.TenantID, assetID, layer); err == nil {
			if contentText, err := editor.blobContent(ctx, revision.ContentRef); err == nil {
				record.Content = contentText
			}
			record.ContentType = revision.ContentType
			record.EffectiveRevision = new(LayerRevisionRecord)
			*record.EffectiveRevision = revision
		}
		records = append(records, record)
	}
	return records, nil
}

// resolveLayerContentRevision 解析某层当前生效的内容修订：优先取全局作用域，
// 未命中时回退到该资产 default 分支的 ref 作用域（仓库同步 base 层即 ref 作用域）。
func (editor *LayerEdit) resolveLayerContentRevision(ctx context.Context, tenantID, assetID uuid.UUID, layer LayerRecord) (LayerRevisionRecord, error) {
	if revision, err := editor.effectiveRevision(ctx, tenantID, layer, overlayScopeGlobal, overlayScopeKeyGlobal); err == nil {
		return revision, nil
	}
	defaultBranch, err := editor.store.GetAssetRepositoryDefaultBranch(ctx, tenantID, assetID)
	if err != nil {
		return LayerRevisionRecord{}, err
	}
	return editor.effectiveRevision(ctx, tenantID, layer, overlayScopeRef, overlayRefTypeBranch+mergeScopeRefSelectorPrefix+defaultBranch)
}

// DraftLayerRecord 是一次内联草稿层的输入。
type DraftLayerRecord struct {
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源。
	Origin string
	// Ord 是 overlay 层排序号。
	Ord int
	// Dialect 是层方言（可为空）。
	Dialect *string
	// Content 是草稿层内容。
	Content string
	// ContentType 是内容媒体类型。
	ContentType string
}

// PreviewAssetViewInput 承载一次资产视图预览请求。
type PreviewAssetViewInput struct {
	// EnabledLayerIDs 是前端启用的层 ID 列表。
	EnabledLayerIDs []uuid.UUID
	// DraftLayers 是内联草稿层列表（可为空）。
	DraftLayers []DraftLayerRecord
	// ContentType 是合并输出的媒体类型。
	ContentType string
}

// PreviewAssetViewResult 是视图预览的合并结果。
type PreviewAssetViewResult struct {
	// EngineVersion 是合并引擎版本。
	EngineVersion string
	// MediaType 是合并输出媒体类型。
	MediaType string
	// MergedContent 是合并后的文档内容。
	MergedContent string
	// Provenance 是 pointer → 最后写入层的溯源。
	Provenance map[string]any
}

// PreviewAssetView 使用真实合并引擎对启用的层与内联草稿层做不落库合并。
func (editor *LayerEdit) PreviewAssetView(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID, input PreviewAssetViewInput) (PreviewAssetViewResult, error) {
	membership, err := editor.tenantMembership(ctx, actor, tenantSlug, scopeLayerRead)
	if err != nil {
		return PreviewAssetViewResult{}, err
	}
	if _, err := editor.store.GetAsset(ctx, membership.TenantID, assetID); err != nil {
		return PreviewAssetViewResult{}, err
	}
	layers, err := editor.store.ListLayersForAsset(ctx, membership.TenantID, assetID)
	if err != nil {
		return PreviewAssetViewResult{}, err
	}
	enabled := make(map[uuid.UUID]bool, len(input.EnabledLayerIDs))
	for _, id := range input.EnabledLayerIDs {
		enabled[id] = true
	}

	content := make(map[string]any)
	provenance := make(map[string]any)

	// base 层恒参与合并。
	selected := make([]LayerRecord, 0, len(layers))
	for _, layer := range layers {
		if layer.Role == layerRoleBase || enabled[layer.ID] {
			selected = append(selected, layer)
		}
	}
	slices.SortFunc(selected, func(a, b LayerRecord) int {
		if a.Role == layerRoleBase {
			return -1
		}
		if b.Role == layerRoleBase {
			return 1
		}
		return a.Ord - b.Ord
	})

	for _, layer := range selected {
		revision, err := editor.resolveLayerContentRevision(ctx, membership.TenantID, assetID, layer)
		if err != nil {
			return PreviewAssetViewResult{}, err
		}
		layerContent, err := editor.blobContent(ctx, revision.ContentRef)
		if err != nil {
			return PreviewAssetViewResult{}, err
		}
		if err := editor.applyLayerContent(layer, revision, layerContent, content, provenance); err != nil {
			return PreviewAssetViewResult{}, err
		}
	}
	// 内联草稿层按 role 追加：base 草稿整体替换，overlay 草稿应用 overlay 合并。
	for _, draft := range input.DraftLayers {
		revision := LayerRevisionRecord{ContentType: draft.ContentType}
		layer := LayerRecord{Role: draft.Role, Origin: draft.Origin, Ord: draft.Ord}
		if err := editor.applyLayerContent(layer, revision, draft.Content, content, provenance); err != nil {
			return PreviewAssetViewResult{}, err
		}
	}

	mergedBytes, err := CanonicalJSON(content)
	if err != nil {
		return PreviewAssetViewResult{}, err
	}
	mediaType := input.ContentType
	if mediaType == "" {
		mediaType = layerRevisionContentTypeYAML
	}
	return PreviewAssetViewResult{
		EngineVersion: mergeEngineVersion, MediaType: mediaType,
		MergedContent: string(mergedBytes), Provenance: provenance,
	}, nil
}

// applyLayerContent 将单个层（含草稿）的文档或 overlay 合并进累积文档。
func (editor *LayerEdit) applyLayerContent(layer LayerRecord, revision LayerRevisionRecord, contentText string, content map[string]any, provenance map[string]any) error {
	if layer.Role == layerRoleBase {
		decoded, err := decodeBaseDocument(contentText)
		if err != nil {
			return err
		}
		for key := range content {
			delete(content, key)
		}
		for key, value := range decoded {
			content[key] = value
		}
		provenance[""] = layer.ID.String()
		return nil
	}
	raw, err := ParseOverlay([]byte(contentText))
	if err != nil {
		return err
	}
	merged, pointers, _, err := ApplyPlatformOverlay(content, raw)
	if err != nil {
		return err
	}
	for key := range content {
		delete(content, key)
	}
	for key, value := range merged {
		content[key] = value
	}
	for _, pointer := range pointers {
		provenance[pointer] = layer.ID.String()
	}
	return nil
}
