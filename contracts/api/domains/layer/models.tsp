/** Shared layer, revision, and review models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

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
