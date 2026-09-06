import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;



/** is a stable lowercase URL identifier containing letters, digits, and hyphens. */
@pattern("^[a-z0-9][a-z0-9-]{0,63}$")
scalar Slug extends string;

/** identifies an asset kind registered in contracts/kinds.yaml. */
@pattern("^[a-z][a-z0-9_]{0,63}$")
scalar KindId extends string;

/** Opaque HTTP entity tag including quotes */
@minLength(3)
@maxLength(128)
@pattern("^(W/)?\"[^\"]+\"$")
scalar ETag extends string;

/** contains an ordered collection of capability values. */
model CapabilityList is string[];

/** represents asset kind data exchanged through the Meridian API. */
model AssetKind {
  /** uniquely identifies this resource. */
  id: KindId;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** indicates whether enabled applies to this asset kind. */
  enabled: boolean;

  /** specifies the milestone associated with this asset kind. */
  @pattern("^M[0-9]+$") milestone: string;

  /** contains the ordered accepted media types associated with this asset kind. */
  acceptedMediaTypes: string[];

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** contains the ordered default views associated with this asset kind. */
  defaultViews: string[];

  /** specifies the schema version associated with this asset kind. */
  @minValue(1) schemaVersion: integer;
}

/** contains an ordered collection of asset kind values. */
model AssetKindList {
  /** contains the ordered items associated with this asset kind list. */
  items: AssetKind[];
}

/** identifies one supported error code value. */
enum ErrorCode {
  unauthenticated,
  csrf_invalid,
  not_found,
  validation_error,
  asset_path_not_found,
  overlay_invalid,
  input_spec_mismatch,
  branch_not_indexed,
  upload_expired,
  nesting_too_deep,
  producer_profile_unavailable,
  unsupported_kind,
  duplicate,
  version_not_publishable,
  base_layer_exists,
  quota_exceeded,
  last_admin,
  credential_in_use,
  invalid_state,
  idempotency_conflict,
  job_not_cancellable,
  config_import_conflict,
  ai_disabled,
  kind_disabled,
  content_too_large,
  rate_limited,
  precondition_failed,
  internal_error,
}

/** is the stable machine-readable failure envelope returned by every JSON API error. */
@extension(
  "x-details-by-code",
  #{
    validation_error: #{ fields: "FieldError[]", unknownFields: "string[]" },
    asset_path_not_found: #{ candidates: "[{path,reason}]" },
    overlay_invalid: #{ errors: "[{line,column,message}]", coordinates: "1-based" },
    version_not_publishable: #{ blockingRevisions: "[{revisionId,layerId,status}]" },
    quota_exceeded: #{ quota: "string", current: "integer", limit: "integer" },
    rate_limited: #{ retryAfterSec: "integer" },
  }
)
@error
model ErrorResponse {
  /** identifies the stable error category clients may branch on. */
  code: ErrorCode;

  /** is a safe human-readable explanation and never contains a stack trace or secret. */
  message: string;

  /** contains error-code-specific structured diagnostics, or null when no detail is available. */
  details?: Record<unknown> | null;

  /** correlates this result with server logs and audit records. */
  @minLength(1) requestId: string;
}

/** defines validated input for the corresponding Meridian API operation. */
model AssetKindPatchRequest {
  /** indicates whether enabled applies to this asset kind patch request. */
  enabled: boolean;
}

/** UUID v7 */
@format("uuid")
scalar Uuid extends string;

/** represents asset name template data exchanged through the Meridian API. */
@maxLength(128)
@pattern("^(?:[a-z0-9-]|\\{file_stem\\}|\\{parent_dir\\})+$")
scalar AssetNameTemplate extends string;

/** identifies one supported layer role value. */
enum LayerRole {
  base,
  overlay,
}

/** identifies one supported layer origin value. */
enum LayerOrigin {
  repo,
  third_party,
  manual,
  ai_generated,
}

/** identifies one supported source mode value. */
enum SourceMode {
  builtin,
  command,
  push,
  manual,
  ai,
}

/** represents ref glob data exchanged through the Meridian API. */
@minLength(1)
@maxLength(255)
@pattern("^[A-Za-z0-9._/*?-]+$")
@format("git-ref-glob")
scalar RefGlob extends string;

/** identifies one supported config source value. */
enum ConfigSource {
  db_manual,
  repo_file,
  repo_bootstrap,
}

/** represents field source data exchanged through the Meridian API. */
model FieldSource {
  /** specifies the pointer associated with this field source. */
  pointer: string;

  /** contains the config source associated with this field source. */
  source: ConfigSource;
}

/** identifies one supported ref type value. */
enum RefType {
  branch,
  tag,
}

/** is a validated Git branch or tag name without a refs prefix. */
@minLength(1)
@maxLength(255)
@pattern("^[A-Za-z0-9._/-]+$")
@format("git-ref-name")
scalar RefName extends string;

/** identifies one supported job status value. */
enum JobStatus {
  pending,
  running,
  succeeded,
  succeeded_with_warnings,
  failed,
  outcome_unknown,
  cancelled,
}

/** RFC 3339 UTC */
scalar Timestamp extends utcDateTime;

/** represents source run summary data exchanged through the Meridian API. */
model SourceRunSummary {
  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** contains the ref type associated with this source run summary. */
  refType: RefType;

  /** contains the ref name associated with this source run summary. */
  ref: RefName;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** is the RFC 3339 UTC instant when processing began. */
  startedAt: Timestamp;

  /** is the RFC 3339 UTC instant when processing reached a terminal state. */
  @oneOf finishedAt: Timestamp | null;
}

