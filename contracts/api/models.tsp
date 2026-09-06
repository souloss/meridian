/**
 * Compatibility facade for the complete shared model graph.
 *
 * The editable ownership lives in common technical modules and domain
 * `models.tsp` files. This facade keeps one stable OpenAPI component boundary
 * until external cross-domain refs and Go import-mapping are fully adopted.
 */
import "./common/foundation.tsp";
import "./common/errors.tsp";
import "./common/pagination.tsp";
import "./common/parameters.tsp";
import "./domains/asset/models.tsp";
import "./domains/auth/models.tsp";
import "./domains/collaboration/models.tsp";
import "./domains/diff/models.tsp";
import "./domains/job/models.tsp";
import "./domains/layer/models.tsp";
import "./domains/platform/models.tsp";
import "./domains/repository/models.tsp";
import "./domains/service/models.tsp";
import "./domains/tenant/models.tsp";
import "./domains/view/models.tsp";

@service
namespace MeridianApi;
