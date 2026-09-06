/**
 * Facade for the shared TypeSpec model graph.
 *
 * Keep this import boundary stable: domains and main.tsp import this file,
 * while the implementation is organized into responsibility-focused modules.
 */
import "./foundation.tsp";
import "./assets.tsp";
import "./auth.tsp";
import "./collaboration.tsp";
import "./diff.tsp";
import "./jobs.tsp";
import "./layers.tsp";
import "./platform.tsp";
import "./repository.tsp";
import "./service.tsp";
import "./tenant.tsp";
import "./views.tsp";
import "./parameters.tsp";

@service
namespace MeridianApi;
