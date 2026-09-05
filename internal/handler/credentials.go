package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListCredentials returns tenant-visible credential metadata without secret material.
func (s *Server) ListCredentials(ctx context.Context, request api.ListCredentialsRequestObject) (api.ListCredentialsResponseObject, error) {
	if s.credentials == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.credentials.ListTenant(ctx, principal, request.TenantSlug, page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.Credential, 0, len(items))
	for _, item := range items {
		responses = append(responses, credentialResponse(item))
	}
	return api.ListCredentials200JSONResponse{CredentialPageJSONResponse: api.CredentialPageJSONResponse(api.CredentialPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	})}, nil
}

// CreateCredential encrypts and persists one tenant-owned credential.
func (s *Server) CreateCredential(ctx context.Context, request api.CreateCredentialRequestObject) (api.CreateCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input, err := tenantCredentialInput(*request.Body)
	if err != nil {
		return nil, err
	}
	created, err := s.credentials.CreateTenant(ctx, principal, request.TenantSlug, input)
	if err != nil {
		return nil, err
	}
	body := credentialResponse(created)
	return api.CreateCredential201JSONResponse{CredentialJSONResponse: api.CredentialJSONResponse{
		Body: body, Headers: api.CredentialResponseHeaders{ETag: new(body.Etag)},
	}}, nil
}

// UpdateCredential conditionally changes tenant credential metadata and sharing.
func (s *Server) UpdateCredential(ctx context.Context, request api.UpdateCredentialRequestObject) (api.UpdateCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := service.CredentialPatch{Name: request.Body.Name}
	if request.Body.SharedScope != nil {
		value := string(*request.Body.SharedScope)
		patch.SharedScope = &value
	}
	if request.Body.TeamIds != nil {
		teamIDs := make([]uuid.UUID, len(*request.Body.TeamIds))
		for index, teamID := range *request.Body.TeamIds {
			teamIDs[index] = serviceUUID(teamID)
		}
		patch.TeamIDs = &teamIDs
	}
	updated, err := s.credentials.UpdateTenant(ctx, principal, request.TenantSlug, serviceUUID(request.CredentialId), request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body := credentialResponse(updated)
	return api.UpdateCredential200JSONResponse{CredentialJSONResponse: api.CredentialJSONResponse{
		Body: body, Headers: api.CredentialResponseHeaders{ETag: new(body.Etag)},
	}}, nil
}

// DeleteCredential conditionally removes a tenant credential and supports forced unbinding.
func (s *Server) DeleteCredential(ctx context.Context, request api.DeleteCredentialRequestObject) (api.DeleteCredentialResponseObject, error) {
	if s.credentials == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	force := request.Params.Force != nil && *request.Params.Force
	if err := s.credentials.DeleteTenant(ctx, principal, request.TenantSlug, serviceUUID(request.CredentialId), request.Params.IfMatch, force); err != nil {
		return nil, err
	}
	return api.DeleteCredential204Response{}, nil
}

// RotateCredential replaces a tenant secret under the supplied ETag.
func (s *Server) RotateCredential(ctx context.Context, request api.RotateCredentialRequestObject) (api.RotateCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	secret, err := rotateSecret(request.Body.Secret)
	if err != nil {
		return nil, err
	}
	rotated, jobs, err := s.credentials.RotateTenant(ctx, principal, request.TenantSlug, serviceUUID(request.CredentialId), request.Params.IfMatch, service.CredentialRotation{
		Secret: secret, ResyncRepositories: request.Body.ResyncRepositories != nil && *request.Body.ResyncRepositories,
	})
	if err != nil {
		return nil, err
	}
	body := api.CredentialRotationResult{
		Credential: credentialResponse(rotated),
		Etag:       credentialResponse(rotated).Etag,
		SyncJobs:   syncJobResponses(jobs),
	}
	return api.RotateCredential200JSONResponse{CredentialRotationJSONResponse: api.CredentialRotationJSONResponse{
		Body: body, Headers: api.CredentialRotationResponseHeaders{ETag: new(body.Etag)},
	}}, nil
}

// ListGlobalCredentials returns platform credential metadata for a platform administrator.
func (s *Server) ListGlobalCredentials(ctx context.Context, request api.ListGlobalCredentialsRequestObject) (api.ListGlobalCredentialsResponseObject, error) {
	if s.credentials == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.credentials.ListGlobal(ctx, principal, page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.GlobalCredential, 0, len(items))
	for _, item := range items {
		responses = append(responses, globalCredentialResponse(item))
	}
	return api.ListGlobalCredentials200JSONResponse{GlobalCredentialPageJSONResponse: api.GlobalCredentialPageJSONResponse(api.GlobalCredentialPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	})}, nil
}