/** represents source spec data exchanged through the Meridian API. */
@extension("x-source-compatibility", "../contracts/domain.yaml#/sourceCompatibility")
model SourceSpec {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** identifies the service associated with this resource. */
  serviceId: Uuid;

  /** contains the kind id associated with this source spec. */
  kind: KindId;

  /** contains the asset name template associated with this source spec. */
  assetNameTemplate: AssetNameTemplate;

  /** contains the layer role associated with this source spec. */
  role: LayerRole;

  /** contains the layer origin associated with this source spec. */
  origin: LayerOrigin;

  /** contains the source mode associated with this source spec. */
  mode: SourceMode;

  /** specifies the path associated with this source spec. */
  @maxLength(512) @format("repository-relative-glob") path: string | null;

  /** specifies the producer profile id associated with this source spec. */
  @oneOf producerProfileId: Uuid | null;

  /** specifies the ord associated with this source spec. */
  @minValue(0) ord: integer;

  /** Effective producer timeout is the minimum of this value and the selected producer profile timeout. Mode defaults are defined in domain.yaml. */
  @minValue(10) @maxValue(3600) timeoutSec: integer;

  /** contains the ordered branch patterns associated with this source spec. */
  @minItems(1) branchPatterns: RefGlob[];

  /** indicates whether enabled applies to this source spec. */
  enabled: boolean;

  /** contains the config source associated with this source spec. */
  configOrigin: ConfigSource;

  /** contains the ordered config field sources associated with this source spec. */
  configFieldSources: FieldSource[];

  /** specifies the last error associated with this source spec. */
  lastError: string | null;

  /** specifies the last run associated with this source spec. */
  @oneOf lastRun: SourceRunSummary | null;

  /** specifies the bindings count associated with this source spec. */
  @minValue(0) bindingsCount: integer;

  /** Non-null only for a manual SourceSpec. createSourceSpec atomically creates its one global SourceBinding and Layer and returns that Layer ID so the caller can immediately call createLayerRevision. Non-manual specs return null until jobs materialize zero or more bindings. */
  @oneOf initialLayerId: Uuid | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of source spec values. */
model SourceSpecList {
  /** contains the ordered items associated with this source spec list. */
  items: SourceSpec[];
}

/** represents source spec input data exchanged through the Meridian API. */
@extension("x-source-compatibility", "../contracts/domain.yaml#/sourceCompatibility")
model SourceSpecInput {
  /** contains the kind id associated with this source spec input. */
  kind: KindId;

  /** specifies the asset name template associated with this source spec input. */
  assetNameTemplate?: AssetNameTemplate = "{file_stem}";

  /** contains the layer role associated with this source spec input. */
  role: LayerRole;

  /** contains the layer origin associated with this source spec input. */
  origin: LayerOrigin;

  /** contains the source mode associated with this source spec input. */
  mode: SourceMode;

  /** specifies the path associated with this source spec input. */
  @maxLength(512) @format("repository-relative-glob") path?: string | null;

  /** specifies the producer profile id associated with this source spec input. */
  @oneOf producerProfileId?: Uuid | null;

  /** specifies the ord associated with this source spec input. */
  @minValue(0) ord?: integer;

  /** Omitted values use the mode default in domain.yaml. */
  @minValue(10) @maxValue(3600) timeoutSec?: integer;

  /** contains the ordered branch patterns associated with this source spec input. */
  @minItems(1) branchPatterns?: RefGlob[] = #["**"];

  /** indicates whether enabled applies to this source spec input. */
  enabled?: boolean = true;

  /** On create only, explicitly archives an existing AI-generated base and its source in the same transaction before creating this repository base. Invalid for other replacements. */
  replaceAiBase?: boolean;

  /** Required only for manual mode; the asset must belong to the path service and match kind. */
  @oneOf targetAssetId?: Uuid | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model SourceSpecCreateRequest {
  ...SourceSpecInput;
}

/** The server merges this partial document with the stored SourceSpec and validates the complete resulting aggregate against sourceCompatibility. */
@extension("x-source-compatibility", "../contracts/domain.yaml#/sourceCompatibility")
model SourceSpecPatchRequest {
  /** contains the asset name template associated with this source spec patch request. */
  assetNameTemplate?: AssetNameTemplate;

  /** contains the layer role associated with this source spec patch request. */
  role?: LayerRole;

  /** contains the layer origin associated with this source spec patch request. */
  origin?: LayerOrigin;

  /** contains the source mode associated with this source spec patch request. */
  mode?: SourceMode;

  /** specifies the path associated with this source spec patch request. */
  @maxLength(512) @format("repository-relative-glob") path?: string | null;

  /** specifies the producer profile id associated with this source spec patch request. */
  @oneOf producerProfileId?: Uuid | null;

  /** specifies the ord associated with this source spec patch request. */
  @minValue(0) ord?: integer;

  /** specifies the timeout sec associated with this source spec patch request. */
  @minValue(10) @maxValue(3600) timeoutSec?: integer;

  /** contains the ordered branch patterns associated with this source spec patch request. */
  @minItems(1) branchPatterns?: RefGlob[];

  /** indicates whether enabled applies to this source spec patch request. */
  enabled?: boolean;
}

/** represents source binding data exchanged through the Meridian API. */
model SourceBinding {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the uuid associated with this source binding. */
  sourceSpecId: Uuid;

  /** specifies the scope type associated with this source binding. */
  scopeType: "ref" | "global";

  /** '*' for global, otherwise 'branch:<name>' or 'tag:<name>' */
  @minLength(1) @maxLength(260) scopeKey: string;

  /** specifies the expansion key associated with this source binding. */
  expansionKey: string;

  /** Null only for push/manual sources with no repository file. A previously resolved repository path is retained when a file disappears and the binding becomes stale. */
  resolvedPath: string | null;

  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** contains the uuid associated with this source binding. */
  layerId: Uuid;

  /** specifies the state associated with this source binding. */
  state: "active" | "stale" | "error";

  /** Null for global non-Git materializations. */
  @pattern("^[0-9a-f]{40,64}$") lastSeenCommit: string | null;
}

/** contains an ordered collection of source binding values. */
model SourceBindingList {
  /** contains the ordered items associated with this source binding list. */
  items: SourceBinding[];
}

/** defines validated input for the corresponding Meridian API operation. */
model ProduceSourceRequest {
  /** contains the ref type associated with this produce source request. */
  refType?: RefType;

  /** contains the ref name associated with this produce source request. */
  ref?: RefName;

  /** indicates whether force applies to this produce source request. */
  force?: boolean;
}

/** represents job accepted data exchanged through the Meridian API. */
model JobAccepted {
  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** indicates whether deduplicated applies to this job accepted. */
  deduplicated: boolean;
}

/** is a stable lowercase asset identifier within one service and kind. */
@pattern("^[a-z0-9][a-z0-9-]{0,63}$")
scalar AssetName extends string;

/** defines validated input for the corresponding Meridian API operation. */
model ServiceAiGenerateRequest {
  /** contains the kind id associated with this service ai generate request. */
  kind: KindId;

  /** contains the asset name associated with this service ai generate request. */
  name: AssetName;

  /** contains the ref type associated with this service ai generate request. */
  refType?: RefType;

  /** contains the ref name associated with this service ai generate request. */
  ref?: RefName;

  /** specifies the hint associated with this service ai generate request. */
  @maxLength(500) hint?: string | null;

  /** contains the uuid associated with this service ai generate request. */
  producerProfileId?: Uuid;
}

/** represents ai generation accepted data exchanged through the Meridian API. */
model AiGenerationAccepted {
  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** identifies the source associated with this resource. */
  sourceId: Uuid;

  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** indicates whether deduplicated applies to this ai generation accepted. */
  deduplicated: boolean;
}

/** identifies one supported lifecycle value. */
enum Lifecycle {
  draft,
  published,
  deprecated,
  retired,
}

/** represents version ref data exchanged through the Meridian API. */
model VersionRef {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the version associated with this version ref. */
  version: string;

  /** contains the lifecycle associated with this version ref. */
  lifecycle: Lifecycle;
}

/** represents layer head data exchanged through the Meridian API. */
model LayerHead {
  /** specifies the scope type associated with this layer head. */
  scopeType: "ref" | "global";

  /** '*' for global, otherwise 'branch:<name>' or 'tag:<name>' */
  @minLength(1) @maxLength(260) scopeKey: string;

  /** specifies the latest revision id associated with this layer head. */
  @oneOf latestRevisionId: Uuid | null;

  /** specifies the effective revision id associated with this layer head. */
  @oneOf effectiveRevisionId: Uuid | null;

  /** indicates whether candidate revision id applies to this layer head. */
  @oneOf candidateRevisionId: Uuid | null;

  /** specifies the generation associated with this layer head. */
  @minValue(0) generation: integer;
}

/** represents layer summary data exchanged through the Meridian API. */
model LayerSummary {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the layer role associated with this layer summary. */
  role: LayerRole;

  /** contains the layer origin associated with this layer summary. */
  origin: LayerOrigin;

  /** specifies the ord associated with this layer summary. */
  @minValue(0) ord: integer;

  /** contains the ordered heads associated with this layer summary. */
  heads: LayerHead[];
}

/** represents asset data exchanged through the Meridian API. */
model Asset {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** identifies the service associated with this resource. */
  serviceId: Uuid;

  /** contains the kind id associated with this asset. */
  kind: KindId;

  /** contains the asset name associated with this asset. */
  name: AssetName;

  /** of currentVersion, otherwise latestVersion, otherwise draft for the selected ref Track. */
  lifecycle: Lifecycle;

  /** specifies the health associated with this asset. */
  health: "ok" | "stale" | "invalid";

  /** specifies the quality score associated with this asset. */
  @minValue(0) @maxValue(100) qualityScore: integer | null;

  /** specifies the current version associated with this asset. */
  @oneOf currentVersion: VersionRef | null;

  /** specifies the latest version associated with this asset. */
  @oneOf latestVersion: VersionRef | null;

  /** contains the ref type associated with this asset. */
  refType: RefType;

  /** contains the ref name associated with this asset. */
  ref: RefName;

  /** contains the ordered layers associated with this asset. */
  layers: LayerSummary[];

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** represents page info data exchanged through the Meridian API. */
model PageInfo {
  /** is the number of matching records across all pages. */
  @minValue(0) total: integer;

  /** is the one-based page number. */
  @minValue(1) page: integer;

  /** is the maximum number of records returned on one page. */
  @minValue(1) @maxValue(100) pageSize: integer;
}

/** identifies one supported revision status value. */
enum RevisionStatus {
  not_required,
  pending_review,
  approved,
  rejected,
  superseded,
}

/** represents layer manifest entry data exchanged through the Meridian API. */
model LayerManifestEntry {
  /** contains the uuid associated with this layer manifest entry. */
  layerId: Uuid;

  /** contains the uuid associated with this layer manifest entry. */
  revisionId: Uuid;

  /** contains the layer role associated with this layer manifest entry. */
  role: LayerRole;

  /** contains the layer origin associated with this layer manifest entry. */
  origin: LayerOrigin;

  /** contains the revision status associated with this layer manifest entry. */
  reviewStatus: RevisionStatus;

  /** specifies the scope type associated with this layer manifest entry. */
  scopeType: "ref" | "global";

  /** '*' for global, otherwise 'branch:<name>' or 'tag:<name>' */
  @minLength(1) @maxLength(260) scopeKey: string;

  /** specifies the ord associated with this layer manifest entry. */
  @minValue(0) ord: integer;

  /** specifies the content hash associated with this layer manifest entry. */
  contentHash: string;
}

/** represents diff counts data exchanged through the Meridian API. */
model DiffCounts {
  /** specifies the added associated with this diff counts. */
  @minValue(0) added: integer;

  /** specifies the removed associated with this diff counts. */
  @minValue(0) removed: integer;

  /** specifies the modified associated with this diff counts. */
  @minValue(0) modified: integer;

  /** specifies the breaking associated with this diff counts. */
  @minValue(0) breaking: integer;

  /** specifies the risky associated with this diff counts. */
  @minValue(0) risky: integer;

  /** specifies the non breaking associated with this diff counts. */
  @minValue(0) nonBreaking: integer;

  /** specifies the informational associated with this diff counts. */
  @minValue(0) informational: integer;
}

/** represents asset version data exchanged through the Meridian API. */
model AssetVersion {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** contains the ref type associated with this asset version. */
  refType: RefType;

  /** contains the ref name associated with this asset version. */
  ref: RefName;

  /** specifies the sequence no associated with this asset version. */
  @minValue(1) sequenceNo: integer;

  /** specifies the version associated with this asset version. */
  version: string;

  /** contains the lifecycle associated with this asset version. */
  lifecycle: Lifecycle;

  /** specifies the quality score associated with this asset version. */
  @minValue(0) @maxValue(100) qualityScore: integer;

  /** specifies the merge engine version associated with this asset version. */
  mergeEngineVersion: string;

  /** specifies the overlay compiler version associated with this asset version. */
  overlayCompilerVersion: string;

  /** specifies the overlay mode associated with this asset version. */
  overlayMode: "lenient" | "strict";

  /** specifies the normalizer version associated with this asset version. */
  normalizerVersion: string;

  /** specifies the kind plugin version associated with this asset version. */
  kindPluginVersion: string;

  /** specifies the input fingerprint associated with this asset version. */
  inputFingerprint: string;

  /** specifies the merged hash associated with this asset version. */
  mergedHash: string;

  /** contains the ordered layer manifest associated with this asset version. */
  layerManifest: LayerManifestEntry[];

  /** specifies the source commit associated with this asset version. */
  sourceCommit: string | null;

  /** specifies the baseline version id associated with this asset version. */
  @oneOf baselineVersionId: Uuid | null;

  /** specifies the diff summary associated with this asset version. */
  @oneOf diffSummary: DiffCounts | null;

  /** specifies the labels associated with this asset version. */
  labels: Record<string>;

  /** specifies the downloads associated with this asset version. */
  downloads: Record<string>;

  /** specifies the indexed at associated with this asset version. */
  @oneOf indexedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;
}

/** contains one paginated page of asset version records. */
model AssetVersionPage {
  ...PageInfo;

  /** contains the ordered items associated with this asset version page. */
  items: AssetVersion[];
}

/** represents provenance entry data exchanged through the Meridian API. */
model ProvenanceEntry {
  /** specifies the pointer associated with this provenance entry. */
  pointer: string;

  /** contains the uuid associated with this provenance entry. */
  layerId: Uuid;

  /** contains the uuid associated with this provenance entry. */
  revisionId: Uuid;
}

/** represents asset item data exchanged through the Meridian API. */
model AssetItem {
  /** specifies the item type associated with this asset item. */
  itemType: string;

  /** specifies the key associated with this asset item. */
  key: string;

  /** specifies the display associated with this asset item. */
  display: Record<unknown>;

  /** contains the ordered provenance associated with this asset item. */
  provenance: ProvenanceEntry[];
}

/** contains one paginated page of asset item records. */
model AssetItemPage {
  ...PageInfo;

  /** contains the ordered items associated with this asset item page. */
  items: AssetItem[];
}

/** contains an ordered collection of provenance values. */
model ProvenanceList {
  /** contains the ordered items associated with this provenance list. */
  items: ProvenanceEntry[];
}

/** defines validated input for the corresponding Meridian API operation. */
model VersionPublishRequest {
  /** specifies the version associated with this version publish request. */
  @maxLength(128) version?: string | null;

  /** specifies the labels associated with this version publish request. */
  labels?: Record<string>;
}

/** defines validated input for the corresponding Meridian API operation. */
model AssetAiGenerateRequest {
  /** identifies the source associated with this resource. */
  @oneOf sourceId?: Uuid | null;

  /** contains the ref type associated with this asset ai generate request. */
  refType?: RefType;

  /** contains the ref name associated with this asset ai generate request. */
  ref?: RefName;

  /** specifies the hint associated with this asset ai generate request. */
  @maxLength(500) hint?: string | null;

  /** contains the uuid associated with this asset ai generate request. */
  producerProfileId?: Uuid;
}

/** represents merge layer input data exchanged through the Meridian API. */
model MergeLayerInput {
  /** contains the uuid associated with this merge layer input. */
  layerId: Uuid;

  /** contains the uuid associated with this merge layer input. */
  revisionId: Uuid;
}

/** defines validated input for the corresponding Meridian API operation. */
model MergePreviewRequest {
  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** contains the ref type associated with this merge preview request. */
  refType?: RefType;

  /** contains the ref name associated with this merge preview request. */
  ref: RefName;

  /** contains the ordered layers associated with this merge preview request. */
  layers?: MergeLayerInput[];

  /** specifies the override content associated with this merge preview request. */
  @maxLength(1048576) overrideContent?: string | null;
}

/** identifies one supported content type value. */
enum ContentType {
  yaml,
  json,
}

/** represents validation issue data exchanged through the Meridian API. */
model ValidationIssue {
  /** specifies the severity associated with this validation issue. */
  severity: "error" | "warning" | "info";

  /** specifies the code associated with this validation issue. */
  code: string;

  /** specifies the message associated with this validation issue. */
  message: string;

  /** specifies the pointer associated with this validation issue. */
  pointer: string | null;
}

/** represents merge preview data exchanged through the Meridian API. */
model MergePreview {
  /** specifies the input fingerprint associated with this merge preview. */
  inputFingerprint: string;

  /** specifies the content associated with this merge preview. */
  @maxLength(1048576) content: string;

  /** contains the content type associated with this merge preview. */
  contentType: ContentType;

  /** contains the ordered validation associated with this merge preview. */
  validation: ValidationIssue[];

  /** contains the ordered provenance associated with this merge preview. */
  provenance: ProvenanceEntry[];
}

/** defines validated input for the corresponding Meridian API operation. */
model AssetPushRequest {
  /** contains the slug associated with this asset push request. */
  serviceSlug: Slug;

  /** contains the kind id associated with this asset push request. */
  kind: KindId;

  /** contains the asset name associated with this asset push request. */
  name: AssetName;

  /** contains the ref type associated with this asset push request. */
  refType?: RefType;

  /** contains the ref name associated with this asset push request. */
  ref: RefName;

  /** specifies the source system associated with this asset push request. */
  @minLength(1)
  @maxLength(128)
  @pattern("^[A-Za-z0-9._:-]+$")
  sourceSystem: string;

  /** indicates whether create if missing applies to this asset push request. */
  createIfMissing?: boolean;

  /** specifies the content associated with this asset push request. */
  @maxLength(1048576) content: string;

  /** contains the content type associated with this asset push request. */
  contentType: ContentType;

  /** specifies the role associated with this asset push request. */
  role?: LayerRole;

  /** specifies the dialect associated with this asset push request. */
  dialect?: string | null;

  /** specifies the source commit associated with this asset push request. */
  sourceCommit?: string | null;
}

/** reports the result of the corresponding Meridian operation. */
model AssetPushResult {
  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** contains the uuid associated with this asset push result. */
  layerId: Uuid;

  /** contains the uuid associated with this asset push result. */
  revisionId: Uuid;

  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** indicates whether deduplicated applies to this asset push result. */
  deduplicated: boolean;
}

/** represents public asset data exchanged through the Meridian API. */
model PublicAsset {
  /** contains the kind id associated with this public asset. */
  kind: KindId;

  /** contains the asset name associated with this public asset. */
  name: AssetName;

  /** contains the lifecycle associated with this public asset. */
  lifecycle: Lifecycle;

  /** contains the version ref associated with this public asset. */
  currentVersion: VersionRef;

  /** specifies the content url associated with this public asset. */
  @format("uri-reference") contentUrl: string;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** carries the anti-forgery token bound to the current cookie session. */
model CsrfToken {
  /** binds a state-changing browser request to the current session. */
  @minLength(32) csrfToken: string;
}

/** contains local credentials used to create a browser session. */
model LoginRequest {
  /** specifies the username associated with this login request. */
  @minLength(1) @maxLength(128) username: string;

  /** is the write-only plaintext password supplied for authentication. */
  
  @minLength(1)
  @maxLength(1024)
  password: string;
}

/** is a local Meridian identity; password hashes and session secrets are never exposed. */
model User {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the username associated with this user. */
  @minLength(1) @maxLength(128) username: string;

  /** specifies the display name associated with this user. */
  @minLength(1) @maxLength(128) displayName: string;

  /** specifies the email associated with this user. */
  @format("email") email: string | null;

  /** is the current lifecycle or processing state. */
  status: "active" | "disabled";

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** identifies one supported tenant role value. */
enum TenantRole {
  tenant_admin,
  maintainer,
  viewer,
}

/** identifies one tenant the current user may enter and the user's role there. */
model TenantMembershipRef {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the slug associated with this tenant membership ref. */
  slug: Slug;

  /** specifies the display name associated with this tenant membership ref. */
  displayName: string;

  /** contains the tenant role associated with this tenant membership ref. */
  role: TenantRole;
}

/** describes the authenticated user and every tenant context available to that user. */
model Me {
  /** is the authenticated local identity. */
  user: User;

  /** lists tenant contexts the authenticated user may access. */
  tenants: TenantMembershipRef[];

  /** indicates whether the user may access platform administration operations. */
  isPlatformAdmin: boolean;
}

/** returns the authenticated principal and the initial session-bound CSRF token. */
model LoginResult {
  /** contains the authenticated user, memberships, and platform role. */
  me: Me;

  /** binds a state-changing browser request to the current session. */
  @minLength(32) csrfToken: string;
}

/** represents user preferences data exchanged through the Meridian API. */
model UserPreferences {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the locale associated with this user preferences. */
  locale: "zh-CN" | "en";

  /** specifies the theme associated with this user preferences. */
  theme: "light" | "dark" | "system";

  /** specifies the default views associated with this user preferences. */
  defaultViews: Record<string>;
}

/** defines validated input for the corresponding Meridian API operation. */
model UserPreferencesPatchRequest {
  /** specifies the locale associated with this user preferences patch request. */
  locale?: "zh-CN" | "en";

  /** specifies the theme associated with this user preferences patch request. */
  theme?: "light" | "dark" | "system";

  /** specifies the default views associated with this user preferences patch request. */
  defaultViews?: Record<string>;
}

/** identifies one supported notification channel kind value. */
enum NotificationChannelKind {
  in_app,
  webhook,
  email,
}

/** represents notification channel data exchanged through the Meridian API. */
model NotificationChannel {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this notification channel. */
  name: string;

  /** contains the notification channel kind associated with this notification channel. */
  kind: NotificationChannelKind;

  /** indicates whether enabled applies to this notification channel. */
  enabled: boolean;

  /** specifies the endpoint associated with this notification channel. */
  endpoint: string | null;

  /** indicates whether secret configured applies to this notification channel. */
  secretConfigured: boolean;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of notification channel values. */
model NotificationChannelList {
  /** contains the ordered items associated with this notification channel list. */
  items: NotificationChannel[];
}

/** defines validated input for the corresponding Meridian API operation. */
model NotificationChannelCreateRequest {
  /** specifies the name associated with this notification channel create request. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the notification channel kind associated with this notification channel create request. */
  kind: NotificationChannelKind;

  /** indicates whether enabled applies to this notification channel create request. */
  enabled: boolean;

  /** specifies the endpoint associated with this notification channel create request. */
  @maxLength(2048) endpoint?: string | null;

  /** is write-only secret material and must never be returned by the API. */
  
  @minLength(32)
  @maxLength(4096)
  secret?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model NotificationChannelPatchRequest {
  /** specifies the name associated with this notification channel patch request. */
  @minLength(1) @maxLength(64) name?: string;

  /** indicates whether enabled applies to this notification channel patch request. */
  enabled?: boolean;

  /** specifies the endpoint associated with this notification channel patch request. */
  @maxLength(2048) endpoint?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model NotificationChannelRotateRequest {
  /** is write-only secret material and must never be returned by the API. */
  
  @minLength(32)
  @maxLength(4096)
  secret: string;
}

/** represents comment data exchanged through the Meridian API. */
model Comment {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** identifies the service associated with this resource. */
  serviceId: Uuid;

  /** contains the user associated with this comment. */
  author: User;

  /** specifies the body associated with this comment. */
  body: string;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of comment records. */
model CommentPage {
  ...PageInfo;

  /** contains the ordered items associated with this comment page. */
  items: Comment[];
}

/** defines validated input for the corresponding Meridian API operation. */
model CommentCreateRequest {
  /** specifies the body associated with this comment create request. */
  @minLength(1) @maxLength(2000) body: string;
}

/** represents subscription scope constraints data exchanged through the Meridian API. */
model SubscriptionScopeConstraints {}

/** identifies one supported domain event type value. */
enum DomainEventType {
  `version.published`,
  `version.breaking`,
  `collect.failed`,
  `service.deprecated`,
  `ai_layer.generated`,
  `layer.approved`,
  `layer.rejected`,
}

/** identifies one supported subscription scope type value. */
enum SubscriptionScopeType {
  tenant,
  service,
  asset,
  system_group,
  asset_kind,
}

/** represents subscription data exchanged through the Meridian API. */
model Subscription {
  ...SubscriptionScopeConstraints;

  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the ordered event types associated with this subscription. */
  @minItems(1) eventTypes: DomainEventType[];

  /** contains the subscription scope type associated with this subscription. */
  scopeType: SubscriptionScopeType;

  /** specifies the scope id associated with this subscription. */
  @oneOf scopeId: Uuid | KindId | null;

  /** contains the ordered channel ids associated with this subscription. */
  @minItems(1) channelIds: Uuid[];

  /** indicates whether enabled applies to this subscription. */
  enabled: boolean;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of subscription values. */
model SubscriptionList {
  /** contains the ordered items associated with this subscription list. */
  items: Subscription[];
}

/** defines validated input for the corresponding Meridian API operation. */
model SubscriptionPutRequest {
  ...SubscriptionScopeConstraints;

  /** contains the ordered event types associated with this subscription put request. */
  @minItems(1) eventTypes: DomainEventType[];

  /** contains the subscription scope type associated with this subscription put request. */
  scopeType: SubscriptionScopeType;

  /** specifies the scope id associated with this subscription put request. */
  @oneOf scopeId: Uuid | KindId | null;

  /** contains the ordered channel ids associated with this subscription put request. */
  @minItems(1) channelIds: Uuid[];

  /** indicates whether enabled applies to this subscription put request. */
  enabled: boolean;
}

/** represents notification data exchanged through the Meridian API. */
model Notification {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the domain event type associated with this notification. */
  eventType: DomainEventType;

  /** specifies the title associated with this notification. */
  title: string;

  /** specifies the body associated with this notification. */
  body: string;

  /** specifies the resource url associated with this notification. */
  @format("uri-reference") resourceUrl: string | null;

  /** specifies the read at associated with this notification. */
  @oneOf readAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of notification records. */
model NotificationPage {
  ...PageInfo;

  /** contains the ordered items associated with this notification page. */
  items: Notification[];

  /** specifies the unread count associated with this notification page. */
  @minValue(0) unreadCount: integer;
}

/** identifies one supported todo status value. */
enum TodoStatus {
  open,
  acked,
}

/** represents breaking todo data exchanged through the Meridian API. */
model BreakingTodo {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the uuid associated with this breaking todo. */
  assetVersionId: Uuid;

  /** contains the uuid associated with this breaking todo. */
  assigneeId: Uuid;

  /** is the current lifecycle or processing state. */
  status: TodoStatus;

  /** contains the diff counts associated with this breaking todo. */
  summary: DiffCounts;

  /** specifies the comment associated with this breaking todo. */
  comment: string | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** specifies the acknowledged at associated with this breaking todo. */
  @oneOf acknowledgedAt: Timestamp | null;
}

/** contains one paginated page of breaking todo records. */
model BreakingTodoPage {
  ...PageInfo;

  /** contains the ordered items associated with this breaking todo page. */
  items: BreakingTodo[];
}

/** defines validated input for the corresponding Meridian API operation. */
model BreakingTodoAcknowledgeRequest {
  /** specifies the comment associated with this breaking todo acknowledge request. */
  @maxLength(2000) comment?: string | null;
}

/** defines supported filters for selecting audit records. */
model AuditFilter {
  /** contains the uuid associated with this audit filter. */
  actorId?: Uuid;

  /** contains the ordered actions associated with this audit filter. */
  actions?: string[];

  /** specifies the resource type associated with this audit filter. */
  resourceType?: string;

  /** specifies the resource id associated with this audit filter. */
  resourceId?: string;

  /** contains the timestamp associated with this audit filter. */
  from?: Timestamp;

  /** contains the timestamp associated with this audit filter. */
  to?: Timestamp;
}

/** represents audit entry data exchanged through the Meridian API. */
model AuditEntry {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the tenant slug associated with this audit entry. */
  @oneOf tenantSlug: Slug | null;

  /** specifies the actor id associated with this audit entry. */
  @oneOf actorId: Uuid | null;

  /** specifies the action associated with this audit entry. */
  action: string;

  /** specifies the resource type associated with this audit entry. */
  resourceType: string;

  /** specifies the resource id associated with this audit entry. */
  resourceId: string | null;

  /** correlates this result with server logs and audit records. */
  requestId: string | null;

  /** specifies the metadata associated with this audit entry. */
  metadata: Record<unknown>;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of audit log records. */
model AuditLogPage {
  ...PageInfo;

  /** contains the ordered items associated with this audit log page. */
  items: AuditEntry[];
}

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
enum JobType {
  `tenant.delete`,
  `repo.sync`,
  `repo.discover`,
  `asset.produce`,
  `asset.merge`,
  `asset.index`,
  `asset.ai_generate`,
  `asset.reindex`,
  `diff.run`,
  `outbox.dispatch`,
  `workspace.gc`,
  `blob.gc`,
  `retention.cleanup`,
}

/** identifies one supported job scope type value. */
enum JobScopeType {
  tenant,
  repository,
  service,
  source,
  track,
  version,
  asset,
  diff,
  system,
}

/** defines supported filters for selecting job records. */
model JobFilter {
  /** contains the ordered types associated with this job filter. */
  types?: JobType[];

  /** contains the ordered statuses associated with this job filter. */
  statuses?: JobStatus[];

  /** contains the job scope type associated with this job filter. */
  scopeType?: JobScopeType;

  /** specifies the scope id associated with this job filter. */
  scopeId?: string;
}

/** identifies one supported job trigger value. */
enum JobTrigger {
  manual,
  schedule,
  webhook,
  api,
  cli,
  system,
  retry,
  `credential-rotated`,
}

/** identifies one supported pipeline stage value. */
enum PipelineStage {
  resolve,
  discover,
  extract,
  merge,
  normalize,
  index,
}

/** represents job stage attempt data exchanged through the Meridian API. */
model JobStageAttempt {
  /** contains the pipeline stage associated with this job stage attempt. */
  stage: PipelineStage;

  /** specifies the attempt associated with this job stage attempt. */
  @minValue(1) attempt: integer;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** is the RFC 3339 UTC instant when processing began. */
  @oneOf startedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when processing reached a terminal state. */
  @oneOf finishedAt: Timestamp | null;

  /** specifies the error associated with this job stage attempt. */
  @oneOf error: ErrorResponse | null;
}

/** represents job data exchanged through the Meridian API. */
model Job {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the slug associated with this job. */
  tenantSlug: Slug;

  /** specifies the retry of job id associated with this job. */
  @oneOf retryOfJobId: Uuid | null;

  /** contains the job type associated with this job. */
  type: JobType;

  /** contains the job trigger associated with this job. */
  trigger: JobTrigger;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** specifies the stage associated with this job. */
  @oneOf stage: PipelineStage | null;

  /** contains the job scope type associated with this job. */
  scopeType: JobScopeType;

  /** specifies the scope id associated with this job. */
  scopeId: string | null;

  /** specifies the ref type associated with this job. */
  @oneOf refType: RefType | null;

  /** specifies the ref associated with this job. */
  @oneOf ref: RefName | null;

  /** Operation-specific non-secret result identifiers and counters. */
  result: Record<unknown> | null;

  /** specifies the progress associated with this job. */
  @minValue(0) @maxValue(100) progress: integer;

  /** indicates whether dirty applies to this job. */
  dirty: boolean;

  /** specifies the attempt associated with this job. */
  @minValue(0) attempt: integer;

  /** specifies the max attempts associated with this job. */
  @minValue(1) maxAttempts: integer;

  /** specifies the next attempt at associated with this job. */
  @oneOf nextAttemptAt: Timestamp | null;

  /** contains the ordered attempts associated with this job. */
  attempts: JobStageAttempt[];

  /** specifies the error associated with this job. */
  @oneOf error: ErrorResponse | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when processing began. */
  @oneOf startedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when processing reached a terminal state. */
  @oneOf finishedAt: Timestamp | null;

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;
}

/** contains one paginated page of job records. */
model JobPage {
  ...PageInfo;

  /** contains the ordered items associated with this job page. */
  items: Job[];
}

/** represents a job log event emitted by Meridian. */
model JobLogEvent {
  /** specifies the event associated with this job log event. */
  event: "log";

  /** uniquely identifies this resource. */
  id: string;

  /** contains the timestamp associated with this job log event. */
  at: Timestamp;

  /** specifies the message associated with this job log event. */
  message: string;

  /** specifies the stage associated with this job log event. */
  @oneOf stage?: PipelineStage | null;
}

/** represents a job state event emitted by Meridian. */
model JobStateEvent {
  /** specifies the event associated with this job state event. */
  event: "state";

  /** uniquely identifies this resource. */
  id: string;

  /** contains the timestamp associated with this job state event. */
  at: Timestamp;

  /** is the current lifecycle or processing state. */
  status: JobStatus;

  /** specifies the progress associated with this job state event. */
  @minValue(0) @maxValue(100) progress: integer;
}

/** represents a job sse event emitted by Meridian. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "event" })
@oneOf
union JobSseEvent {
  log: JobLogEvent,
  state: JobStateEvent,
}

/** defines validated input for the corresponding Meridian API operation. */
model LayerOrderReplaceRequest {
  /** contains the ordered layer ids associated with this layer order replace request. */
  @minItems(1) layerIds: Uuid[];
}

/** represents layer data exchanged through the Meridian API. */
model Layer {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** identifies the asset associated with this resource. */
  assetId: Uuid;

  /** specifies the source spec id associated with this layer. */
  @oneOf sourceSpecId: Uuid | null;

  /** contains the layer role associated with this layer. */
  role: LayerRole;

  /** contains the layer origin associated with this layer. */
  origin: LayerOrigin;

  /** specifies the ord associated with this layer. */
  @minValue(0) ord: integer;

  /** specifies the dialect associated with this layer. */
  dialect: string | null;

  /** indicates whether enabled applies to this layer. */
  enabled: boolean;

  /** contains the ordered heads associated with this layer. */
  heads: LayerHead[];

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of layer values. */
model LayerList {
  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the ordered items associated with this layer list. */
  items: Layer[];
}

/** defines validated input for the corresponding Meridian API operation. */
model LayerPatchRequest {
  /** contains the layer role associated with this layer patch request. */
  role?: LayerRole;

  /** specifies the dialect associated with this layer patch request. */
  dialect?: string | null;

  /** indicates whether enabled applies to this layer patch request. */
  enabled?: boolean;
}

/** represents layer revision data exchanged through the Meridian API. */
model LayerRevision {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the uuid associated with this layer revision. */
  layerId: Uuid;

  /** specifies the scope type associated with this layer revision. */
  scopeType: "ref" | "global";

  /** '*' for global, otherwise 'branch:<name>' or 'tag:<name>' */
  @minLength(1) @maxLength(260) scopeKey: string;

  /** specifies the content hash associated with this layer revision. */
  contentHash: string;

  /** contains the content type associated with this layer revision. */
  contentType: ContentType;

  /** specifies the content url associated with this layer revision. */
  @format("uri-reference") contentUrl?: string;

  /** specifies the dialect associated with this layer revision. */
  dialect: string | null;

  /** specifies the source branch associated with this layer revision. */
  sourceBranch: string | null;

  /** specifies the git commit associated with this layer revision. */
  gitCommit: string | null;

  /** specifies the created by associated with this layer revision. */
  @oneOf createdBy: Uuid | null;

  /** specifies the ai meta associated with this layer revision. */
  aiMeta: Record<unknown> | null;

  /** contains the revision status associated with this layer revision. */
  reviewStatus: RevisionStatus;

  /** specifies the review comment associated with this layer revision. */
  reviewComment: string | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of layer revision records. */
model LayerRevisionPage {
  ...PageInfo;

  /** contains the ordered items associated with this layer revision page. */
  items: LayerRevision[];
}

/** defines validated input for the corresponding Meridian API operation. */
model LayerRevisionCreateRequest {
  /** specifies the scope type associated with this layer revision create request. */
  scopeType: "ref" | "global";

  /** '*' for global, otherwise 'branch:<name>' or 'tag:<name>' */
  @minLength(1) @maxLength(260) scopeKey: string;

  /** specifies the expected effective revision id associated with this layer revision create request. */
  @oneOf expectedEffectiveRevisionId: Uuid | null;

  /** specifies the content associated with this layer revision create request. */
  @maxLength(1048576) content: string;

  /** contains the content type associated with this layer revision create request. */
  contentType: ContentType;

  /** specifies the dialect associated with this layer revision create request. */
  dialect?: string | null;

  /** indicates whether submit for review applies to this layer revision create request. */
  submitForReview?: boolean = true;
}

/** represents layer revision submission data exchanged through the Meridian API. */
model LayerRevisionSubmission {
  /** is the monotonic optimistic-concurrency version of this resource. */
  revision: LayerRevision;

  /** identifies the asynchronous job associated with this resource. */
  jobId: Uuid;

  /** indicates whether deduplicated applies to this layer revision submission. */
  deduplicated: boolean;
}

/** defines validated input for the corresponding Meridian API operation. */
model LayerRollbackRequest {
  /** specifies the scope type associated with this layer rollback request. */
  scopeType: "ref" | "global";

  /** '*' for global, otherwise 'branch:<name>' or 'tag:<name>' */
  @minLength(1) @maxLength(260) scopeKey: string;

  /** specifies the expected effective revision id associated with this layer rollback request. */
  @oneOf expectedEffectiveRevisionId: Uuid | null;

  /** contains the uuid associated with this layer rollback request. */
  targetRevisionId: Uuid;
}

/** represents review context data exchanged through the Meridian API. */
model ReviewContext {
  /** is the monotonic optimistic-concurrency version of this resource. */
  revision: LayerRevision;

  /** specifies the current effective revision associated with this review context. */
  @oneOf currentEffectiveRevision: LayerRevision | null;

  /** contains the diff result associated with this review context. */
  diff: DiffResult;

  /** specifies the author associated with this review context. */
  @oneOf author: User | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model RevisionApproveRequest {
  /** specifies the comment associated with this revision approve request. */
  @maxLength(2000) comment?: string | null;
}

/** reports the result of the corresponding Meridian operation. */
model RevisionReviewResult {
  /** is the monotonic optimistic-concurrency version of this resource. */
  revision: LayerRevision;

  /** specifies the effective revision id associated with this revision review result. */
  @oneOf effectiveRevisionId: Uuid | null;

  /** specifies the merge job id associated with this revision review result. */
  @oneOf mergeJobId: Uuid | null;

  /** contains the ordered superseded revision ids associated with this revision review result. */
  supersededRevisionIds: Uuid[];

  /** indicates whether deduplicated applies to this revision review result. */
  deduplicated: boolean;
}

/** defines validated input for the corresponding Meridian API operation. */
model RevisionRejectRequest {
  /** specifies the comment associated with this revision reject request. */
  @minLength(1) @maxLength(2000) comment: string;
}

/** defines supported filters for selecting review records. */
model ReviewFilter {
  /** is the current lifecycle or processing state. */
  status?: RevisionStatus[];

  /** identifies the service associated with this resource. */
  serviceId?: Uuid;

  /** contains the kind id associated with this review filter. */
  kind?: KindId;

  /** contains the layer origin associated with this review filter. */
  origin?: LayerOrigin;

  /** contains the uuid associated with this review filter. */
  authorId?: Uuid;
}

/** contains one paginated page of review records. */
model ReviewPage {
  ...PageInfo;

  /** contains the ordered items associated with this review page. */
  items: ReviewContext[];
}

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
model ViewOverride {
  /** specifies the view id associated with this view override. */
  viewId: string;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** contains the view override value associated with this view override. */
  override: ViewOverrideValue;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of view override values. */
model ViewOverrideList {
  /** contains the ordered items associated with this view override list. */
  items: ViewOverride[];
}

/** defines validated input for the corresponding Meridian API operation. */
model ViewOverridePutRequest {
  ...ViewOverrideValue;
}

/** represents view input spec data exchanged through the Meridian API. */
model ViewInputSpec {
  /** specifies the mode associated with this view input spec. */
  mode: "single" | "versions" | "collection" | "scope";

  /** specifies the kinds associated with this view input spec. */
  @oneOf kinds: "*" | KindId[];

  /** contains the ordered scopes associated with this view input spec. */
  scopes?: ("service" | "system_group" | "tenant")[];

  /** specifies the min docs associated with this view input spec. */
  @minValue(1) minDocs?: integer;

  /** specifies the max docs associated with this view input spec. */
  @minValue(1) @maxValue(50) maxDocs?: integer;

  /** indicates whether same kind applies to this view input spec. */
  sameKind?: boolean;

  /** indicates whether same asset applies to this view input spec. */
  sameAsset?: boolean;
}

/** represents view definition data exchanged through the Meridian API. */
model ViewDefinition {
  /** uniquely identifies this resource. */
  id: string;

  /** specifies the name key associated with this view definition. */
  nameKey: string;

  /** specifies the milestone associated with this view definition. */
  @pattern("^M[0-9]+$") milestone: string;

  /** specifies the mount associated with this view definition. */
  mount: "component" | "iframe" | "external";

  /** specifies the component associated with this view definition. */
  component?: string;

  /** specifies the entrypoint associated with this view definition. */
  entrypoint?: string;

  /** specifies the external url associated with this view definition. */
  externalUrl?: url;

  /** contains the view input spec associated with this view definition. */
  input: ViewInputSpec;

  /** contains the ordered item types associated with this view definition. */
  itemTypes?: string[];

  /** contains the ordered columns associated with this view definition. */
  columns?: Record<unknown>[];

  /** specifies the columns source associated with this view definition. */
  columnsSource?: string;

  /** contains the ordered fallback columns associated with this view definition. */
  fallbackColumns?: Record<unknown>[];

  /** specifies the query associated with this view definition. */
  query?: Record<unknown>;

  /** specifies the default options associated with this view definition. */
  defaultOptions: Record<unknown>;

  /** specifies the options schema associated with this view definition. */
  optionsSchema: Record<unknown>;
}

/** contains an ordered collection of view values. */
model ViewList {
  /** contains the ordered items associated with this view list. */
  items: ViewDefinition[];
}

/** identifies the scope selected by a request. */
model ScopeSelector {
  /** specifies the type associated with this scope selector. */
  type: "service" | "system_group" | "tenant";

  /** Must be null for tenant and non-null for service or system_group. */
  @oneOf id: Uuid | null;
}

/** Input arity and kind constraints are validated from the selected entry in views.yaml. */
model ViewResolveRequest {
  /** specifies the view id associated with this view resolve request. */
  viewId: string;

  /** Required by single, versions and collection views; omitted for scope views. */
  @maxItems(50) inputs?: DocumentSelector[] = #[];

  /** Required by scope views; omitted for document-input views. */
  @oneOf scope?: ScopeSelector | null;

  /** specifies the options associated with this view resolve request. */
  options?: Record<unknown>;
}

/** represents document view resolution data exchanged through the Meridian API. */
model DocumentViewResolution {
  /** specifies the kind associated with this document view resolution. */
  kind: "document";

  /** contains the view definition associated with this document view resolution. */
  view: ViewDefinition;

  /** contains the artifact link associated with this document view resolution. */
  document: ArtifactLink;
}

/** represents items view resolution data exchanged through the Meridian API. */
model ItemsViewResolution {
  /** specifies the kind associated with this items view resolution. */
  kind: "items";

  /** contains the view definition associated with this items view resolution. */
  view: ViewDefinition;

  /** contains the ordered items associated with this items view resolution. */
  items: AssetItem[];
}

/** represents graph node data exchanged through the Meridian API. */
model GraphNode {
  /** uniquely identifies this resource. */
  id: string;

  /** specifies the label associated with this graph node. */
  label: string;

  /** identifies the service associated with this resource. */
  serviceId: Uuid;
}

/** represents graph edge data exchanged through the Meridian API. */
model GraphEdge {
  /** uniquely identifies this resource. */
  id: string;

  /** specifies the from associated with this graph edge. */
  from: string;

  /** specifies the to associated with this graph edge. */
  to: string;

  /** specifies the details associated with this graph edge. */
  details: Record<unknown>;
}

/** represents graph view resolution data exchanged through the Meridian API. */
model GraphViewResolution {
  /** specifies the kind associated with this graph view resolution. */
  kind: "dep_graph";

  /** contains the view definition associated with this graph view resolution. */
  view: ViewDefinition;

  /** contains the ordered nodes associated with this graph view resolution. */
  nodes: GraphNode[];

  /** contains the ordered edges associated with this graph view resolution. */
  edges: GraphEdge[];

  /** indicates whether truncated applies to this graph view resolution. */
  truncated: boolean;
}

/** represents dashboard view resolution data exchanged through the Meridian API. */
model DashboardViewResolution {
  /** specifies the kind associated with this dashboard view resolution. */
  kind: "catalog_dashboard";

  /** contains the view definition associated with this dashboard view resolution. */
  view: ViewDefinition;

  /** specifies the metrics associated with this dashboard view resolution. */
  metrics: Record<unknown>;
}

/** represents view resolution data exchanged through the Meridian API. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
@oneOf
union ViewResolution {
  document: DocumentViewResolution,
  items: ItemsViewResolution,
  dep_graph: GraphViewResolution,
  catalog_dashboard: DashboardViewResolution,
}

/** represents share link data exchanged through the Meridian API. */
model ShareLink {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the resource type associated with this share link. */
  resourceType: "view" | "diff_snapshot";

  /** Diff snapshot ID; null for a frozen view descriptor. */
  @oneOf resourceId: Uuid | null;

  /** specifies the descriptor digest associated with this share link. */
  descriptorDigest: string;

  /** specifies the url associated with this share link. */
  @format("uri-reference") url: string;

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  expiresAt: Timestamp;

  /** specifies the revoked at associated with this share link. */
  @oneOf revokedAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of share link records. */
model ShareLinkPage {
  ...PageInfo;

  /** contains the ordered items associated with this share link page. */
  items: ShareLink[];
}

/** Creates a view share. The server resolves all ref selectors to immutable version IDs, freezes scope membership and options, and allowlists only the resulting artifacts. Diff snapshots use the dedicated diff-snapshot share operation. */
model ShareLinkCreateRequest {
  /** specifies the resource type associated with this share link create request. */
  resourceType: "view";

  /** specifies the view id associated with this share link create request. */
  @minLength(1) @maxLength(64) viewId: string;

  /** contains the ordered inputs associated with this share link create request. */
  @maxItems(50) inputs?: DocumentSelector[] = #[];

  /** specifies the scope associated with this share link create request. */
  @oneOf scope?: ScopeSelector | null;

  /** specifies the options associated with this share link create request. */
  options?: Record<unknown>;

  /** specifies the expires in seconds associated with this share link create request. */
  @minValue(300) @maxValue(2592000) expiresInSeconds: integer;
}

/** represents shared view data exchanged through the Meridian API. */
model SharedView {
  /** specifies the resource type associated with this shared view. */
  resourceType: "view" | "diff_snapshot";

  /** is the RFC 3339 UTC instant after which this value is invalid. */
  expiresAt: Timestamp;

  /** specifies the resolution associated with this shared view. */
  @oneOf resolution: ViewResolution | DiffSnapshot;
}

/** represents system group data exchanged through the Meridian API. */
model SystemGroup {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the parent id associated with this system group. */
  @oneOf parentId: Uuid | null;

  /** contains the slug associated with this system group. */
  slug: Slug;

  /** specifies the display name associated with this system group. */
  displayName: string;

  /** specifies the description associated with this system group. */
  description: string | null;

  /** contains the ordered service ids associated with this system group. */
  serviceIds: Uuid[];

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of system group values. */
model SystemGroupList {
  /** contains the ordered items associated with this system group list. */
  items: SystemGroup[];
}

/** defines validated input for the corresponding Meridian API operation. */
model SystemGroupCreateRequest {
  /** contains the slug associated with this system group create request. */
  slug: Slug;

  /** specifies the display name associated with this system group create request. */
  @minLength(1) @maxLength(128) displayName: string;

  /** specifies the description associated with this system group create request. */
  @maxLength(2000) description?: string | null;

  /** specifies the parent id associated with this system group create request. */
  @oneOf parentId?: Uuid | null;

  /** contains the ordered service ids associated with this system group create request. */
  serviceIds?: Uuid[];
}

/** defines validated input for the corresponding Meridian API operation. */
model SystemGroupPatchRequest {
  /** specifies the display name associated with this system group patch request. */
  @minLength(1) @maxLength(128) displayName?: string;

  /** specifies the description associated with this system group patch request. */
  @maxLength(2000) description?: string | null;

  /** specifies the parent id associated with this system group patch request. */
  @oneOf parentId?: Uuid | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model SystemGroupMembersReplaceRequest {
  /** contains the ordered service ids associated with this system group members replace request. */
  serviceIds: Uuid[];
}

/** defines validated input for the corresponding Meridian API operation. */
model PublicViewResolveRequest {
  /** specifies the view id associated with this public view resolve request. */
  viewId: "swagger-ui" | "redoc" | "source";

  /** contains the kind id associated with this public view resolve request. */
  kind: KindId;

  /** contains the asset name associated with this public view resolve request. */
  assetName: AssetName;

  /** specifies the options associated with this public view resolve request. */
  options?: Record<unknown>;
}


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
