/** Shared tenant, credential, team, and tag models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** reports successful liveness or readiness evaluation. */
model Health {
  /** is ok when the requested health check succeeds. */
  status: "ok";
}

/** identifies the running server build and its public API contract. */
model VersionInfo {
  /** is the semantic release version or dev for a local build. */
  serverVersion: string;

  /** is the HTTP route major implemented by this server. */
  apiVersion: "v1";

  /** is the frozen OpenAPI contract version embedded in this build. */
  contractVersion: "1.0.0";

  /** is the source revision embedded at build time or unknown when unset. */
  buildCommit: string;
}

/** represents tenant settings data exchanged through the Meridian API. */
model TenantSettings {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the external revision trust mode associated with this tenant settings. */
  externalRevisionTrustMode: ExternalRevisionTrustMode;

  /** indicates whether auto publish applies to this tenant settings. */
  autoPublish: boolean;

  /** specifies the default locale associated with this tenant settings. */
  defaultLocale: "zh-CN" | "en";

  /** specifies the default ai producer profile id associated with this tenant settings. */
  @format("uuid") defaultAiProducerProfileId: string | null;

  /** contains the retention settings associated with this tenant settings. */
  retention: RetentionSettings;
}

/** defines validated input for the corresponding Meridian API operation. */
model TenantSettingsPatchRequest {
  /** contains the external revision trust mode associated with this tenant settings patch request. */
  externalRevisionTrustMode?: ExternalRevisionTrustMode;

  /** indicates whether auto publish applies to this tenant settings patch request. */
  autoPublish?: boolean;

  /** specifies the default locale associated with this tenant settings patch request. */
  defaultLocale?: "zh-CN" | "en";

  /** specifies the default ai producer profile id associated with this tenant settings patch request. */
  @format("uuid") defaultAiProducerProfileId?: string | null;

  /** contains the retention settings associated with this tenant settings patch request. */
  retention?: RetentionSettings;
}

/** represents producer profile option data exchanged through the Meridian API. */
model ProducerProfileOption {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the name associated with this producer profile option. */
  name: string;

  /** contains the producer profile kind associated with this producer profile option. */
  kind: ProducerProfileKind;

  /** contains the ordered supported kinds associated with this producer profile option. */
  supportedKinds: KindId[];

  /** contains the producer network mode associated with this producer profile option. */
  network: ProducerNetworkMode;

  /** contains the producer dependency status associated with this producer profile option. */
  dependencyStatus: ProducerDependencyStatus;

  /** specifies the unavailable reason associated with this producer profile option. */
  unavailableReason: string | null;
}

/** contains an ordered collection of producer profile option values. */
model ProducerProfileOptionList {
  /** contains the ordered items associated with this producer profile option list. */
  items: ProducerProfileOption[];
}

/** contains one paginated page of member records. */
model MemberPage {
  ...PageInfo;

  /** contains the ordered items associated with this member page. */
  items: Member[];
}

/** identifies one supported token scope value. */
enum TokenScope {
  `asset:push`,
  `asset:read`,
  `job:run`,
}