// CreateGlobalCredential encrypts and persists one platform-owned credential.
func (s *Server) CreateGlobalCredential(ctx context.Context, request api.CreateGlobalCredentialRequestObject) (api.CreateGlobalCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input, err := globalCredentialInput(*request.Body)
	if err != nil {
		return nil, err
	}
	created, err := s.credentials.CreateGlobal(ctx, principal, input)
	if err != nil {
		return nil, err
	}
	body := globalCredentialResponse(created)
	return api.CreateGlobalCredential201JSONResponse{GlobalCredentialJSONResponse: api.GlobalCredentialJSONResponse{
		Body: body, Headers: api.GlobalCredentialResponseHeaders{ETag: new(body.Etag)},
	}}, nil
}

// UpdateGlobalCredential conditionally changes a platform credential name.
func (s *Server) UpdateGlobalCredential(ctx context.Context, request api.UpdateGlobalCredentialRequestObject) (api.UpdateGlobalCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.credentials.UpdateGlobal(ctx, principal, serviceUUID(request.CredentialId), request.Params.IfMatch, request.Body.Name)
	if err != nil {
		return nil, err
	}
	body := globalCredentialResponse(updated)
	return api.UpdateGlobalCredential200JSONResponse{GlobalCredentialJSONResponse: api.GlobalCredentialJSONResponse{
		Body: body, Headers: api.GlobalCredentialResponseHeaders{ETag: new(body.Etag)},
	}}, nil
}

// DeleteGlobalCredential conditionally removes a platform credential and supports forced unbinding.
func (s *Server) DeleteGlobalCredential(ctx context.Context, request api.DeleteGlobalCredentialRequestObject) (api.DeleteGlobalCredentialResponseObject, error) {
	if s.credentials == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	force := request.Params.Force != nil && *request.Params.Force
	if err := s.credentials.DeleteGlobal(ctx, principal, serviceUUID(request.CredentialId), request.Params.IfMatch, force); err != nil {
		return nil, err
	}
	return api.DeleteGlobalCredential204Response{}, nil
}

// RotateGlobalCredential replaces a platform secret under the supplied ETag.
func (s *Server) RotateGlobalCredential(ctx context.Context, request api.RotateGlobalCredentialRequestObject) (api.RotateGlobalCredentialResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	secret, err := rotateSecret(request.Body.Secret)
	if err != nil {
		return nil, err
	}
	rotated, jobs, err := s.credentials.RotateGlobal(ctx, principal, serviceUUID(request.CredentialId), request.Params.IfMatch, service.CredentialRotation{
		Secret: secret, ResyncRepositories: request.Body.ResyncRepositories != nil && *request.Body.ResyncRepositories,
	})
	if err != nil {
		return nil, err
	}
	bodyCredential := globalCredentialResponse(rotated)
	body := api.GlobalCredentialRotationResult{Credential: bodyCredential, Etag: bodyCredential.Etag, SyncJobs: syncJobResponses(jobs)}
	return api.RotateGlobalCredential200JSONResponse{GlobalCredentialRotationJSONResponse: api.GlobalCredentialRotationJSONResponse{
		Body: body, Headers: api.GlobalCredentialRotationResponseHeaders{ETag: new(body.Etag)},
	}}, nil
}

// ListKnownHosts returns tenant-approved SSH host identities without stored public-key material.
func (s *Server) ListKnownHosts(ctx context.Context, request api.ListKnownHostsRequestObject) (api.ListKnownHostsResponseObject, error) {
	if s.credentials == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.credentials.ListKnownHosts(ctx, principal, request.TenantSlug, page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.KnownHost, 0, len(items))
	for _, item := range items {
		responses = append(responses, knownHostResponse(item))
	}
	return api.ListKnownHosts200JSONResponse{KnownHostPageJSONResponse: api.KnownHostPageJSONResponse(api.KnownHostPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	})}, nil
}

// CreateKnownHost validates and stores one manually approved SSH host key.
func (s *Server) CreateKnownHost(ctx context.Context, request api.CreateKnownHostRequestObject) (api.CreateKnownHostResponseObject, error) {
	if s.credentials == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.credentials.CreateKnownHost(ctx, principal, request.TenantSlug, request.Body.Host, request.Body.Port, request.Body.PublicKey)
	if err != nil {
		return nil, err
	}
	return api.CreateKnownHost201JSONResponse{KnownHostJSONResponse: api.KnownHostJSONResponse(knownHostResponse(created))}, nil
}

