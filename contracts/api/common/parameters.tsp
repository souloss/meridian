/** Reusable HTTP path, query, and header parameter models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

namespace Parameters {
  model TenantSlug {
    /** identifies the tenant that scopes this request. */
    @path tenantSlug: MeridianApi.Slug;
  }
  model KindId {
    /** identifies or filters the kind id value in the path. */
    @path kindId: MeridianApi.KindId;
  }
  model IfMatch {
    /** carries the current ETag required for a conditional mutation. */
    @header `If-Match`: MeridianApi.ETag;
  }
  model ServiceSlug {
    /** identifies or filters the service slug value in the path. */
    @path serviceSlug: MeridianApi.Slug;
  }
  model SourceId {
    /** identifies or filters the source id value in the path. */
    @path sourceId: MeridianApi.Uuid;
  }
  model IdempotencyKey {
    /** Retained for 24 hours; a replay returns the original status and body. */
    @format("uuid") @header `Idempotency-Key`: string;
  }
  model AssetId {
    /** identifies or filters the asset id value in the path. */
    @path assetId: MeridianApi.Uuid;
  }
  model RefType {
    /** identifies or filters the ref type value in the query. */
    @query(#{ explode: true }) refType?: MeridianApi.RefType;
  }
  model RefName {
    /** identifies or filters the ref value in the query. */
    @query(#{ explode: true }) ref?: MeridianApi.RefName;
  }
  model Page {
    /** selects a one-based result page. */
    @minValue(1) @query(#{ explode: true }) page?: integer = 1;
  }
  model PageSize {
    /** limits the number of records returned on one page. */
    @minValue(1)
    @maxValue(100)
    @query(#{ explode: true })
    pageSize?: integer = 20;
  }
  model VersionId {
    /** identifies or filters the version id value in the path. */
    @path versionId: MeridianApi.Uuid;
  }
  model Query {
    /** identifies or filters the q value in the query. */
    @minLength(1)
    @maxLength(200)
    @query(#{ explode: true })
    q?: string;
  }
  model AssetName {
    /** identifies or filters the asset name value in the path. */
    @path assetName: MeridianApi.AssetName;
  }
  model ChannelId {
    /** identifies or filters the channel id value in the path. */
    @path channelId: MeridianApi.Uuid;
  }
  model UnreadFilter {
    /** identifies or filters the unread value in the query. */
    @query(#{ explode: true }) unread?: boolean;
  }
  model NotificationId {
    /** identifies or filters the notification id value in the path. */
    @path notificationId: MeridianApi.Uuid;
  }
  model TodoStatusFilter {
    /** identifies or filters the status value in the query. */
    @query(#{ explode: true }) status?: MeridianApi.TodoStatus;
  }
  model TodoId {
    /** identifies or filters the todo id value in the path. */
    @path todoId: MeridianApi.Uuid;
  }
  model AuditFilters {
    /** identifies or filters the filter value in the query. */
    @query(#{ explode: true }) filter?: MeridianApi.AuditFilter;
  }
  model SnapshotId {
    /** identifies or filters the snapshot id value in the path. */
    @path snapshotId: MeridianApi.Uuid;
  }
  model ExportFormat {
    /** identifies or filters the format value in the query. */
    @query(#{ explode: true }) format: "json" | "markdown";
  }
  model RuleSetId {
    /** identifies or filters the rule set id value in the path. */
    @path ruleSetId: MeridianApi.Uuid;
  }
  model RequiredQuery {
    /** identifies or filters the q value in the query. */
    @minLength(1)
    @maxLength(200)
    @query(#{ explode: true })
    q: string;
  }
  model SearchFacets {
    /** identifies or filters the filter value in the query. */
    @query(#{ explode: true }) filter?: MeridianApi.SearchFilter;
  }
  model JobFilters {
    /** identifies or filters the filter value in the query. */
    @query(#{ explode: true }) filter?: MeridianApi.JobFilter;
  }
  model JobId {
    /** identifies or filters the job id value in the path. */
    @path jobId: MeridianApi.Uuid;
  }
  model LastEventId {
    /** resumes an event stream strictly after this non-negative persisted log sequence. */
    @maxLength(19)
    @pattern("^(0|[1-9][0-9]*)$")
    @header
    `Last-Event-ID`?: string;
  }
  model LayerId {
    /** identifies or filters the layer id value in the path. */
    @path layerId: MeridianApi.Uuid;
  }
  model RevisionId {
    /** identifies or filters the revision id value in the path. */
    @path revisionId: MeridianApi.Uuid;
  }
  model ReviewFilters {
    /** identifies or filters the filter value in the query. */
    @query(#{ explode: true }) filter?: MeridianApi.ReviewFilter;
  }
  model UserId {
    /** identifies or filters the user id value in the path. */
    @path userId: MeridianApi.Uuid;
  }
  model CredentialId {
    /** identifies or filters the credential id value in the path. */
    @path credentialId: MeridianApi.Uuid;
  }
  model Force {
    /** identifies or filters the force value in the query. */
    @query(#{ explode: true }) force?: boolean;
  }
  model ProducerProfileId {
    /** identifies or filters the producer profile id value in the path. */
    @path producerProfileId: MeridianApi.Uuid;
  }
  model PlatformJobFilters {
    /** identifies or filters the filter value in the query. */
    @query(#{ explode: true }) filter?: MeridianApi.PlatformJobFilter;
  }
  model PlatformAuditFilters {
    /** identifies or filters the filter value in the query. */
    @query(#{ explode: true }) filter?: MeridianApi.PlatformAuditFilter;
  }
  model RepositoryId {
    /** identifies or filters the repository id value in the path. */
    @path repositoryId: MeridianApi.Uuid;
  }
  model CandidateId {
    /** identifies or filters the candidate id value in the path. */
    @path candidateId: MeridianApi.Uuid;
  }
  model PreviewId {
    /** identifies or filters the preview id value in the path. */
    @path previewId: MeridianApi.Uuid;
  }
  model WebhookDelivery {
    /** identifies or filters the x meridian delivery value in the header. */
    @minLength(1)
    @maxLength(128)
    @header
    `X-Meridian-Delivery`: string;
  }
  model WebhookEvent {
    /** identifies or filters the x meridian event value in the header. */
    @header `X-Meridian-Event`: "push";
  }
  model WebhookSignature {
    /** identifies or filters the x meridian signature 256 value in the header. */
    @pattern("^sha256=[0-9a-f]{64}$") @header `X-Meridian-Signature-256`: string;
  }
  model Sort {
    /** identifies or filters the sort value in the query. */
    @minLength(1)
    @maxLength(64)
    @query(#{ explode: true })
    sort?: string;
  }
  model ContentToken {
    /** is the signed capability authorizing one artifact download. */
    @minLength(32)
    @maxLength(2048)
    @path
    token: string;
  }
  model TokenId {
    /** identifies or filters the token id value in the path. */
    @path tokenId: MeridianApi.Uuid;
  }
  model TeamId {
    /** identifies or filters the team id value in the path. */
    @path teamId: MeridianApi.Uuid;
  }
  model TagId {
    /** identifies or filters the tag id value in the path. */
    @path tagId: MeridianApi.Uuid;
  }
  model ViewId {
    /** identifies or filters the view id value in the path. */
    @minLength(1)
    @maxLength(64)
    @path
    viewId: string;
  }
  model KindFilter {
    /** identifies or filters the kind value in the query. */
    @query(#{ explode: true }) kind?: MeridianApi.KindId;
  }
  model ShareLinkId {
    /** identifies or filters the share link id value in the path. */
    @path shareLinkId: MeridianApi.Uuid;
  }
  model ShareToken {
    /** is the opaque capability authorizing access to one shared view. */
    @minLength(32)
    @maxLength(512)
    @path
    shareToken: string;
  }
  model GroupId {
    /** identifies or filters the group id value in the path. */
    @path groupId: MeridianApi.Uuid;
  }
}
