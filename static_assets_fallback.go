//go:build !embed_web

package meridian

// StaticAssets keeps backend-only development and tests independent of Node.
var StaticAssets = FallbackAssets
