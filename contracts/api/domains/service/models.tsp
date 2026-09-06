/** Shared service, access, and configuration models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** identifies one supported service visibility value. */
enum ServiceVisibility {
  private,
  internal,
  public,
}

/** represents candidate override data exchanged through the Meridian API. */
model CandidateOverride {
  /** contains the uuid associated with this candidate override. */
  candidateId: Uuid;

  /** contains the slug associated with this candidate override. */
  slug?: Slug;

  /** specifies the display name associated with this candidate override. */
  @minLength(1) @maxLength(128) displayName?: string;

  /** contains the service visibility associated with this candidate override. */
  visibility?: ServiceVisibility;
}

/** defines validated input for the corresponding Meridian API operation. */
model CandidatesAcceptRequest {
  /** indicates whether candidate ids applies to this candidates accept request. */
  @minItems(1) @maxItems(100) candidateIds: Uuid[];

  /** contains the ordered overrides associated with this candidates accept request. */
  overrides?: CandidateOverride[];
}

/** represents owner refs data exchanged through the Meridian API. */
model OwnerRefs {
  /** contains the ordered user ids associated with this owner refs. */
  userIds: Uuid[];

  /** contains the ordered team ids associated with this owner refs. */
  teamIds: Uuid[];
}

/** represents tag data exchanged through the Meridian API. */
model Tag {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this tag. */
  @minLength(1) @maxLength(64) name: string;

  /** specifies the color associated with this tag. */
  @pattern("^#[0-9A-Fa-f]{6}$") color: string | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** represents asset summary data exchanged through the Meridian API. */
model AssetSummary {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the kind id associated with this asset summary. */
  kind: KindId;

  /** contains the asset name associated with this asset summary. */
  name: AssetName;

  /** of currentVersion, otherwise latestVersion, otherwise draft for the repository default Track. */
  lifecycle: Lifecycle;

  /** specifies the health associated with this asset summary. */
  health: "ok" | "stale" | "invalid";

  /** specifies the current version associated with this asset summary. */
  @oneOf currentVersion: VersionRef | null;

  /** specifies the latest version associated with this asset summary. */
  @oneOf latestVersion: VersionRef | null;

  /** contains the ref type associated with this asset summary. */
  refType: RefType;

  /** contains the ref name associated with this asset summary. */
  ref: RefName;
}

/** represents missing kind data exchanged through the Meridian API. */
model MissingKind {
  /** contains the kind id associated with this missing kind. */
  kind: KindId;

  /** indicates whether configure applies to this missing kind. */
  canConfigure: boolean;

  /** indicates whether generate with ai applies to this missing kind. */
  canGenerateWithAi: boolean;
}

/** represents drift info data exchanged through the Meridian API. */
model DriftInfo {
  /** indicates whether drift applies to this drift info. */
  hasDrift: boolean;

  /** contains the ordered fields associated with this drift info. */
  fields: FieldSource[];

  /** specifies the file commit associated with this drift info. */
  fileCommit: string | null;
}

/** represents service data exchanged through the Meridian API. */
model Service {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the slug associated with this service. */
  slug: Slug;

  /** specifies the display name associated with this service. */
  displayName: string;

  /** specifies the description associated with this service. */
  description: string | null;

  /** Empty string means repository root. */
  @maxLength(512) @format("repository-relative-path") rootDir: string;

  /** specifies the language associated with this service. */
  @maxLength(64) language: string | null;

  /** specifies the framework associated with this service. */
  @maxLength(64) framework: string | null;

  /** contains the service visibility associated with this service. */
  visibility: ServiceVisibility;

  /** contains the lifecycle associated with this service. */
  lifecycle: Lifecycle;

  /** contains the repository ref associated with this service. */
  repository: RepositoryRef;

  /** contains the owner refs associated with this service. */
  owners: OwnerRefs;

  /** contains the ordered tags associated with this service. */
  tags: Tag[];

  /** contains the ordered assets associated with this service. */
  assets: AssetSummary[];

  /** contains the ordered missing kinds associated with this service. */
  missingKinds: MissingKind[];

  /** contains the drift info associated with this service. */
  drift: DriftInfo;

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** indicates whether starred applies to this service. */
  starred: boolean;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of service values. */
model ServiceList {
  /** contains the ordered items associated with this service list. */
  items: Service[];
}

/** defines validated input for the corresponding Meridian API operation. */
model ConfigImportPreviewRequest {
  /** contains the ref type associated with this config import preview request. */
  refType?: RefType;

  /** contains the ref name associated with this config import preview request. */
  ref?: RefName;
}

/** represents config field diff data exchanged through the Meridian API. */
model ConfigFieldDiff {
  /** specifies the pointer associated with this config field diff. */
  pointer: string;

  /** specifies the database value associated with this config field diff. */
  databaseValue: unknown;

  /** specifies the file value associated with this config field diff. */
  fileValue: unknown;

  /** contains the config source associated with this config field diff. */
  source: ConfigSource;
}

/** represents config import preview data exchanged through the Meridian API. */
model ConfigImportPreview {
  /** contains the uuid associated with this config import preview. */
  previewId: Uuid;

  /** identifies the repository associated with this resource. */
  repositoryId: Uuid;

  /** specifies the commit associated with this config import preview. */
  commit: string;

  /** is the lowercase SHA-256 digest of the normalized configuration. */
  configDigest: string;

  /** contains the ordered services associated with this config import preview. */
  services: Service[];

  /** contains the ordered sources associated with this config import preview. */
  sources: SourceSpec[];

  /** contains the ordered drift associated with this config import preview. */
  drift: ConfigFieldDiff[];

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  expiresAt: Timestamp;
}

/** defines validated input for the corresponding Meridian API operation. */
model ConfigImportApplyRequest {
  /** is the lowercase SHA-256 digest of the normalized configuration. */
  configDigest: string;

  /** specifies the default resolution associated with this config import apply request. */
  defaultResolution: "keep_db" | "take_file";

  /** Explicitly applies the domain baseReplacement transaction for matching repository bases. */
  replaceAiBases?: boolean;

  /** specifies the field resolutions associated with this config import apply request. */
  fieldResolutions?: Record<"keep_db" | "take_file">;
}

/** reports the result of the corresponding Meridian operation. */
model ConfigImportResult {
  /** identifies the repository associated with this resource. */
  repositoryId: Uuid;

  /** specifies the commit associated with this config import result. */
  commit: string;

  /** is the lowercase SHA-256 digest of the normalized configuration. */
  configDigest: string;

  /** contains the ordered created service ids associated with this config import result. */
  createdServiceIds: Uuid[];

  /** contains the ordered updated service ids associated with this config import result. */
  updatedServiceIds: Uuid[];

  /** contains the ordered source spec ids associated with this config import result. */
  sourceSpecIds: Uuid[];
}

/** afterCommit is null exactly when deleted is true; beforeCommit may be null for a new ref. */
model GitPushWebhook {
  /** contains the ref type associated with this git push webhook. */
  refType: RefType;

  /** contains the ref name associated with this git push webhook. */
  ref: RefName;

  /** specifies the before commit associated with this git push webhook. */
  @pattern("^[0-9a-f]{40,64}$") beforeCommit: string | null;

  /** specifies the after commit associated with this git push webhook. */
  @pattern("^[0-9a-f]{40,64}$") afterCommit: string | null;

  /** indicates whether deleted applies to this git push webhook. */
  deleted: boolean;
}

/** defines validated input for the corresponding Meridian API operation. */
model ServiceCreateRequest {
  /** contains the slug associated with this service create request. */
  slug: Slug;

  /** specifies the display name associated with this service create request. */
  @minLength(1) @maxLength(128) displayName: string;

  /** specifies the description associated with this service create request. */
  @maxLength(2000) description?: string | null;

  /** specifies the visibility associated with this service create request. */
  visibility?: ServiceVisibility;

  /** contains the owner refs associated with this service create request. */
  owners?: OwnerRefs;

  /** contains the ordered tag ids associated with this service create request. */
  tagIds?: Uuid[];

  /** Null or omitted means repository root and is stored as an empty rootDir. */
  @maxLength(512) @format("repository-relative-path") path?: string | null;
}

/** contains one paginated page of service records. */
model ServicePage {
  ...PageInfo;

  /** contains the ordered items associated with this service page. */
  items: Service[];
}

/** defines validated input for the corresponding Meridian API operation. */
model ServicePatchRequest {
  /** specifies the display name associated with this service patch request. */
  @minLength(1) @maxLength(128) displayName?: string;

  /** specifies the description associated with this service patch request. */
  @maxLength(2000) description?: string | null;

  /** contains the service visibility associated with this service patch request. */
  visibility?: ServiceVisibility;

  /** contains the lifecycle associated with this service patch request. */
  lifecycle?: Lifecycle;

  /** contains the owner refs associated with this service patch request. */
  owners?: OwnerRefs;

  /** contains the ordered tag ids associated with this service patch request. */
  tagIds?: Uuid[];
}

/** represents star state data exchanged through the Meridian API. */
model StarState {
  /** indicates whether starred applies to this star state. */
  starred: boolean;
}

/** identifies one supported service role value. */
enum ServiceRole {
  owner,
  maintainer,
  viewer,
}

/** represents service grant data exchanged through the Meridian API. */
model ServiceGrant {
  /** specifies the principal type associated with this service grant. */
  principalType: "user" | "team";

  /** contains the uuid associated with this service grant. */
  principalId: Uuid;

  /** contains the service role associated with this service grant. */
  role: ServiceRole;

  /** contains the ordered permissions associated with this service grant. */
  permissions: ("layer:edit" | "layer:approve" | "asset:publish")[];
}

/** represents service access data exchanged through the Meridian API. */
model ServiceAccess {
  /** contains the ordered grants associated with this service access. */
  grants: ServiceGrant[];

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** defines validated input for the corresponding Meridian API operation. */
model ServiceAccessPutRequest {
  /** contains the ordered grants associated with this service access put request. */
  grants: ServiceGrant[];
}

/** defines validated input for the corresponding Meridian API operation. */
model DriftResolutionRequest {
  /** is the lowercase SHA-256 digest of the normalized configuration. */
  configDigest: string;

  /** specifies the resolution associated with this drift resolution request. */
  resolution: "keep_db" | "take_file" | "ignore";

  /** specifies the fields associated with this drift resolution request. */
  fields?: Record<"keep_db" | "take_file" | "ignore">;
}

/** represents public service data exchanged through the Meridian API. */
model PublicService {
  /** contains the slug associated with this public service. */
  slug: Slug;

  /** specifies the display name associated with this public service. */
  displayName: string;

  /** specifies the description associated with this public service. */
  description: string | null;

  /** contains the lifecycle associated with this public service. */
  lifecycle: Lifecycle;

  /** contains the ordered tags associated with this public service. */
  tags: string[];

  /** contains the ordered assets associated with this public service. */
  assets: AssetSummary[];

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** reports successful liveness or readiness evaluation. */