func tenantCredentialInput(body api.CredentialCreateRequest) (service.CredentialInput, error) {
	if sshInput, err := body.AsCredentialCreateRequest0(); err == nil && string(sshInput.Kind) == "ssh_key" {
		if sshInput.SshKey.PrivateKeyPem == nil {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{
			Name: sshInput.Name, Secret: service.CredentialSecret{Kind: "ssh_key", PrivateKey: *sshInput.SshKey.PrivateKeyPem, Passphrase: optionalString(sshInput.SshKey.Passphrase)},
			SharedScope: optionalSharedScope0(sshInput.SharedScope), TeamIDs: apiUUIDs(sshInput.TeamIds),
		}, nil
	}
	if httpInput, err := body.AsCredentialCreateRequest1(); err == nil && string(httpInput.Kind) == "http_token" {
		if httpInput.HttpToken.Token == nil {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{
			Name: httpInput.Name, Secret: service.CredentialSecret{Kind: "http_token", HTTPUsername: httpInput.HttpToken.Username, HTTPToken: *httpInput.HttpToken.Token},
			SharedScope: optionalSharedScope1(httpInput.SharedScope), TeamIDs: apiUUIDs(httpInput.TeamIds),
		}, nil
	}
	return service.CredentialInput{}, service.ErrValidation
}

func globalCredentialInput(body api.GlobalCredentialCreateRequest) (service.CredentialInput, error) {
	if sshInput, err := body.AsGlobalCredentialCreateRequest0(); err == nil && string(sshInput.Kind) == "ssh_key" {
		if sshInput.SshKey.PrivateKeyPem == nil {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{Name: sshInput.Name, Secret: service.CredentialSecret{
			Kind: "ssh_key", PrivateKey: *sshInput.SshKey.PrivateKeyPem, Passphrase: optionalString(sshInput.SshKey.Passphrase),
		}}, nil
	}
	if httpInput, err := body.AsGlobalCredentialCreateRequest1(); err == nil && string(httpInput.Kind) == "http_token" {
		if httpInput.HttpToken.Token == nil {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{Name: httpInput.Name, Secret: service.CredentialSecret{
			Kind: "http_token", HTTPUsername: httpInput.HttpToken.Username, HTTPToken: *httpInput.HttpToken.Token,
		}}, nil
	}
	return service.CredentialInput{}, service.ErrValidation
}

func rotateSecret(value api.CredentialRotateRequest_Secret) (service.CredentialSecret, error) {
	if sshInput, err := value.AsSshSecretInput(); err == nil && sshInput.PrivateKeyPem != nil {
		return service.CredentialSecret{Kind: "ssh_key", PrivateKey: *sshInput.PrivateKeyPem, Passphrase: optionalString(sshInput.Passphrase)}, nil
	}
	if httpInput, err := value.AsHttpSecretInput(); err == nil && httpInput.Token != nil {
		return service.CredentialSecret{Kind: "http_token", HTTPUsername: httpInput.Username, HTTPToken: *httpInput.Token}, nil
	}
	return service.CredentialSecret{}, service.ErrValidation
}

func credentialResponse(record service.CredentialRecord) api.Credential {
	etagKind := "credential"
	if record.IsGlobal {
		etagKind = "global-credential"
	}
	return api.Credential{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKind, record.ID.String(), record.Revision), Name: record.Name,
		Kind: api.CredentialKind(record.Kind), Fingerprint: record.Encrypted.Fingerprint, SharedScope: api.CredentialSharedScope(record.SharedScope),
		TeamIds: uuidResponses(record.TeamIDs), IsGlobal: record.IsGlobal, CreatedBy: api.Uuid(record.CreatedBy),
		LastUsedAt: nullableTime(record.LastUsedAt), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func globalCredentialResponse(record service.GlobalCredentialRecord) api.GlobalCredential {
	return api.GlobalCredential{
		Id: api.Uuid(record.ID), Etag: revisionETag("global-credential", record.ID.String(), record.Revision), Name: record.Name,
		Kind: api.CredentialKind(record.Kind), Fingerprint: record.Encrypted.Fingerprint, CreatedBy: api.Uuid(record.CreatedBy),
		LastUsedAt: nullableTime(record.LastUsedAt), Revision: int(record.Revision), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func knownHostResponse(record service.KnownHostRecord) api.KnownHost {
	return api.KnownHost{Id: api.Uuid(record.ID), Host: record.Host, Port: int(record.Port), KeyType: api.KnownHostKeyType(record.KeyType), Fingerprint: record.Fingerprint, Source: api.KnownHostSource(record.Source), CreatedAt: record.CreatedAt}
}

func syncJobResponses(jobs []service.CredentialSyncJob) []api.CredentialSyncJob {
	responses := make([]api.CredentialSyncJob, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, api.CredentialSyncJob{TenantSlug: api.Slug(job.TenantSlug), RepositoryId: api.Uuid(job.RepositoryID), JobId: api.Uuid(job.JobID), Deduplicated: job.Deduplicated})
	}
	return responses
}

func optionalSharedScope0(value *api.CredentialCreateRequest0SharedScope) string {
	if value == nil {
		return "private"
	}
	return string(*value)
}

func optionalSharedScope1(value *api.CredentialCreateRequest1SharedScope) string {
	if value == nil {
		return "private"
	}
	return string(*value)
}

func apiUUIDs(value *[]api.Uuid) []uuid.UUID {
	if value == nil {
		return nil
	}
	ids := make([]uuid.UUID, len(*value))
	for index, item := range *value {
		ids[index] = serviceUUID(item)
	}
	return ids
}

func optionalString(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

func uuidResponses(values []uuid.UUID) []api.Uuid {
	responses := make([]api.Uuid, len(values))
	for index, value := range values {
		responses[index] = api.Uuid(value)
	}
	return responses
}
