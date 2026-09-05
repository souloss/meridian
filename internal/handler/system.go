package handler

import (
	"bytes"
	"context"
	"io/fs"

	meridian "github.com/meridian-labs/meridian"
	"github.com/meridian-labs/meridian/internal/buildinfo"
	"github.com/meridian-labs/meridian/internal/generated/api"
)

func (s *Server) Healthz(context.Context, api.HealthzRequestObject) (api.HealthzResponseObject, error) {
	return api.Healthz200JSONResponse{HealthJSONResponse: api.HealthJSONResponse(api.Health{Status: api.HealthStatusOk})}, nil
}

func (s *Server) Readyz(context.Context, api.ReadyzRequestObject) (api.ReadyzResponseObject, error) {
	if !s.ready.Load() {
		return api.Readyz503JSONResponse{}, nil
	}
	return api.Readyz200JSONResponse{HealthJSONResponse: api.HealthJSONResponse(api.Health{Status: api.HealthStatusOk})}, nil
}

func (s *Server) GetOpenApiContract(context.Context, api.GetOpenApiContractRequestObject) (api.GetOpenApiContractResponseObject, error) {
	data, err := fs.ReadFile(meridian.OpenAPIContract, "contracts/openapi.yaml")
	if err != nil {
		return nil, err
	}
	return api.GetOpenApiContract200ApplicationyamlResponse{
		Body:          bytes.NewReader(data),
		ContentLength: int64(len(data)),
	}, nil
}

func (s *Server) GetVersion(context.Context, api.GetVersionRequestObject) (api.GetVersionResponseObject, error) {
	return api.GetVersion200JSONResponse(api.VersionInfo{
		ServerVersion:   buildinfo.CurrentVersion(),
		ApiVersion:      api.V1,
		ContractVersion: api.N100,
		BuildCommit:     buildinfo.CurrentCommit(),
	}), nil
}
