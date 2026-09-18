package handler

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	meridian "github.com/meridian-labs/meridian"
)

const (
	// indexHTMLName 是静态站点入口文件名。
	indexHTMLName = "index.html"
	// nuxtAssetsPrefix 是 Nuxt 构建产物目录前缀。
	nuxtAssetsPrefix = "_nuxt/"
	// htmlSuffix 是 HTML 文件的后缀。
	htmlSuffix = ".html"
)

// static 服务前端静态站点：API 路径外，命中文件直接返回，
// 未命中且请求非资源文件时回退到 SPA 入口 index.html。
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		s.notFound(w, r)
		return
	}
	requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if requested == "." || requested == "" {
		requested = indexHTMLName
	}
	if fileExists(s.assets, requested) {
		setStaticCacheHeader(w, requested)
		http.ServeFileFS(w, r, s.assets, requested)
		return
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && mime.TypeByExtension(path.Ext(requested)) == "" && fileExists(s.assets, indexHTMLName) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, s.assets, indexHTMLName)
		return
	}
	http.NotFound(w, r)
}

// setStaticCacheHeader 按文件类型设置静态资源缓存头：HTML 不缓存，Nuxt 产物永久缓存。
func setStaticCacheHeader(w http.ResponseWriter, name string) {
	if name == indexHTMLName || strings.HasSuffix(name, htmlSuffix) {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}
	if strings.HasPrefix(name, nuxtAssetsPrefix) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

// staticAssets 解析内嵌静态资源：优先 Nuxt 产物，回退到 fallback 站点。
func staticAssets() fs.FS {
	assets, err := fs.Sub(meridian.StaticAssets, "web/.output/public")
	if err == nil && fileExists(assets, indexHTMLName) {
		return assets
	}
	fallback, err := fs.Sub(meridian.FallbackAssets, "web/fallback")
	if err != nil {
		panic("embedded fallback assets are unavailable")
	}
	return fallback
}

// fileExists 报告指定名称是否存在于给定文件系统。
func fileExists(assets fs.FS, name string) bool {
	_, err := fs.Stat(assets, name)
	return err == nil
}
