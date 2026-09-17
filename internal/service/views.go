package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"uuid"
)

// ViewDefinition is the built-in view registry entry projected to the API.
type ViewDefinition struct {
	ID             string
	NameKey        string
	Milestone      string
	Mount          string
	Component      *string
	Entrypoint     *string
	ExternalURL    *string
	InputMode      string
	InputKinds     []string
	InputMinDocs   *int
	InputMaxDocs   *int
	InputSameKind  bool
	InputSameAsset bool
	InputScopes    []string
	ItemTypes      []string
	DefaultOptions map[string]any
	OptionsSchema  map[string]any
	Columns        []map[string]any
	ColumnsSource  *string
	FallbackColumns []map[string]any
}

// ErrViewInputMismatch indicates the request arity or kind violates the view contract.
var ErrViewInputMismatch = errors.New("view input does not satisfy the view contract")

// ErrBranchNotIndexed indicates the selected ref has no indexed version.
var ErrBranchNotIndexed = errors.New("the selected branch is not indexed")

// Views validates and resolves view descriptors against the built-in registry.
type Views struct {
	store      AssetStore
	identities IdentityStore
}

// NewViews constructs view resolution use cases.
func NewViews(store AssetStore, identities IdentityStore) *Views {
	return &Views{store: store, identities: identities}
}

// Registry returns the M1 built-in view definitions declared by views.yaml.
func (views *Views) Registry() []ViewDefinition {
	return []ViewDefinition{
		{ID: "swagger-ui", NameKey: "view.swaggerUi", Milestone: "M1", Mount: "iframe", Entrypoint: stringPtr("/viewer-assets/swagger-ui/index.html"), InputMode: "single", InputKinds: []string{"openapi"}, DefaultOptions: map[string]any{"tryItOut": false}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "redoc", NameKey: "view.redoc", Milestone: "M1", Mount: "iframe", Entrypoint: stringPtr("/viewer-assets/redoc/index.html"), InputMode: "single", InputKinds: []string{"openapi"}, DefaultOptions: map[string]any{}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "source", NameKey: "view.source", Milestone: "M1", Mount: "component", Component: stringPtr("SourceView"), InputMode: "single", InputKinds: []string{"*"}, DefaultOptions: map[string]any{"layerAnnotations": false}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "operations", NameKey: "view.operations", Milestone: "M1", Mount: "component", Component: stringPtr("ItemsTableView"), InputMode: "single", InputKinds: []string{"openapi"}, ItemTypes: []string{"operation"}, DefaultOptions: map[string]any{"pageSize": 50}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
		{ID: "items-table", NameKey: "view.itemsTable", Milestone: "M1", Mount: "component", Component: stringPtr("ItemsTableView"), InputMode: "single", InputKinds: []string{"*"}, DefaultOptions: map[string]any{"pageSize": 50}, OptionsSchema: map[string]any{"type": "object", "additionalProperties": false}},
	}
}

// ViewResolution describes one resolved view output.
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

// DocumentResolution carries a resolved document descriptor.
type DocumentResolution struct {
	VersionID uuid.UUID
	ContentRef string
	MediaType string
	ExpiresAt string
	URL       string
}

// GraphNode is one resolved dependency graph node.
type GraphNode struct {
	ID string `json:"id"`
}

// GraphEdge is one resolved dependency graph edge.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Resolve validates a resolve request against the registry and materializes
// the declared output kind.
func (views *Views) Resolve(ctx context.Context, actor Principal, tenantSlug string, viewID string, versionIDs []uuid.UUID, scope *ScopeSelector, options map[string]any) (ViewResolution, error) {
	membership, err := views.tenantMembership(ctx, actor, tenantSlug, "asset:read")
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
	case "single":
		if len(versionIDs) != 1 {
			return ViewResolution{}, ErrViewInputMismatch
		}
		version, err := views.store.GetAssetVersion(ctx, membership.TenantID, versionIDs[0])
		if err != nil {
			return ViewResolution{}, err
		}
		if !version.IndexComplete && definition.ID == "operations" {
			return ViewResolution{}, ErrBranchNotIndexed
		}
		resolution.Kind = "document"
		resolution.Document = &DocumentResolution{VersionID: version.ID, ContentRef: "", MediaType: "application/yaml"}
	case "versions", "collection":
		if len(versionIDs) == 0 {
			return ViewResolution{}, ErrViewInputMismatch
		}
		resolution.Kind = "items"
		for _, id := range versionIDs {
			items, _, err := views.store.ListAssetVersionItems(ctx, membership.TenantID, id, "", 100, 0)
			if err != nil {
				return ViewResolution{}, err
			}
			resolution.Items = append(resolution.Items, items...)
		}
	case "scope":
		resolution.Kind = "dashboard"
		resolution.Metrics = map[string]any{}
		if slices.Contains(definition.InputScopes, "system_group") || slices.Contains(definition.InputScopes, "tenant") {
			resolution.Kind = "graph"
		}
	default:
		return ViewResolution{}, ErrViewInputMismatch
	}
	return resolution, nil
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
	case "single":
		if len(versionIDs) != 1 || scope != nil {
			return ErrViewInputMismatch
		}
	case "versions":
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
	case "collection":
		if len(versionIDs) == 0 || scope != nil {
			return ErrViewInputMismatch
		}
	case "scope":
		if len(versionIDs) != 0 || scope == nil {
			return ErrViewInputMismatch
		}
	}
	return nil
}

// ScopeSelector identifies a scope view's target.
type ScopeSelector struct {
	Type string
	ID   *uuid.UUID
}

func (views *Views) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, "*")) {
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
