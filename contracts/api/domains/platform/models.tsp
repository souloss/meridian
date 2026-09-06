/** Shared platform administration and producer profile models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** contains one paginated page of user records. */
model UserPage {
  ...PageInfo;

  /** contains the ordered items associated with this user page. */
  items: User[];
}

/** defines validated input for the corresponding Meridian API operation. */
model UserCreateRequest {
  /** specifies the username associated with this user create request. */
  @minLength(1) @maxLength(128) username: string;

  /** specifies the display name associated with this user create request. */
  @minLength(1) @maxLength(128) displayName: string;

  /** specifies the email associated with this user create request. */
  @format("email") email?: string | null;

  /** is the write-only plaintext password supplied for authentication. */

  @minLength(12)
  @maxLength(1024)
  password: string;
}

/** defines validated input for the corresponding Meridian API operation. */
model UserPatchRequest {
  /** specifies the display name associated with this user patch request. */
  @minLength(1) @maxLength(128) displayName?: string;

  /** specifies the email associated with this user patch request. */
  @format("email") email?: string | null;

  /** is the current lifecycle or processing state. */
  status?: "active" | "disabled";

  /** is the write-only plaintext password supplied for authentication. */

  @minLength(12)
  @maxLength(1024)
  password?: string;
}

/** identifies one supported tenant status value. */
enum TenantStatus {
  active,
  disabled,
}

/** represents quota data exchanged through the Meridian API. */
model Quota {
  /** specifies the max repositories associated with this quota. */
  @minValue(0) maxRepositories: integer;

  /** specifies the max services associated with this quota. */
  @minValue(0) maxServices: integer;

  /** specifies the max storage bytes associated with this quota. */
  @minValue(0) maxStorageBytes: integer;

  /** specifies the max collect concurrency associated with this quota. */
  @minValue(1) maxCollectConcurrency: integer;
}

