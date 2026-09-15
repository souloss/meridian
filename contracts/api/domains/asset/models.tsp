/** Shared asset and source models used across asset, repository, service, and diff operations. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** identifies an asset kind registered in contracts/kinds.yaml. */
@pattern("^[a-z][a-z0-9_]{0,63}$")
scalar KindId extends string;

/** contains an ordered collection of capability values. */
model CapabilityList is string[];

/** identifies one supported source mode value. */
enum SourceMode {
  builtin,
  command,
  push,
  manual,
  ai,
}

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

/** defines validated input for the corresponding Meridian API operation. */
model AssetKindPatchRequest {
  /** indicates whether enabled applies to this asset kind patch request. */
  enabled: boolean;
}

/** represents asset name template data exchanged through the Meridian API. */
@maxLength(128)
@pattern("^(?:[a-z0-9-]|\\{file_stem\\}|\\{parent_dir\\})+$")
scalar AssetNameTemplate extends string;

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
