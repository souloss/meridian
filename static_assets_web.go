//go:build embed_web

package meridian

import "embed"

//go:embed all:web/.output/public
var StaticAssets embed.FS
