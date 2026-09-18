package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// TestCredential 仅为一次有边界的连接探测解密一个租户可见凭据。
func (s *Server) TestCredential(ctx context.Context, request tenant.TestCredentialRequestObject) (tenant.TestCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.credentials.TestTenant(ctx, principal, request.TenantSlug, serviceUUID(request.CredentialId), request.Body.RepositoryUrl)
	if err != nil {
		return nil, err
	}
	return tenant.TestCredential200JSONResponse(connectionTestResponse(result)), nil
}

// TestGlobalCredential 仅为一次有边界的连接探测解密一个平台凭据。
func (s *Server) TestGlobalCredential(ctx context.Context, request platform.TestGlobalCredentialRequestObject) (platform.TestGlobalCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.credentials.TestGlobal(ctx, principal, serviceUUID(request.CredentialId), request.Body.RepositoryUrl)
	if err != nil {
		return nil, err
	}
	return platform.TestGlobalCredential200JSONResponse(connectionTestResponse(result)), nil
}

// CheckRepositoryConnection 在仓库持久化之前探测远端连通性。
func (s *Server) CheckRepositoryConnection(ctx context.Context, request repository.CheckRepositoryConnectionRequestObject) (repository.CheckRepositoryConnectionResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var credentialID *uuid.UUID
	if request.Body.CredentialId.IsSpecified() && !request.Body.CredentialId.IsNull() {
		value := serviceUUID(request.Body.CredentialId.MustGet())
		credentialID = new(value)
	}
	result, err := s.credentials.CheckRepositoryConnection(ctx, principal, request.TenantSlug, request.Body.Url, credentialID)
	if err != nil {
		return nil, err
	}
	return repository.CheckRepositoryConnection200JSONResponse(connectionTestResponse(result)), nil
}

// connectionTestResponse 将连接探测结果投影为 API 形状。
func connectionTestResponse(result service.ConnectionTestResult) api.ConnectionTest {
	errorClass := nullable.NewNullNullable[api.ConnectionTestErrorClass]()
	if result.ErrorClass != "" {
		errorClass = nullable.NewNullableWithValue(api.ConnectionTestErrorClass(result.ErrorClass))
	}
	hostKeyCandidate := nullable.NewNullNullable[api.KnownHostCandidate]()
	if result.HostKeyCandidate != nil {
		hostKeyCandidate = nullable.NewNullableWithValue(api.KnownHostCandidate{
			Host: result.HostKeyCandidate.Host, Port: result.HostKeyCandidate.Port,
			KeyType:   api.KnownHostCandidateKeyType(result.HostKeyCandidate.KeyType),
			PublicKey: result.HostKeyCandidate.PublicKey, Fingerprint: result.HostKeyCandidate.Fingerprint,
		})
	}
	return api.ConnectionTest{Ok: result.OK, ErrorClass: errorClass, Message: result.Message, HostKeyCandidate: hostKeyCandidate}
}
