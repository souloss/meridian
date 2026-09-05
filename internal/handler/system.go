package handler

import (
	"io/fs"
	"net/http"

	meridian "github.com/meridian-labs/meridian"
	"github.com/meridian-labs/meridian/internal/buildinfo"
	"github.com/meridian-labs/meridian/internal/generated/api"
)

func (s *Server) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.Health{Status: api.HealthStatusOk})
}

func (s *Server) Readyz(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeError(w, r, http.StatusServiceUnavailable, "internal_error", "service is not ready")
		return
	}
	writeJSON(w, http.StatusOK, api.Health{Status: api.HealthStatusOk})
}

func (s *Server) GetOpenApiContract(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(meridian.OpenAPIContract, "contracts/openapi.yaml")
	if err != nil {
		http.Error(w, "contract unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) GetVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.VersionInfo{
		ServerVersion:   buildinfo.CurrentVersion(),
		ApiVersion:      api.V1,
		ContractVersion: api.N100,
		BuildCommit:     buildinfo.CurrentCommit(),
	})
}
