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

// Healthz 返回存活探针响应（进程健康即 OK）。
func (s *Server) Healthz(context.Context, system.HealthzRequestObject) (system.HealthzResponseObject, error) {
	return system.Healthz200JSONResponse(api.Health{Status: api.HealthStatusOk}), nil
}

// Readyz 返回就绪探针响应：仅当服务就绪时为 200，否则为 503。
func (s *Server) Readyz(context.Context, system.ReadyzRequestObject) (system.ReadyzResponseObject, error) {
	if !s.ready.Load() {
		return system.Readyz503JSONResponse{}, nil
	}
	return system.Readyz200JSONResponse(api.Health{Status: api.HealthStatusOk}), nil
}

// GetOpenApiContract 返回内嵌的 OpenAPI 契约文档。
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

// GetVersion 返回服务版本信息。
func (s *Server) GetVersion(context.Context, system.GetVersionRequestObject) (system.GetVersionResponseObject, error) {
	return system.GetVersion200JSONResponse(api.VersionInfo{
		ServerVersion:   buildinfo.CurrentVersion(),
		ApiVersion:      api.V1,
		ContractVersion: api.N100,
		BuildCommit:     buildinfo.CurrentCommit(),
	}), nil
}
