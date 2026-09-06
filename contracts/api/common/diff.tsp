/** Shared diff, search, and rule-set models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** defines validated input for the corresponding Meridian API operation. */
model UploadRequest {
  /** specifies the file associated with this upload request. */
  file: HttpPart<bytes>;

  /** contains the kind id associated with this upload request. */
  kind: HttpPart<KindId>;

  /** contains the content type associated with this upload request. */
  contentType: HttpPart<ContentType>;
}

/** represents upload data exchanged through the Meridian API. */
model Upload {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the kind id associated with this upload. */
  kind: KindId;

  /** contains the content type associated with this upload. */
  contentType: ContentType;

  /** specifies the digest associated with this upload. */
  digest: string;

  /** specifies the size bytes associated with this upload. */
  @minValue(1) @maxValue(10485760) sizeBytes: integer;

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  expiresAt: Timestamp;
}

/** identifies the version selected by a request. */
model VersionSelector {
  /** specifies the type associated with this version selector. */
  type: "version";

  /** identifies the asset version associated with this resource. */
  versionId: Uuid;
}

/** identifies the ref selected by a request. */
model RefSelector {
  /** specifies the type associated with this ref selector. */
  type: "ref";

  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** contains the ref type associated with this ref selector. */
  refType: RefType;

  /** contains the ref name associated with this ref selector. */
  ref: RefName;
}

/** identifies the upload selected by a request. */
model UploadSelector {
  /** specifies the type associated with this upload selector. */
  type: "upload";

  /** contains the uuid associated with this upload selector. */
  uploadId: Uuid;
}

/** identifies the document selected by a request. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "type" })
@oneOf
union DocumentSelector {
  version: VersionSelector,
  ref: RefSelector,
  upload: UploadSelector,
}

/** defines validated input for the corresponding Meridian API operation. */
model DiffCreateRequest {
  /** contains the document selector associated with this diff create request. */
  left: DocumentSelector;

  /** contains the document selector associated with this diff create request. */
  right: DocumentSelector;

  /** specifies the rule set id associated with this diff create request. */
  @oneOf ruleSetId?: Uuid | null;

  /** indicates whether persist applies to this diff create request. */
  persist?: boolean = true;
}

/** Immutable resolution. version requires assetId/versionId and upload requires uploadId; requestedRef fields are informational and never re-resolved. */
model ResolvedDocumentRef {
  /** specifies the source type associated with this resolved document ref. */
  sourceType: "version" | "upload";

  /** contains the kind id associated with this resolved document ref. */
  kind: KindId;

  /** specifies the content hash associated with this resolved document ref. */
  contentHash: string;

  /** identifies the asset associated with this resource. */
  @oneOf assetId: Uuid | null;

  /** identifies the asset version associated with this resource. */
  @oneOf versionId: Uuid | null;

  /** specifies the upload id associated with this resolved document ref. */
  @oneOf uploadId: Uuid | null;

  /** specifies the requested ref type associated with this resolved document ref. */
  @oneOf requestedRefType: RefType | null;

  /** specifies the requested ref associated with this resolved document ref. */
  @oneOf requestedRef: RefName | null;
}

/** identifies one supported diff level value. */
enum DiffLevel {
  breaking,
  risky,
  non_breaking,
  informational,
}

/** represents diff change data exchanged through the Meridian API. */
model DiffChange {
  /** uniquely identifies this resource. */
  id: string;

  /** contains the diff level associated with this diff change. */
  level: DiffLevel;

  /** specifies the code associated with this diff change. */
  code: string;

  /** specifies the path associated with this diff change. */
  path: string;

  /** specifies the summary associated with this diff change. */
  summary: string;

  /** specifies the before associated with this diff change. */
  before: unknown;

  /** specifies the after associated with this diff change. */
  after: unknown;
}

