package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/generated/api/diff"
	"github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// Search 运行跨类别租户搜索工作流。
func (s *Server) Search(ctx context.Context, request diff.SearchRequestObject) (diff.SearchResponseObject, error) {
	if s.searchService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	filter := service.SearchFilter{}
	if request.Params.Filter != nil {
		filter = searchFilterInput(*request.Params.Filter)
	}
	result, err := s.searchService.Run(ctx, principal, string(request.TenantSlug), service.SearchInput{
		Query: string(request.Params.Q), Filter: filter, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	items := make([]api.SearchHit, 0, len(result.Items))
	for _, hit := range result.Items {
		items = append(items, searchHitResponse(hit))
	}
	return diff.Search200JSONResponse(api.SearchResult{
		Total: result.Total, Page: result.Page, PageSize: result.PageSize, Items: items, Facets: searchFacetSetResponse(result.Facets),
	}), nil
}

// CreateSystemGroup 创建一个系统分组。
func (s *Server) CreateSystemGroup(ctx context.Context, request view.CreateSystemGroupRequestObject) (view.CreateSystemGroupResponseObject, error) {
	if s.systemGroups == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.systemGroups.CreateSystemGroup(ctx, principal, string(request.TenantSlug), service.SystemGroupCreateInput{
		Slug: string(request.Body.Slug), DisplayName: request.Body.DisplayName,
		Description: nullableStringValue(request.Body.Description), ServiceIDs: apiUUIDs(request.Body.ServiceIds),
	})
	if err != nil {
		return nil, err
	}
	body := systemGroupResponse(record)
	return view.CreateSystemGroup201JSONResponse{
		Body: body, Headers: view.CreateSystemGroup201ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// PutSystemGroupMembers replaces one group's members.
func (s *Server) PutSystemGroupMembers(ctx context.Context, request view.PutSystemGroupMembersRequestObject) (view.PutSystemGroupMembersResponseObject, error) {
	if s.systemGroups == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.systemGroups.PutSystemGroupMembers(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.GroupId),
		request.Params.IfMatch, apiUUIDs(&request.Body.ServiceIds),
	)
	if err != nil {
		return nil, err
	}
	body := systemGroupResponse(record)
	return view.PutSystemGroupMembers200JSONResponse{
		Body: body, Headers: view.PutSystemGroupMembers200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

func searchFilterInput(filter api.SearchFilter) service.SearchFilter {
	out := service.SearchFilter{}
	if filter.RepositoryIds != nil {
		out.RepositoryIDs = apiUUIDs(filter.RepositoryIds)
	}
	if filter.ServiceIds != nil {
		out.ServiceIDs = apiUUIDs(filter.ServiceIds)
	}
	if filter.GroupIds != nil {
		out.GroupIDs = apiUUIDs(filter.GroupIds)
	}
	if filter.Kinds != nil {
		for _, kind := range *filter.Kinds {
			out.Kinds = append(out.Kinds, string(kind))
		}
	}
	if filter.ItemTypes != nil {
		out.ItemTypes = append(out.ItemTypes, *filter.ItemTypes...)
	}
	if filter.Languages != nil {
		out.Languages = append(out.Languages, *filter.Languages...)
	}
	if filter.Lifecycles != nil {
		for _, lifecycle := range *filter.Lifecycles {
			out.Lifecycles = append(out.Lifecycles, string(lifecycle))
		}
	}
	if filter.HasAiLayer != nil {
		out.HasAiLayer = filter.HasAiLayer
	}
	if filter.HasBreakingChanges != nil {
		out.HasBreakingChanges = filter.HasBreakingChanges
	}
	return out
}

func searchHitResponse(hit service.SearchHitRecord) api.SearchHit {
	repository := nullable.NewNullNullable[api.RepositoryRef]()
	repository.Set(api.RepositoryRef{Id: api.Uuid(hit.Repository.ID), DefaultBranch: api.RefName(hit.Repository.DefaultBranch)})
	serviceRef := nullable.NewNullNullable[api.ServiceRef]()
	if hit.Service != nil {
		serviceRef.Set(api.ServiceRef{Id: api.Uuid(hit.Service.ID), Slug: api.Slug(hit.Service.Slug), DisplayName: hit.Service.DisplayName})
	}
	kind := nullable.NewNullNullable[api.KindId]()
	if hit.Kind != "" {
		kind.Set(api.KindId(hit.Kind))
	}
	deepLink := nullable.NewNullNullable[api.SearchDeepLink]()
	if hit.DeepLink != nil {
		itemKey := nullable.NewNullNullable[string]()
		if hit.DeepLink.ItemKey != nil {
			itemKey.Set(*hit.DeepLink.ItemKey)
		}
		deepLink.Set(api.SearchDeepLink{AssetId: api.Uuid(hit.DeepLink.AssetID), VersionId: api.Uuid(hit.DeepLink.VersionID), ItemKey: itemKey})
	}
	subtitle := nullable.NewNullNullable[string]()
	if hit.Subtitle != nil {
		subtitle.Set(*hit.Subtitle)
	}
	highlights := map[string]any{}
	for key, values := range hit.Highlights {
		highlights[key] = append([]string{}, values...)
	}
	return api.SearchHit{
		Type: api.SearchHitType(hit.Type), Id: hit.ID, Title: hit.Title, Subtitle: subtitle, Score: hit.Score,
		Repository: repository, Service: serviceRef, Kind: kind, DeepLink: deepLink, Highlights: highlights,
	}
}

func searchFacetSetResponse(facets service.SearchFacetSet) api.SearchFacetSet {
	return api.SearchFacetSet{
		Repositories: searchBuckets(facets.Repositories), Teams: searchBuckets(facets.Teams), Groups: searchBuckets(facets.Groups),
		Kinds: searchBuckets(facets.Kinds), Lifecycles: searchBuckets(facets.Lifecycles), Tags: searchBuckets(facets.Tags),
		Languages: searchBuckets(facets.Languages), ItemTypes: searchBuckets(facets.ItemTypes),
		HasAiLayer: searchBuckets(facets.HasAiLayer), HasBreakingChanges: searchBuckets(facets.HasBreakingChanges),
	}
}

func searchBuckets(buckets []service.SearchFacetBucket) []api.SearchFacetBucket {
	out := make([]api.SearchFacetBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, api.SearchFacetBucket{Value: bucket.Value, Count: bucket.Count})
	}
	return out
}

func systemGroupResponse(record service.SystemGroupRecord) api.SystemGroup {
	description := nullable.NewNullNullable[string]()
	if record.Description != nil {
		description.Set(*record.Description)
	}
	serviceIDs := make([]api.Uuid, 0, len(record.ServiceIDs))
	for _, id := range record.ServiceIDs {
		serviceIDs = append(serviceIDs, api.Uuid(id))
	}
	return api.SystemGroup{
		Id: api.Uuid(record.ID), Etag: revisionETag("system-group", record.ID.String(), record.Revision),
		Slug: api.Slug(record.Slug), DisplayName: record.DisplayName, Description: description,
		ServiceIds: serviceIDs, Capabilities: api.CapabilityList{}, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
