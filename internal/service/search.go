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
	// 批量预取命中条目的服务与仓库，避免逐命中 GetServiceByID/GetRepositoryByService 的 N+1 往返。
	serviceRefs := search.serviceRefs(ctx, membership.TenantID, items)
	repositoryRefs := search.repositoryRefs(ctx, membership.TenantID, serviceRefs)
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
		if serviceRef, ok := serviceRefs[item.ServiceID]; ok {
			hit.Service = &serviceRef
			if repository, repoOK := repositoryRefs[item.ServiceID]; repoOK {
				hit.Repository = repository
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

// serviceRefs 一次查询返回全部命中条目所属服务的引用，按服务 ID 索引。
func (search *Search) serviceRefs(ctx context.Context, tenantID uuid.UUID, items []AssetItemRecord) map[uuid.UUID]ServiceRef {
	serviceIDs := make([]uuid.UUID, 0, len(items))
	seen := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if !seen[item.ServiceID] {
			seen[item.ServiceID] = true
			serviceIDs = append(serviceIDs, item.ServiceID)
		}
	}
	services, err := search.store.ListServicesByIDs(ctx, tenantID, serviceIDs)
	if err != nil {
		return map[uuid.UUID]ServiceRef{}
	}
	refs := make(map[uuid.UUID]ServiceRef, len(services))
	for _, service := range services {
		refs[service.ID] = ServiceRef{ID: service.ID, Slug: service.Slug, DisplayName: service.DisplayName}
	}
	return refs
}

// repositoryRefs 一次查询返回全部命中服务所属仓库的引用，按服务 ID 索引。
func (search *Search) repositoryRefs(ctx context.Context, tenantID uuid.UUID, serviceRefs map[uuid.UUID]ServiceRef) map[uuid.UUID]RepositoryRef {
	serviceIDs := make([]uuid.UUID, 0, len(serviceRefs))
	for serviceID := range serviceRefs {
		serviceIDs = append(serviceIDs, serviceID)
	}
	repositories, err := search.store.ListRepositoriesByServices(ctx, tenantID, serviceIDs)
	if err != nil {
		return map[uuid.UUID]RepositoryRef{}
	}
	refs := make(map[uuid.UUID]RepositoryRef, len(repositories))
	for serviceID, repository := range repositories {
		refs[serviceID] = RepositoryRef{ID: repository.ID, DefaultBranch: repository.DefaultBranch}
	}
	return refs
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
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, search.identities)
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