/** represents tenant data exchanged through the Meridian API. */
model Tenant {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the slug associated with this tenant. */
  slug: Slug;

  /** specifies the display name associated with this tenant. */
  @minLength(1) @maxLength(128) displayName: string;

  /** is the current lifecycle or processing state. */
  status: TenantStatus;

  /** contains the quota associated with this tenant. */
  quota: Quota;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of tenant records. */
model TenantPage {
  ...PageInfo;

  /** contains the ordered items associated with this tenant page. */
  items: Tenant[];
}

/** defines validated input for the corresponding Meridian API operation. */
model TenantCreateRequest {
  /** contains the slug associated with this tenant create request. */
  slug: Slug;

  /** specifies the display name associated with this tenant create request. */
  @minLength(1) @maxLength(128) displayName: string;

  /** contains the quota associated with this tenant create request. */
  quota?: Quota;
}

/** defines validated input for the corresponding Meridian API operation. */
model TenantDeleteRequest {
  /** contains the slug associated with this tenant delete request. */
  confirmationSlug: Slug;

  /** specifies the current password associated with this tenant delete request. */
   currentPassword: string;
}

/** represents tenant deletion accepted data exchanged through the Meridian API. */
model TenantDeletionAccepted {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** indicates whether deduplicated applies to this tenant deletion accepted. */
  deduplicated: boolean;
}

/** defines validated input for the corresponding Meridian API operation. */
model TenantPatchRequest {
  /** specifies the display name associated with this tenant patch request. */
  @minLength(1) @maxLength(128) displayName?: string;

  /** is the current lifecycle or processing state. */
  status?: TenantStatus;

  /** contains the quota associated with this tenant patch request. */
  quota?: Quota;
}

/** defines validated input for the corresponding Meridian API operation. */
model MemberPutRequest {
  /** contains the tenant role associated with this member put request. */
  role: TenantRole;
}

/** represents member data exchanged through the Meridian API. */
model Member {
  /** contains the user associated with this member. */
  user: User;

  /** contains the tenant role associated with this member. */
  role: TenantRole;

  /** contains the timestamp associated with this member. */
  joinedAt: Timestamp;
}

/** identifies one supported external revision trust mode value. */
enum ExternalRevisionTrustMode {
  review_required,
  trust_ai,
  trust_ai_and_third_party,
}

/** represents retention settings data exchanged through the Meridian API. */
model RetentionSettings {
  /** specifies the job logs days associated with this retention settings. */
  @minValue(1) @maxValue(3650) jobLogsDays: integer;

  /** specifies the archived revisions days associated with this retention settings. */
  @minValue(1) @maxValue(3650) archivedRevisionsDays: integer;
}

/** represents tenant settings template data exchanged through the Meridian API. */
model TenantSettingsTemplate {
  /** contains the external revision trust mode associated with this tenant settings template. */
  externalRevisionTrustMode: ExternalRevisionTrustMode;

  /** indicates whether auto publish applies to this tenant settings template. */
  autoPublish: boolean;

  /** specifies the default locale associated with this tenant settings template. */
  defaultLocale: "zh-CN" | "en";

  /** specifies the default ai producer profile id associated with this tenant settings template. */
  @format("uuid") defaultAiProducerProfileId: string | null;

  /** contains the retention settings associated with this tenant settings template. */
  retention: RetentionSettings;
}

/** represents view override value data exchanged through the Meridian API. */
model ViewOverrideValue {
  /** indicates whether enabled applies to this view override value. */
  enabled?: boolean;

  /** specifies the default options associated with this view override value. */
  defaultOptions?: Record<unknown>;
}

/** represents default notification channel template data exchanged through the Meridian API. */
model DefaultNotificationChannelTemplate {
  /** contains the slug associated with this default notification channel template. */
  key: Slug;

  /** specifies the name associated with this default notification channel template. */
  @minLength(1) @maxLength(64) name: string;

  /** specifies the kind associated with this default notification channel template. */
  kind: "in_app";

  /** indicates whether enabled applies to this default notification channel template. */
  enabled: boolean;
}

/** represents platform settings data exchanged through the Meridian API. */
model PlatformSettings {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the quota associated with this platform settings. */
  defaultQuota: Quota;

  /** contains the tenant settings template associated with this platform settings. */
  defaultTenantSettings: TenantSettingsTemplate;

  /** specifies the default view overrides associated with this platform settings. */
  defaultViewOverrides: Record<ViewOverrideValue>;

  /** contains the ordered default notification channels associated with this platform settings. */
  defaultNotificationChannels: DefaultNotificationChannelTemplate[];
}

/** defines validated input for the corresponding Meridian API operation. */
model PlatformSettingsPatchRequest {
  /** contains the quota associated with this platform settings patch request. */
  defaultQuota?: Quota;

  /** contains the tenant settings template associated with this platform settings patch request. */
  defaultTenantSettings?: TenantSettingsTemplate;

  /** specifies the default view overrides associated with this platform settings patch request. */
  defaultViewOverrides?: Record<ViewOverrideValue>;

  /** contains the ordered default notification channels associated with this platform settings patch request. */
  defaultNotificationChannels?: DefaultNotificationChannelTemplate[];
}

/** identifies one supported credential kind value. */
enum CredentialKind {
  ssh_key,
  http_token,
}

/** SSH uses the OpenSSH SHA256 public-key fingerprint; HTTP tokens use the keyed fingerprint defined in domain.yaml; both encode 32 digest bytes without padding. */
@pattern("^(?:SHA256:[A-Za-z0-9+/]{43}|HMAC-SHA256:[A-Za-z0-9_-]{43})$")
scalar CredentialFingerprint extends string;

/** represents global credential data exchanged through the Meridian API. */
model GlobalCredential {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this global credential. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the credential kind associated with this global credential. */
  kind: CredentialKind;

  /** contains the credential fingerprint associated with this global credential. */
  fingerprint: CredentialFingerprint;

  /** contains the uuid associated with this global credential. */
  createdBy: Uuid;

  /** specifies the last used at associated with this global credential. */
  @oneOf lastUsedAt: Timestamp | null;

  /** is the monotonic optimistic-concurrency version of this resource. */
  @minValue(1) revision: integer;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of global credential records. */
model GlobalCredentialPage {
  ...PageInfo;

  /** contains the ordered items associated with this global credential page. */
  items: GlobalCredential[];
}

/** carries write-only SSH private key material for encryption at rest. */
model SshSecretInput {
  /** is the write-only PEM-encoded SSH private key and must never be logged. */
   @minLength(32) privateKeyPem: string;

  /** unlocks the private key when encrypted and must never be logged. */
   passphrase?: string | null;
}

/** carries write-only HTTP basic or token credentials for encryption at rest. */
model HttpSecretInput {
  /** specifies the username associated with this http secret input. */
  @minLength(1) @maxLength(128) username: string;

  /** is secret bearer material and must never be logged or persisted as plaintext. */
   @minLength(8) token: string;
}

/** defines validated input for the corresponding Meridian API operation. */
@oneOf
union GlobalCredentialCreateRequest {
  {
    @minLength(1)
    @maxLength(64)
    name: string,

    kind: "ssh_key",
    sshKey: SshSecretInput,
  },
  {
    @minLength(1)
    @maxLength(64)
    name: string,

    kind: "http_token",
    httpToken: HttpSecretInput,
  },
}

/** defines validated input for the corresponding Meridian API operation. */
model GlobalCredentialPatchRequest {
  /** specifies the name associated with this global credential patch request. */
  @minLength(1) @maxLength(64) name: string;
}

/** HTTPS URL, ssh:// URL, or SCP-like user\@host:path */
@minLength(1)
@maxLength(2048)
@format("git-remote-url")
scalar GitRemoteUrl extends string;

/** defines validated input for the corresponding Meridian API operation. */
model ConnectionTestRequest {
  /** contains the git remote url associated with this connection test request. */
  repositoryUrl: GitRemoteUrl;
}

/** represents known host candidate data exchanged through the Meridian API. */
model KnownHostCandidate {
  /** specifies the host associated with this known host candidate. */
  @minLength(1) @maxLength(255) host: string;

  /** specifies the port associated with this known host candidate. */
  @minValue(1) @maxValue(65535) port: integer = 22;

  /** SSH public-key algorithm parsed from the RFC4253 blob. */
  keyType: "ssh-ed25519" | "ssh-rsa";

  /** Base64 RFC4253 public-key blob; safe to return in an unknown-host candidate. */
  @minLength(1)
  @maxLength(8192)
  @pattern("^[A-Za-z0-9+/]+={0,2}$")
  publicKey: string;

  /** SHA256 fingerprint of the RFC4253 public-key blob, standard base64 without padding. */
  @pattern("^SHA256:[A-Za-z0-9+/]{43}$") fingerprint: string;
}

/** represents connection test data exchanged through the Meridian API. */
model ConnectionTest {
  /** indicates whether ok applies to this connection test. */
  ok: boolean;

  /** specifies the error class associated with this connection test. */
  errorClass: "dns" | "auth" | "host_key" | "timeout" | "other" | null | null;

  /** specifies the message associated with this connection test. */
  message: string;

  /** specifies the host key candidate associated with this connection test. */
  @oneOf hostKeyCandidate: KnownHostCandidate | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model CredentialRotateRequest {
  /** is write-only secret material and must never be returned by the API. */
  @oneOf secret: SshSecretInput | HttpSecretInput;

  /** Omitted means false. When true, enqueue one default-branch sync for every non-deleted repository referencing this credential. */
  resyncRepositories?: boolean;
}

/** represents credential sync job data exchanged through the Meridian API. */
model CredentialSyncJob {
  /** contains the slug associated with this credential sync job. */
  tenantSlug: Slug;

  /** identifies the repository associated with this resource. */
  repositoryId: Uuid;

  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** indicates whether deduplicated applies to this credential sync job. */
  deduplicated: boolean;
}

/** reports the result of the corresponding Meridian operation. */
model GlobalCredentialRotationResult {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the global credential associated with this global credential rotation result. */
  credential: GlobalCredential;

  /** contains the ordered sync jobs associated with this global credential rotation result. */
  syncJobs: CredentialSyncJob[];
}

/** identifies one supported producer profile kind value. */
enum ProducerProfileKind {
  command,
  ai,
}

/** identifies one supported producer network mode value. */
enum ProducerNetworkMode {
  none,
  inherit,
}

/** identifies one supported producer dependency status value. */
enum ProducerDependencyStatus {
  available,
  unavailable,
}

/** represents producer profile data exchanged through the Meridian API. */
model ProducerProfile {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this producer profile. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the producer profile kind associated with this producer profile. */
  kind: ProducerProfileKind;

  /** specifies the executable associated with this producer profile. */
  @maxLength(512) @pattern("^/") executable: string;

  /** contains the ordered args associated with this producer profile. */
  @maxItems(64) args: string[];

  /** Names only; every name must be in the deployment PRODUCER_ENV_ALLOWLIST. Values are never stored or returned. */
  envAllowlist: string[];

  /** contains the ordered supported kinds associated with this producer profile. */
  @minItems(1) supportedKinds: KindId[];

  /** indicates whether replay safe applies to this producer profile. */
  replaySafe: boolean;

  /** contains the producer network mode associated with this producer profile. */
  network: ProducerNetworkMode;

  /** specifies the timeout sec associated with this producer profile. */
  @minValue(10) @maxValue(3600) timeoutSec: integer;

  /** specifies the memory mi b associated with this producer profile. */
  @minValue(64) @maxValue(16384) memoryMiB: integer;

  /** specifies the cpu seconds associated with this producer profile. */
  @minValue(1) @maxValue(3600) cpuSeconds: integer;

  /** specifies the pids associated with this producer profile. */
  @minValue(1) @maxValue(1024) pids: integer;

  /** indicates whether enabled applies to this producer profile. */
  enabled: boolean;

  /** contains the producer dependency status associated with this producer profile. */
  dependencyStatus: ProducerDependencyStatus;

  /** specifies the unavailable reason associated with this producer profile. */
  unavailableReason: string | null;

  /** is the monotonic optimistic-concurrency version of this resource. */
  @minValue(1) revision: integer;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of producer profile records. */
model ProducerProfilePage {
  ...PageInfo;

  /** contains the ordered items associated with this producer profile page. */
  items: ProducerProfile[];
}

/** defines validated input for the corresponding Meridian API operation. */
model ProducerProfileCreateRequest {
  /** specifies the name associated with this producer profile create request. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the producer profile kind associated with this producer profile create request. */
  kind: ProducerProfileKind;

  /** specifies the executable associated with this producer profile create request. */
  @maxLength(512) @pattern("^/") executable: string;

  /** contains the ordered args associated with this producer profile create request. */
  @maxItems(64) args: string[];

  /** Names only; must be a subset of the deployment producer environment allowlist. */
  envAllowlist: string[];

  /** contains the ordered supported kinds associated with this producer profile create request. */
  @minItems(1) supportedKinds: KindId[];

  /** indicates whether replay safe applies to this producer profile create request. */
  replaySafe: boolean;

  /** contains the producer network mode associated with this producer profile create request. */
  network: ProducerNetworkMode;

  /** Omitted values default to 300 seconds for command profiles and 600 seconds for ai profiles, as defined in domain.yaml. */
  @minValue(10) @maxValue(3600) timeoutSec?: integer;

  /** specifies the memory mi b associated with this producer profile create request. */
  @minValue(64) @maxValue(16384) memoryMiB: integer = 1024;

  /** specifies the cpu seconds associated with this producer profile create request. */
  @minValue(1) @maxValue(3600) cpuSeconds: integer = 600;

  /** specifies the pids associated with this producer profile create request. */
  @minValue(1) @maxValue(1024) pids: integer = 128;

  /** indicates whether enabled applies to this producer profile create request. */
  enabled: boolean = true;
}

/** defines validated input for the corresponding Meridian API operation. */
model ProducerProfilePatchRequest {
  /** specifies the name associated with this producer profile patch request. */
  @minLength(1) @maxLength(64) name?: string;

  /** specifies the executable associated with this producer profile patch request. */
  @maxLength(512) @pattern("^/") executable?: string;

  /** contains the ordered args associated with this producer profile patch request. */
  @maxItems(64) args?: string[];

  /** Names only; must be a subset of the deployment producer environment allowlist. */
  envAllowlist?: string[];

  /** contains the ordered supported kinds associated with this producer profile patch request. */
  @minItems(1) supportedKinds?: KindId[];

  /** indicates whether replay safe applies to this producer profile patch request. */
  replaySafe?: boolean;

  /** contains the producer network mode associated with this producer profile patch request. */
  network?: ProducerNetworkMode;

  /** specifies the timeout sec associated with this producer profile patch request. */
  @minValue(10) @maxValue(3600) timeoutSec?: integer;

  /** specifies the memory mi b associated with this producer profile patch request. */
  @minValue(64) @maxValue(16384) memoryMiB?: integer;

  /** specifies the cpu seconds associated with this producer profile patch request. */
  @minValue(1) @maxValue(3600) cpuSeconds?: integer;

  /** specifies the pids associated with this producer profile patch request. */
  @minValue(1) @maxValue(1024) pids?: integer;

  /** indicates whether enabled applies to this producer profile patch request. */
  enabled?: boolean;
}

/** defines supported filters for selecting platform job records. */
model PlatformJobFilter {
  /** contains the ordered types associated with this platform job filter. */
  types?: JobType[];

  /** contains the ordered statuses associated with this platform job filter. */
  statuses?: JobStatus[];

  /** contains the job scope type associated with this platform job filter. */
  scopeType?: JobScopeType;

  /** specifies the scope id associated with this platform job filter. */
  scopeId?: string;

  /** contains the slug associated with this platform job filter. */
  tenantSlug?: Slug;
}

/** represents platform job data exchanged through the Meridian API. */
model PlatformJob {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the slug associated with this platform job. */
  tenantSlug: Slug;

  /** contains the job type associated with this platform job. */
  type: JobType;

  /** contains the job trigger associated with this platform job. */
  trigger: JobTrigger;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** specifies the stage associated with this platform job. */
  @oneOf stage: PipelineStage | null;

  /** contains the job scope type associated with this platform job. */
  scopeType: JobScopeType;

  /** Exposed only for tenant or repository scope; null for service, asset, version, layer, and other business scopes. */
  scopeId: string | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when processing began. */
  @oneOf startedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when processing reached a terminal state. */
  @oneOf finishedAt: Timestamp | null;
}

/** contains one paginated page of platform job records. */
model PlatformJobPage {
  ...PageInfo;

  /** contains the ordered items associated with this platform job page. */
  items: PlatformJob[];
}

/** defines supported filters for selecting platform audit records. */
