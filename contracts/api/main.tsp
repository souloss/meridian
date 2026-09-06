import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";
import "./common/models.tsp";
import "./domains/asset/routes.tsp";
import "./domains/auth/routes.tsp";
import "./domains/collaboration/routes.tsp";
import "./domains/diff/routes.tsp";
import "./domains/job/routes.tsp";
import "./domains/layer/routes.tsp";
import "./domains/platform/routes.tsp";
import "./domains/repository/routes.tsp";
import "./domains/service/routes.tsp";
import "./domains/system/routes.tsp";
import "./domains/tenant/routes.tsp";
import "./domains/view/routes.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service(#{ title: "Meridian API" })
@info(#{ version: "1.0.0" })
@server("/")
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "meridian_session"> | BearerAuth)
@tagMetadata(#[
  #{ name: "System" },
  #{ name: "Auth" },
  #{ name: "Platform" },
  #{ name: "Tenant" },
  #{ name: "Repository" },
  #{ name: "Service" },
  #{ name: "Asset" },
  #{ name: "Layer" },
  #{ name: "Diff" },
  #{ name: "View" },
  #{ name: "Job" },
  #{ name: "Collaboration" }
])
namespace MeridianApi;
