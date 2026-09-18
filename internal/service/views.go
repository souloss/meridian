package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"uuid"
)

// ViewDefinition 是投影到 API 的内置视图注册表条目。
type ViewDefinition struct {
	ID              string
	NameKey         string
	Milestone       string
	Mount           string
	Component       *string
	Entrypoint      *string
	ExternalURL     *string
	InputMode       string
	InputKinds      []string
	InputMinDocs    *int
	InputMaxDocs    *int
	InputSameKind   bool
	InputSameAsset  bool
	InputScopes     []string
	ItemTypes       []string
	DefaultOptions  map[string]any
	OptionsSchema   map[string]any
	Columns         []map[string]any
	ColumnsSource   *string
	FallbackColumns []map[string]any
}

// ErrViewInputMismatch 表示请求元数或类别违反视图契约。
// 对外映射：ErrorCodeInputSpecMismatch（HTTP 422）。
var ErrViewInputMismatch = errors.New("view input does not satisfy the view contract")

// ErrBranchNotIndexed 表示所选引用尚无已索引版本。
// 对外映射：ErrorCodeBranchNotIndexed（HTTP 422）。
var ErrBranchNotIndexed = errors.New("the selected branch is not indexed")

// Views 依据内置注册表校验并解析视图描述符。
type Views struct {
	store      AssetStore
	identities IdentityStore
}

// NewViews 构造视图解析用例。
func NewViews(store AssetStore, identities IdentityStore) *Views {
	return &Views{store: store, identities: identities}
}

// Registry 返回 views.yaml 声明的 M1 内置视图定义。
func (views *Views) Registry() []ViewDefinition {
	return []ViewDefinition{
		{ID: "swagger-ui", NameKey: "view.swaggerUi", Milestone: "M1", Mount: "iframe", Entrypoint: stringPtr("/viewer-assets/swagger-ui/index.html"), InputMode: viewInputModeSingle, InputKinds: []string{"openapi"}, DefaultOptions: map[string]any{"tryItOut": false}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "redoc", NameKey: "view.redoc", Milestone: "M1", Mount: "iframe", Entrypoint: stringPtr("/viewer-assets/redoc/index.html"), InputMode: viewInputModeSingle, InputKinds: []string{"openapi"}, DefaultOptions: map[string]any{}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "source", NameKey: "view.source", Milestone: "M1", Mount: "component", Component: stringPtr("SourceView"), InputMode: viewInputModeSingle, InputKinds: []string{scopeWildcard}, DefaultOptions: map[string]any{"layerAnnotations": false}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "operations", NameKey: "view.operations", Milestone: "M1", Mount: "component", Component: stringPtr("ItemsTableView"), InputMode: viewInputModeSingle, InputKinds: []string{"openapi"}, ItemTypes: []string{"operation"}, DefaultOptions: map[string]any{"pageSize": 50}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "items-table", NameKey: "view.itemsTable", Milestone: "M1", Mount: "component", Component: stringPtr("ItemsTableView"), InputMode: viewInputModeSingle, InputKinds: []string{scopeWildcard}, DefaultOptions: map[string]any{"pageSize": 50}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "dep-graph", NameKey: "view.dependencyGraph", Milestone: "M4", Mount: "component", Component: stringPtr("DependencyGraphView"), InputMode: viewInputModeScope, InputKinds: []string{kindDependency}, InputScopes: []string{viewScopeTypeSystemGroup, viewScopeTypeTenant}, DefaultOptions: map[string]any{"layout": "hierarchical", "focusDepth": 1}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
	}
}

// ViewResolution 描述一次已解析的视图输出。
type ViewResolution struct {
	Kind      string
	View      ViewDefinition
	Document  *DocumentResolution
	Items     []AssetItemRecord
	Nodes     []GraphNode
	Edges     []GraphEdge
	Metrics   map[string]any
	Truncated bool
}

// DocumentResolution 承载一个已解析文档描述符。
type DocumentResolution struct {
	VersionID  uuid.UUID
	ContentRef string
	MediaType  string
	ExpiresAt  string
	URL        string
}

// GraphNode 是一个已解析的依赖图节点。
type GraphNode struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	ServiceID uuid.UUID `json:"serviceId"`
}

// GraphEdge 是一条已解析的依赖图边。
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Resolve 依据注册表校验 resolve 请求并物化声明的输出类别。对于 M4 dep-graph 作用域视图，
// 它解析所选系统分组的成员服务及其依赖边。
func (views *Views) Resolve(ctx context.Context, actor Principal, tenantSlug string, viewID string, versionIDs []uuid.UUID, scope *ScopeSelector, options map[string]any) (ViewResolution, error) {
	membership, err := views.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return ViewResolution{}, err
	}
	definition, ok := views.findView(viewID)
	if !ok {
		return ViewResolution{}, ErrNotFound
	}
	if err := views.validateArity(definition, versionIDs, scope); err != nil {
		return ViewResolution{}, err
	}
	resolution := ViewResolution{View: definition}

	switch definition.InputMode {
	case viewInputModeSingle:
		if len(versionIDs) != 1 {
			return ViewResolution{}, ErrViewInputMismatch
		}
		version, err := views.store.GetAssetVersion(ctx, membership.TenantID, versionIDs[0])
		if err != nil {
			return ViewResolution{}, err
		}
		if !version.IndexComplete && definition.ID == viewIDOperations {
			return ViewResolution{}, ErrBranchNotIndexed
		}
		resolution.Kind = viewResolutionKindDocument
		resolution.Document = &DocumentResolution{VersionID: version.ID, ContentRef: "", MediaType: contentTypeYAML}
	case viewInputModeVersions, viewInputModeCollection:
		if len(versionIDs) == 0 {
			return ViewResolution{}, ErrViewInputMismatch
		}
		resolution.Kind = viewResolutionKindItems
		for _, id := range versionIDs {
			items, _, err := views.store.ListAssetVersionItems(ctx, membership.TenantID, id, "", itemsFetchBatchSize, 0)
			if err != nil {
				return ViewResolution{}, err
			}
			resolution.Items = append(resolution.Items, items...)
		}
	case viewInputModeScope:
		resolution.Kind = viewResolutionKindDashboard
		resolution.Metrics = map[string]any{}
		if slices.Contains(definition.InputScopes, viewScopeTypeSystemGroup) || slices.Contains(definition.InputScopes, viewScopeTypeTenant) {
			nodes, edges, err := views.resolveGraph(ctx, membership.TenantID, definition.ID, scope)
			if err != nil {
				return ViewResolution{}, err
			}
			resolution.Kind = ViewKindDepGraph
			resolution.Nodes = nodes
			resolution.Edges = edges
			resolution.Truncated = false
		}
	default:
		return ViewResolution{}, ErrViewInputMismatch
	}
	return resolution, nil
}

