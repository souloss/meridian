package meridian

import "embed"

// Contract and static assets are embedded from the repository root so the
// production binary does not depend on the source checkout.
var (
	//go:embed contracts/openapi.yaml
	OpenAPIContract embed.FS
	//go:embed web/fallback
	FallbackAssets embed.FS
)
