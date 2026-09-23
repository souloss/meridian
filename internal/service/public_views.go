package service

import (
	"context"
	"uuid"
)

// ResolvePublicView 解析匿名公开视图：按 slug 定位公开资产后，复用 Views.Resolve 输出。
func (views *Views) ResolvePublicView(ctx context.Context, tenantSlug, serviceSlug, kind, assetName, viewID string, options map[string]any) (ViewResolution, error) {
	asset, err := views.store.GetPublicAsset(ctx, tenantSlug, serviceSlug, kind, assetName)
	if err != nil {
		return ViewResolution{}, err
	}
	if asset.CurrentVersion == nil {
		return ViewResolution{}, ErrNotFound
	}
	// 公开视图仅允许注册表中三个文档视图之一（redoc/source/swagger-ui）。
	definition, ok := views.findView(viewID)
	if !ok {
		return ViewResolution{}, ErrNotFound
	}
	if definition.InputMode != viewInputModeSingle {
		return ViewResolution{}, ErrViewInputMismatch
	}
	resolution := ViewResolution{
		Kind: viewResolutionKindDocument,
		View: definition,
		Document: &DocumentResolution{
			VersionID:  asset.CurrentVersion.ID,
			ContentRef: asset.ContentRef,
			MediaType:  contentTypeYAML,
		},
	}
	return resolution, nil
}

var _ = uuid.Nil