/** exposes PAT metadata without returning the bearer token or stored token hash. */
model ApiToken {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the name associated with this api token. */
  @minLength(1) @maxLength(64) name: string;

  /** lists the explicit capabilities granted to this token. */
  @minItems(1) scopes: TokenScope[];

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  @oneOf expiresAt: Timestamp | null;

  /** is the most recent successful authentication instant, or null when unused. */
  @oneOf lastUsedAt: Timestamp | null;

  /** is the revocation instant, or null while the token remains active. */
  @oneOf revokedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of token records. */
model TokenPage {
  ...PageInfo;

  /** contains the ordered items associated with this token page. */
  items: ApiToken[];
}

/** defines the name, least-privilege scopes, and optional expiry of a new PAT. */
model TokenCreateRequest {
  /** specifies the name associated with this token create request. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the ordered scopes associated with this token create request. */
  @minItems(1) scopes: TokenScope[];

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  @oneOf expiresAt?: Timestamp | null;
}

/** returns PAT metadata plus the bearer token exactly once at creation time. */
model TokenCreated {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the name associated with this token created. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the ordered scopes associated with this token created. */
  @minItems(1) scopes: TokenScope[];

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  @oneOf expiresAt: Timestamp | null;

  /** specifies the last used at associated with this token created. */
  @oneOf lastUsedAt: Timestamp | null;

  /** specifies the revoked at associated with this token created. */
  @oneOf revokedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is secret bearer material and must never be logged or persisted as plaintext. */
   @pattern("^pat_") token: string;
}

/** exposes encrypted-secret metadata and fingerprint only; plaintext secret material is never returned. */
model Credential {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this credential. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the credential kind associated with this credential. */
  kind: CredentialKind;

  /** contains the credential fingerprint associated with this credential. */
  fingerprint: CredentialFingerprint;

  /** controls whether only the creator, selected teams, or the whole tenant may use the credential. */
  sharedScope: "private" | "team" | "tenant";

  /** lists teams allowed to use a team-scoped credential and is empty for other scopes. */
  teamIds: Uuid[];

  /** indicates whether a platform administrator owns this credential outside any tenant. */
  isGlobal: boolean;

  /** identifies the user who created the credential. */
  createdBy: Uuid;

  /** is the most recent successful credential use, or null when unused. */
  @oneOf lastUsedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of credential records. */
model CredentialPage {
  ...PageInfo;

  /** contains the ordered items associated with this credential page. */
  items: Credential[];
}

/** supplies exactly one SSH or HTTP secret and its authorized sharing scope. */
@oneOf
union CredentialCreateRequest {
  {
    @minLength(1)
    @maxLength(64)
    name: string,

    kind: "ssh_key",
    sshKey: SshSecretInput,
    sharedScope?: "private" | "team" | "tenant" = "private",
    teamIds?: Uuid[],
  },
  {
    @minLength(1)
    @maxLength(64)
    name: string,

    kind: "http_token",
    httpToken: HttpSecretInput,
    sharedScope?: "private" | "team" | "tenant" = "private",
    teamIds?: Uuid[],
  },
}

/** defines validated input for the corresponding Meridian API operation. */
model CredentialPatchRequest {
  /** specifies the name associated with this credential patch request. */
  @minLength(1) @maxLength(64) name?: string;

  /** specifies the shared scope associated with this credential patch request. */
  sharedScope?: "private" | "team" | "tenant";

  /** contains the ordered team ids associated with this credential patch request. */
  teamIds?: Uuid[];
}

/** reports the result of the corresponding Meridian operation. */
model CredentialRotationResult {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the credential associated with this credential rotation result. */
  credential: Credential;

  /** contains the ordered sync jobs associated with this credential rotation result. */
  syncJobs: CredentialSyncJob[];
}

/** represents known host data exchanged through the Meridian API. */
model KnownHost {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the host associated with this known host. */
  @minLength(1) @maxLength(255) host: string;

  /** specifies the port associated with this known host. */
  @minValue(1) @maxValue(65535) port: integer;

  /** SSH public-key algorithm parsed from the stored RFC4253 blob. */
  keyType: "ssh-ed25519" | "ssh-rsa";

  /** specifies the fingerprint associated with this known host. */
  @pattern("^SHA256:[A-Za-z0-9+/]{43}$") fingerprint: string;

  /** specifies the source associated with this known host. */
  source: "manual" | "accept_new";

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of known host records. */
model KnownHostPage {
  ...PageInfo;

  /** contains the ordered items associated with this known host page. */
  items: KnownHost[];
}

/** defines validated input for the corresponding Meridian API operation. */
model KnownHostCreateRequest {
  /** specifies the host associated with this known host create request. */
  @minLength(1) @maxLength(255) host: string;

  /** specifies the port associated with this known host create request. */
  @minValue(1) @maxValue(65535) port?: integer = 22;

  /** Base64 RFC4253 public-key blob. The server derives and validates keyType and fingerprint; clients cannot submit either field. */
  @minLength(1)
  @maxLength(8192)
  @pattern("^[A-Za-z0-9+/]+={0,2}$")
  publicKey: string;
}

/** represents team data exchanged through the Meridian API. */
model Team {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the slug associated with this team. */
  slug: Slug;

  /** specifies the display name associated with this team. */
  @minLength(1) @maxLength(128) displayName: string;

  /** contains the ordered member ids associated with this team. */
  memberIds: Uuid[];

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of team records. */
model TeamPage {
  ...PageInfo;

  /** contains the ordered items associated with this team page. */
  items: Team[];
}

/** defines validated input for the corresponding Meridian API operation. */
model TeamCreateRequest {
  /** contains the slug associated with this team create request. */
  slug: Slug;

  /** specifies the display name associated with this team create request. */
  @minLength(1) @maxLength(128) displayName: string;
}

/** defines validated input for the corresponding Meridian API operation. */
model TeamPatchRequest {
  /** specifies the display name associated with this team patch request. */
  @minLength(1) @maxLength(128) displayName?: string;
}

/** defines validated input for the corresponding Meridian API operation. */
model TeamMembersReplaceRequest {
  /** contains the ordered user ids associated with this team members replace request. */
  userIds: Uuid[];
}

/** contains an ordered collection of tag values. */
model TagList is Tag[];

/** defines validated input for the corresponding Meridian API operation. */
model TagCreateRequest {
  /** specifies the name associated with this tag create request. */
  @minLength(1) @maxLength(64) name: string;

  /** specifies the color associated with this tag create request. */
  @pattern("^#[0-9A-Fa-f]{6}$") color?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model TagPatchRequest {
  /** specifies the name associated with this tag patch request. */
  @minLength(1) @maxLength(64) name?: string;

  /** specifies the color associated with this tag patch request. */
  @pattern("^#[0-9A-Fa-f]{6}$") color?: string | null;
}

/** represents view override data exchanged through the Meridian API. */
