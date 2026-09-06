/** Shared repository and discovery models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** represents ref glob data exchanged through the Meridian API. */
@minLength(1)
@maxLength(255)
@pattern("^[A-Za-z0-9._/*?-]+$")
@format("git-ref-glob")
scalar RefGlob extends string;

/** is a validated Git branch or tag name without a refs prefix. */
@minLength(1)
@maxLength(255)
@pattern("^[A-Za-z0-9._/-]+$")
@format("git-ref-name")
scalar RefName extends string;

/** identifies one supported ref type value. */
enum RefType {
  branch,
  tag,
}

/** defines supported filters for selecting platform audit records. */
model PlatformAuditFilter {
  /** contains the uuid associated with this platform audit filter. */
  actorId?: Uuid;

  /** contains the ordered actions associated with this platform audit filter. */
  actions?: string[];

  /** specifies the resource type associated with this platform audit filter. */
  resourceType?: string;

  /** specifies the resource id associated with this platform audit filter. */
  resourceId?: string;

  /** contains the timestamp associated with this platform audit filter. */
  from?: Timestamp;

  /** contains the timestamp associated with this platform audit filter. */
  to?: Timestamp;

  /** contains the slug associated with this platform audit filter. */
  tenantSlug?: Slug;
}

/** defines validated input for the corresponding Meridian API operation. */
model RepositoryConnectionCheckRequest {
  /** contains the git remote url associated with this repository connection check request. */
  url: GitRemoteUrl;

  /** Tenant-visible credential ID; may identify a tenant credential or a global credential. Null means public access. */
  @oneOf credentialId?: Uuid | null;
}

/** represents branch policy data exchanged through the Meridian API. */
model BranchPolicy {
  /** contains the ordered branch patterns associated with this branch policy. */
  @minItems(1) branchPatterns: RefGlob[];

  /** contains the ordered tag patterns associated with this branch policy. */
  tagPatterns: RefGlob[];
}

/** represents fetch config data exchanged through the Meridian API. */
model FetchConfig {
  /** indicates whether shallow applies to this fetch config. */
  shallow: boolean = true;

  /** specifies the depth associated with this fetch config. */
  @minValue(1) @maxValue(10000) depth?: integer | null = 50;

  /** indicates whether submodules applies to this fetch config. */
  submodules: boolean;

  /** specifies the proxy associated with this fetch config. */
  @maxLength(2048) proxy?: string | null;

  /** contains the ordered path allow associated with this fetch config. */
  pathAllow: string[];

  /** contains the ordered path ignore associated with this fetch config. */
  pathIgnore: string[];

  /** specifies the known host policy associated with this fetch config. */
  knownHostPolicy: "strict" | "accept_new" = "strict";
}

/** represents repository error data exchanged through the Meridian API. */
model RepositoryError {
  /** specifies the class associated with this repository error. */
  class: "dns" | "auth" | "host_key" | "timeout" | "invalid_config" | "other";

  /** specifies the message associated with this repository error. */
  message: string;
}

/** represents repository health data exchanged through the Meridian API. */
model RepositoryHealth {
  /** specifies the last sync at associated with this repository health. */
  @oneOf lastSyncAt: Timestamp | null;

  /** specifies the last commit associated with this repository health. */
  lastCommit: string | null;

  /** specifies the last error associated with this repository health. */
  @oneOf lastError: RepositoryError | null;

  /** specifies the fail streak associated with this repository health. */
  @minValue(0) failStreak: integer;

  /** specifies the duration ms associated with this repository health. */
  @minValue(0) durationMs: integer | null;
}

/** represents repository data exchanged through the Meridian API. */
model Repository {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the git remote url associated with this repository. */
  url: GitRemoteUrl;

  /** Tenant-visible credential ID; may identify a tenant credential or a global credential. Null means public access. */
  @oneOf credentialId: Uuid | null;

  /** contains the ref name associated with this repository. */
  defaultBranch: RefName;

  /** contains the branch policy associated with this repository. */
  branchPolicy: BranchPolicy;

  /** contains the fetch config associated with this repository. */
  fetchConfig: FetchConfig;

  /** Standard five-field cron in UTC; null disables schedule */
  @maxLength(128) @format("cron-5-field") syncCron: string | null;

  /** specifies the note associated with this repository. */
  @maxLength(500) note: string | null;

  /** contains the repository health associated with this repository. */
  health: RepositoryHealth;

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of repository records. */
model RepositoryPage {
  ...PageInfo;

  /** contains the ordered items associated with this repository page. */
  items: Repository[];
}

/** defines validated input for the corresponding Meridian API operation. */
model RepositoryCreateRequest {
  /** contains the git remote url associated with this repository create request. */
  url: GitRemoteUrl;

  /** Tenant-visible credential ID; may identify a tenant credential or a global credential. Null means public access. */
  @oneOf credentialId?: Uuid | null;

  /** contains the ref name associated with this repository create request. */
  defaultBranch: RefName;

  /** contains the branch policy associated with this repository create request. */
  branchPolicy?: BranchPolicy;

  /** contains the fetch config associated with this repository create request. */
  fetchConfig?: FetchConfig;

  /** Standard five-field cron in UTC; null disables schedule */
  @maxLength(128) @format("cron-5-field") syncCron?: string | null;

  /** specifies the note associated with this repository create request. */
  @maxLength(500) note?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model RepositoryPatchRequest {
  /** Tenant-visible credential ID; may identify a tenant credential or a global credential. Null means public access. */
  @oneOf credentialId?: Uuid | null;

  /** contains the ref name associated with this repository patch request. */
  defaultBranch?: RefName;

  /** contains the branch policy associated with this repository patch request. */
  branchPolicy?: BranchPolicy;

  /** contains the fetch config associated with this repository patch request. */
  fetchConfig?: FetchConfig;

  /** Standard five-field cron in UTC; null disables schedule */
  @maxLength(128) @format("cron-5-field") syncCron?: string | null;

  /** specifies the note associated with this repository patch request. */
  @maxLength(500) note?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model RepositorySyncRequest {
  /** contains the ref type associated with this repository sync request. */
  refType?: RefType;

  /** contains the ref name associated with this repository sync request. */
  ref?: RefName;

  /** indicates whether force applies to this repository sync request. */
  force?: boolean;
}

/** defines validated input for the corresponding Meridian API operation. */
model RepositoryDiscoverRequest {
  /** contains the ref type associated with this repository discover request. */
  refType?: RefType;

  /** contains the ref name associated with this repository discover request. */
  ref?: RefName;

  /** contains the ordered path prefixes associated with this repository discover request. */
  pathPrefixes?: string[];
}

/** represents discovery candidate data exchanged through the Meridian API. */
model DiscoveryCandidate {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the path associated with this discovery candidate. */
  path: string;

  /** contains the slug associated with this discovery candidate. */
  suggestedSlug: Slug;

  /** specifies the display name associated with this discovery candidate. */
  displayName: string;

  /** specifies the confidence associated with this discovery candidate. */
  @minValue(0) @maxValue(1) confidence: numeric;

  /** contains the ordered detected kinds associated with this discovery candidate. */
  detectedKinds: KindId[];

  /** is the current lifecycle or processing state. */
  status: "pending" | "accepted" | "dismissed";

  /** contains the timestamp associated with this discovery candidate. */
  discoveredAt: Timestamp;
}

/** contains one paginated page of candidate records. */
model CandidatePage {
  ...PageInfo;

  /** contains the ordered items associated with this candidate page. */
  items: DiscoveryCandidate[];
}

/** identifies one supported service visibility value. */