// resolveGraph 将 dep-graph 作用域视图解析到其成员服务与依赖边。当作用域选择系统分组时，
// 仅纳入该分组成员；租户作用域则纳入租户内每个服务。
func (views *Views) resolveGraph(ctx context.Context, tenantID uuid.UUID, viewID string, scope *ScopeSelector) ([]GraphNode, []GraphEdge, error) {
	if viewID != viewIDDepGraph {
		return nil, nil, nil
	}
	var serviceIDs []uuid.UUID
	if scope != nil && scope.Type == viewScopeTypeSystemGroup && scope.ID != nil {
		groupID := *scope.ID
		groupMembers, err := views.store.ListSystemGroupMembers(ctx, tenantID, groupID)
		if err != nil {
			return nil, nil, err
		}
		serviceIDs = groupMembers
	} else {
		services, _, err := views.store.ListServicesForTenant(ctx, tenantID, 1000, 0)
		if err != nil {
			return nil, nil, err
		}
		for _, service := range services {
			serviceIDs = append(serviceIDs, service.ID)
		}
	}
	nodes := make([]GraphNode, 0, len(serviceIDs))
	byID := make(map[uuid.UUID]ServiceRecord, len(serviceIDs))
	for _, serviceID := range serviceIDs {
		service, err := views.store.GetServiceByID(ctx, tenantID, serviceID)
		if err != nil {
			continue
		}
		nodes = append(nodes, GraphNode{ID: service.Slug, Label: service.DisplayName, ServiceID: service.ID})
		byID[service.ID] = service
	}
	edges := make([]GraphEdge, 0)
	for _, serviceID := range serviceIDs {
		service := byID[serviceID]
		_ = service
		targets, err := views.dependencyTargets(ctx, tenantID, serviceID)
		if err != nil {
			continue
		}
		for _, target := range targets {
			edges = append(edges, GraphEdge{Source: byID[serviceID].Slug, Target: target})
		}
	}
	return nodes, edges, nil
}

// dependencyTargets 返回资产依赖边所指向的服务 slug，从服务的 dependency 类别资产条目解析而来。
func (views *Views) dependencyTargets(ctx context.Context, tenantID, serviceID uuid.UUID) ([]string, error) {
	assets, err := views.store.ListAssetsForService(ctx, tenantID, serviceID)
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0)
	for _, asset := range assets {
		if asset.Kind != kindDependency {
			continue
		}
		if defaultBranch, err := views.store.GetAssetRepositoryDefaultBranch(ctx, tenantID, asset.ID); err == nil {
			if track, trackErr := views.store.GetAssetRefTrack(ctx, tenantID, asset.ID, refTypeBranch, defaultBranch); trackErr == nil && track.CurrentVersionID != nil {
				items, _, itemsErr := views.store.ListAssetVersionItems(ctx, tenantID, *track.CurrentVersionID, "", itemsFetchBatchSize, 0)
				if itemsErr != nil {
					continue
				}
				for _, item := range items {
					if item.ItemType != itemTypeEdge {
						continue
					}
					to, _ := item.Display["to"].(string)
					if to != "" {
						targets = append(targets, to)
					}
				}
			}
		}
	}
	return targets, nil
}

func (views *Views) findView(id string) (ViewDefinition, bool) {
	for _, definition := range views.Registry() {
		if definition.ID == id {
			return definition, true
		}
	}
	return ViewDefinition{}, false
}

func (views *Views) validateArity(definition ViewDefinition, versionIDs []uuid.UUID, scope *ScopeSelector) error {
	switch definition.InputMode {
	case viewInputModeSingle:
		if len(versionIDs) != 1 || scope != nil {
			return ErrViewInputMismatch
		}
	case viewInputModeVersions:
		min, max := 2, 2
		if definition.InputMinDocs != nil {
			min = *definition.InputMinDocs
		}
		if definition.InputMaxDocs != nil {
			max = *definition.InputMaxDocs
		}
		if len(versionIDs) < min || len(versionIDs) > max || scope != nil {
			return ErrViewInputMismatch
		}
	case viewInputModeCollection:
		if len(versionIDs) == 0 || scope != nil {
			return ErrViewInputMismatch
		}
	case viewInputModeScope:
		if len(versionIDs) != 0 || scope == nil {
			return ErrViewInputMismatch
		}
	}
	return nil
}

// ScopeSelector 标识作用域视图的目标。
type ScopeSelector struct {
	Type string
	ID   *uuid.UUID
}

func (views *Views) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || views.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := views.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func stringPtr(value string) *string {
	return &value
}

var _ = strings.TrimSpace
