package service

import (
	"context"
	"uuid"
)

// Search 协调跨类别的租户搜索工作流。
type Search struct {
	store      AssetStore
	groups     SystemGroupStore
	identities IdentityStore
}

// NewSearch 构造 M4 搜索用例。
func NewSearch(store AssetStore, groups SystemGroupStore, identities IdentityStore) *Search {
	return &Search{store: store, groups: groups, identities: identities}
}

// Run 对已索引资产条目执行一次租户搜索，应用 facet 过滤与租户隔离。
func (search *Search) Run(ctx context.Context, actor Principal, tenantSlug string, input SearchInput) (SearchResultRecord, error) {
	membership, err := search.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return SearchResultRecord{}, err
	}
	if input.Query == "" {
		return SearchResultRecord{}, ErrValidation
	}
	if err := validatePagination(input.Page, input.PageSize); err != nil {
		return SearchResultRecord{}, err
	}
	// 按分组过滤时将组成员解析为服务 id 集合。
	if len(input.Filter.GroupIDs) > 0 {
		grouped := make(map[uuid.UUID]bool)
		for _, groupID := range input.Filter.GroupIDs {
			members, memberErr := search.groups.ListSystemGroupMembers(ctx, membership.TenantID, groupID)
			if memberErr != nil {
				return SearchResultRecord{}, memberErr
			}
			for _, serviceID := range members {
				grouped[serviceID] = true
			}
		}
		input.Filter.ServiceIDs = append(input.Filter.ServiceIDs, mapKeys(grouped)...)
	}

	items, total, err := search.store.SearchItems(ctx, membership.TenantID, input.Query, input.Filter, int32(input.PageSize), int32((input.Page-1)*input.PageSize))
	if err != nil {
		return SearchResultRecord{}, err
	}
	hits := make([]SearchHitRecord, 0, len(items))
	for _, item := range items {
		hit := SearchHitRecord{
			Type: "item", ID: item.Key, Title: item.Key,
			Kind: item.Kind, Score: 1,
			DeepLink: &SearchDeepLink{
				AssetID: item.AssetID, VersionID: item.AssetVersionID,
				ItemKey: stringPtrOrNil(item.Key),
			},
			Highlights: search.highlights(item),
		}
		serviceRef, serviceErr := search.serviceRef(ctx, membership.TenantID, item.ServiceID)
		if serviceErr == nil {
			hit.Service = &serviceRef
			repo, repoErr := search.repositoryRef(ctx, membership.TenantID, serviceRef.ID)
			if repoErr == nil {
				hit.Repository = repo
			}
			hit.Subtitle = stringPtrOrNil(serviceRef.Slug)
		}
		hits = append(hits, hit)
	}
	facets := search.facets(ctx, membership.TenantID, input.Filter)
	return SearchResultRecord{
		Total: int(total), Page: input.Page, PageSize: input.PageSize, Items: hits, Facets: facets,
	}, nil
}

func (search *Search) serviceRef(ctx context.Context, tenantID, serviceID uuid.UUID) (ServiceRef, error) {
	service, err := search.store.GetServiceByID(ctx, tenantID, serviceID)
	if err != nil {
		return ServiceRef{}, err
	}
	return ServiceRef{ID: service.ID, Slug: service.Slug, DisplayName: service.DisplayName}, nil
}

func (search *Search) repositoryRef(ctx context.Context, tenantID, serviceID uuid.UUID) (RepositoryRef, error) {
	repository, err := search.store.GetRepositoryByService(ctx, tenantID, serviceID)
	if err != nil {
		return RepositoryRef{}, err
	}
	return RepositoryRef{ID: repository.ID, DefaultBranch: repository.DefaultBranch}, nil
}

func (search *Search) highlights(item AssetItemRecord) map[string][]string {
	highlights := map[string][]string{}
	for _, value := range item.Display {
		highlights["display"] = append(highlights["display"], stringValue(value))
	}
	if len(highlights["display"]) == 0 {
		highlights["display"] = []string{item.Key}
	}
	return highlights
}

func (search *Search) facets(ctx context.Context, tenantID uuid.UUID, filter SearchFilter) SearchFacetSet {
	facets := SearchFacetSet{
		Repositories: []SearchFacetBucket{}, Teams: []SearchFacetBucket{}, Groups: []SearchFacetBucket{},
		Kinds: []SearchFacetBucket{}, Lifecycles: []SearchFacetBucket{}, Tags: []SearchFacetBucket{},
		Languages: []SearchFacetBucket{}, ItemTypes: []SearchFacetBucket{},
		HasAiLayer: []SearchFacetBucket{}, HasBreakingChanges: []SearchFacetBucket{},
	}
	kinds, err := search.store.ListAssetKinds(ctx)
	if err != nil {
		return facets
	}
	for _, kind := range kinds {
		if kind.Enabled {
			facets.Kinds = append(facets.Kinds, SearchFacetBucket{Value: kind.ID, Count: 0})
		}
	}
	return facets
}

func (search *Search) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!containsString(actor.Scopes, permission) && !containsString(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || search.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := search.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func mapKeys(values map[uuid.UUID]bool) []uuid.UUID {
	keys := make([]uuid.UUID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}
