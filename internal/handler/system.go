package handler

import (
	"bytes"
	"context"
	"io/fs"

	meridian "github.com/meridian-labs/meridian"
	"github.com/meridian-labs/meridian/internal/buildinfo"
	"github.com/meridian-labs/meridian/internal/generated/api"
	system "github.com/meridian-labs/meridian/internal/generated/api/system"
)

func (s *Server) Healthz(context.Context, system.HealthzRequestObject) (system.HealthzResponseObject, error) {
	return system.Healthz200JSONResponse(api.Health{Status: api.HealthStatusOk}), nil
}

func (s *Server) Readyz(context.Context, system.ReadyzRequestObject) (system.ReadyzResponseObject, error) {
	if !s.ready.Load() {
		return system.Readyz503JSONResponse{}, nil
	}
	return system.Readyz200JSONResponse(api.Health{Status: api.HealthStatusOk}), nil
}

func (s *Server) GetOpenApiContract(context.Context, system.GetOpenApiContractRequestObject) (system.GetOpenApiContractResponseObject, error) {
	data, err := fs.ReadFile(meridian.OpenAPIContract, "contracts/openapi.yaml")
	if err != nil {
		return nil, err
	}
	return system.GetOpenApiContract200ApplicationyamlResponse{
		Body:          bytes.NewReader(data),
		ContentLength: int64(len(data)),
	}, nil
}

func (s *Server) GetVersion(context.Context, system.GetVersionRequestObject) (system.GetVersionResponseObject, error) {
	return system.GetVersion200JSONResponse(api.VersionInfo{
		ServerVersion:   buildinfo.CurrentVersion(),
		ApiVersion:      api.V1,
		ContractVersion: api.N100,
		BuildCommit:     buildinfo.CurrentCommit(),
	}), nil
}
