import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";
import "./common/models.tsp";
import "./domains/asset.tsp";
import "./domains/auth.tsp";
import "./domains/collaboration.tsp";
import "./domains/diff.tsp";
import "./domains/job.tsp";
import "./domains/layer.tsp";
import "./domains/platform.tsp";
import "./domains/repository.tsp";
import "./domains/service.tsp";
import "./domains/system.tsp";
import "./domains/tenant.tsp";
import "./domains/view.tsp";

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
