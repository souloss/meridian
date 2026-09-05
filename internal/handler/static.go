package handler

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	meridian "github.com/meridian-labs/meridian"
)

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		s.notFound(w, r)
		return
	}
	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = "index.html"
	}
	if fileExists(s.assets, requested) {
		setStaticCacheHeader(w, requested)
		http.ServeFileFS(w, r, s.assets, requested)
		return
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && mime.TypeByExtension(path.Ext(requested)) == "" && fileExists(s.assets, "index.html") {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, s.assets, "index.html")
		return
	}
	http.NotFound(w, r)
}

func setStaticCacheHeader(w http.ResponseWriter, name string) {
	if name == "index.html" || strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}
	if strings.HasPrefix(name, "_nuxt/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

func staticAssets() fs.FS {
	assets, err := fs.Sub(meridian.StaticAssets, "web/.output/public")
	if err == nil && fileExists(assets, "index.html") {
		return assets
	}
	fallback, err := fs.Sub(meridian.FallbackAssets, "web/fallback")
	if err != nil {
		panic("embedded fallback assets are unavailable")
	}
	return fallback
}

func fileExists(assets fs.FS, name string) bool {
	_, err := fs.Stat(assets, name)
	return err == nil
}
