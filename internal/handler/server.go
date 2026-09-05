package handler

import (
	"io/fs"
	"net/http"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/meridian-labs/meridian/internal/generated/api"
)

type Server struct {
	api.Unimplemented
	ready  atomic.Bool
	assets fs.FS
}

func New() *Server {
	s := &Server{assets: staticAssets()}
	s.ready.Store(true)
	return s
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	api.HandlerFromMux(s, r)
	r.NotFound(s.static)
	return r
}
