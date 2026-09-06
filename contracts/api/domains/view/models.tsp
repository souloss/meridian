/** Shared view, resolution, sharing, and system group models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

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
