package handler

import (
	"encoding/json/v2"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/meridian-labs/meridian/internal/i18n"
)

// requestIDHeader 把 chi 请求中间件生成的请求 ID 写入响应头。
func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(middleware.RequestIDHeader, middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}

// notFound 返回统一的 404 错误响应（资源不存在或无权访问）。
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, errorCodeNotFound, nil)
}

// writeError 按请求语言渲染错误码对应的消息并返回错误响应。
func writeError(w http.ResponseWriter, r *http.Request, status int, code string, details map[string]any) {
	writeErrorDetails(w, r, status, code, details, nil)
}

// writeErrorDetails 渲染错误响应信封：code 稳定不变，message 按 Accept-Language
// 本地化并支持模板变量（通过 templateData 传入）。
func writeErrorDetails(w http.ResponseWriter, r *http.Request, status int, code string, details, templateData map[string]any) {
	message := localize(code, i18n.Negotiate(r.Header.Get(i18n.HeaderAcceptLanguage)), templateData)
	payload := struct {
		Code      string         `json:"code"`
		Details   map[string]any `json:"details,omitempty"`
		Message   string         `json:"message"`
		RequestID string         `json:"requestId"`
	}{
		Code: code, Details: details, Message: message, RequestID: middleware.GetReqID(r.Context()),
	}
	writeJSON(w, status, payload)
}

// writeJSON 写入一个 JSON 响应体并设置 Content-Type。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, payload)
}
