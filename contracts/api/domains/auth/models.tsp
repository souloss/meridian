/** Shared authentication and user preference wire models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

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
