package handler

import (
	"encoding/json/v2"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(middleware.RequestIDHeader, middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "not_found", "resource not found")
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeErrorDetails(w, r, status, code, message, nil)
}

func writeErrorDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, payload)
}