/** reports the result of the corresponding Meridian operation. */
model DiffResult {
  /** contains the kind id associated with this diff result. */
  kind: KindId;

  /** contains the resolved document ref associated with this diff result. */
  left: ResolvedDocumentRef;

  /** contains the resolved document ref associated with this diff result. */
  right: ResolvedDocumentRef;

  /** Non-null exactly when DiffCreateRequest.persist is true. */
  @oneOf snapshotId: Uuid | null;

  /** contains the diff counts associated with this diff result. */
  summary: DiffCounts;

  /** contains the ordered changes associated with this diff result. */
  changes: DiffChange[];

  /** contains the timestamp associated with this diff result. */
  generatedAt: Timestamp;
}

/** represents diff snapshot data exchanged through the Meridian API. */
model DiffSnapshot {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the diff result associated with this diff snapshot. */
  result: DiffResult;

  /** contains the uuid associated with this diff snapshot. */
  createdBy: Uuid;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of diff snapshot records. */
model DiffSnapshotPage {
  ...PageInfo;

  /** contains the ordered items associated with this diff snapshot page. */
  items: DiffSnapshot[];
}

/** represents artifact link data exchanged through the Meridian API. */
model ArtifactLink {
  /** specifies the url associated with this artifact link. */
  @format("uri-reference") url: string;

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  expiresAt: Timestamp;
}

/** defines validated input for the corresponding Meridian API operation. */
model DiffSnapshotShareRequest {
  /** specifies the expires in seconds associated with this diff snapshot share request. */
  @minValue(300) @maxValue(2592000) expiresInSeconds: integer;
}

/** represents share link created data exchanged through the Meridian API. */
model ShareLinkCreated {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is secret bearer material and must never be logged or persisted as plaintext. */
   @minLength(32) token: string;

  /** specifies the resource type associated with this share link created. */
  resourceType: "view" | "diff_snapshot";

  /** specifies the resource id associated with this share link created. */
  @oneOf resourceId: Uuid | null;

  /** specifies the descriptor digest associated with this share link created. */
  descriptorDigest: string;

  /** specifies the url associated with this share link created. */
  @format("uri-reference") url: string;

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  expiresAt: Timestamp;

  /** specifies the revoked at associated with this share link created. */
  @oneOf revokedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** represents diff rule data exchanged through the Meridian API. */
model DiffRule {
  /** specifies the code associated with this diff rule. */
  code: string;

  /** contains the diff level associated with this diff rule. */
  level: DiffLevel;

  /** indicates whether enabled applies to this diff rule. */
  enabled: boolean;
}

/** represents diff rule set data exchanged through the Meridian API. */
model DiffRuleSet {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this diff rule set. */
  name: string;

  /** contains the kind id associated with this diff rule set. */
  kind: KindId;

  /** specifies the version associated with this diff rule set. */
  @minValue(1) version: integer;

  /** contains the ordered rules associated with this diff rule set. */
  rules: DiffRule[];

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of diff rule set values. */
model DiffRuleSetList {
  /** contains the ordered items associated with this diff rule set list. */
  items: DiffRuleSet[];
}

/** defines validated input for the corresponding Meridian API operation. */
model DiffRuleSetCreateRequest {
  /** specifies the name associated with this diff rule set create request. */
  @minLength(1) @maxLength(128) name: string;

  /** contains the kind id associated with this diff rule set create request. */
  kind: KindId;

  /** contains the ordered rules associated with this diff rule set create request. */
  rules: DiffRule[];
}

/** defines validated input for the corresponding Meridian API operation. */
model DiffRuleSetPatchRequest {
  /** specifies the name associated with this diff rule set patch request. */
  @minLength(1) @maxLength(128) name?: string;

  /** contains the ordered rules associated with this diff rule set patch request. */
  rules?: DiffRule[];
}

/** defines supported filters for selecting search records. */
model SearchFilter {
  /** contains the ordered repository ids associated with this search filter. */
  repositoryIds?: Uuid[];

  /** contains the ordered service ids associated with this search filter. */
  serviceIds?: Uuid[];

  /** contains the ordered team ids associated with this search filter. */
  teamIds?: Uuid[];

  /** contains the ordered group ids associated with this search filter. */
  groupIds?: Uuid[];

  /** contains the ordered kinds associated with this search filter. */
  kinds?: KindId[];

  /** contains the ordered lifecycles associated with this search filter. */
  lifecycles?: Lifecycle[];

  /** contains the ordered tag ids associated with this search filter. */
  tagIds?: Uuid[];

  /** contains the ordered languages associated with this search filter. */
  languages?: string[];

  /** contains the ordered item types associated with this search filter. */
  itemTypes?: string[];

  /** indicates whether ai layer applies to this search filter. */
  hasAiLayer?: boolean;

  /** indicates whether breaking changes applies to this search filter. */
  hasBreakingChanges?: boolean;
}

/** represents repository ref data exchanged through the Meridian API. */
model RepositoryRef {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the ref name associated with this repository ref. */
  defaultBranch: RefName;
}

/** represents service ref data exchanged through the Meridian API. */
model ServiceRef {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the slug associated with this service ref. */
  slug: Slug;

  /** specifies the display name associated with this service ref. */
  displayName: string;
}

/** represents search deep link data exchanged through the Meridian API. */
model SearchDeepLink {
  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** identifies the asset version associated with this resource. */
  versionId: Uuid;

  /** specifies the item key associated with this search deep link. */
  itemKey: string | null;
}

/** represents search hit data exchanged through the Meridian API. */
model SearchHit {
  /** specifies the type associated with this search hit. */
  type: "repository" | "service" | "asset" | "item";

  /** uniquely identifies this resource. */
  id: string;

  /** specifies the title associated with this search hit. */
  title: string;

  /** specifies the subtitle associated with this search hit. */
  subtitle: string | null;

  /** specifies the score associated with this search hit. */
  score: numeric;

  /** Non-null for every hit; repository hits use this as their owning resource. */
  @oneOf repository: RepositoryRef | null;

  /** Null only for repository hits. */
  @oneOf service: ServiceRef | null;

  /** specifies the kind associated with this search hit. */
  @oneOf kind: KindId | null;

  /** Required for asset and item hits; null for repository and service hits. */
  @oneOf deepLink: SearchDeepLink | null;

  /** specifies the highlights associated with this search hit. */
  highlights: Record<string[]>;
}

/** represents search facet bucket data exchanged through the Meridian API. */
model SearchFacetBucket {
  /** specifies the value associated with this search facet bucket. */
  value: string;

  /** specifies the count associated with this search facet bucket. */
  @minValue(0) count: integer;
}

/** represents search facet set data exchanged through the Meridian API. */
model SearchFacetSet {
  /** contains the ordered repositories associated with this search facet set. */
  repositories: SearchFacetBucket[];

  /** contains the ordered teams associated with this search facet set. */
  teams: SearchFacetBucket[];

  /** contains the ordered groups associated with this search facet set. */
  groups: SearchFacetBucket[];

  /** contains the ordered kinds associated with this search facet set. */
  kinds: SearchFacetBucket[];

  /** contains the ordered lifecycles associated with this search facet set. */
  lifecycles: SearchFacetBucket[];

  /** contains the ordered tags associated with this search facet set. */
  tags: SearchFacetBucket[];

  /** contains the ordered languages associated with this search facet set. */
  languages: SearchFacetBucket[];

  /** contains the ordered item types associated with this search facet set. */
  itemTypes: SearchFacetBucket[];

  /** indicates whether ai layer applies to this search facet set. */
  hasAiLayer: SearchFacetBucket[];

  /** indicates whether breaking changes applies to this search facet set. */
  hasBreakingChanges: SearchFacetBucket[];
}

/** reports the result of the corresponding Meridian operation. */
model SearchResult {
  ...PageInfo;

  /** contains the ordered items associated with this search result. */
  items: SearchHit[];

  /** contains the search facet set associated with this search result. */
  facets: SearchFacetSet;
}

/** identifies one supported job type value. */
