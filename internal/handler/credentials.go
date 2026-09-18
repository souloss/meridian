package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json/v2"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

const (
	// etagKindCredential 是租户凭据 ETag 的实体类型令牌。
	etagKindCredential = "credential"
	// etagKindGlobalCredential 是平台凭据 ETag 的实体类型令牌。
	etagKindGlobalCredential = "global-credential"
	// operationRotateCredential 是租户凭据轮换的幂等摘要操作名。
	operationRotateCredential = "rotateCredential"
	// operationRotateGlobalCredential 是平台凭据轮换的幂等摘要操作名。
	operationRotateGlobalCredential = "rotateGlobalCredential"
)

// ListCredentials 返回租户可见的凭据元数据（不含秘密材料）。
func (s *Server) ListCredentials(ctx context.Context, request tenant.ListCredentialsRequestObject) (tenant.ListCredentialsResponseObject, error) {
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
	return tenant.ListCredentials200JSONResponse(api.CredentialPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// CreateCredential 加密并持久化一个租户自有凭据。
func (s *Server) CreateCredential(ctx context.Context, request tenant.CreateCredentialRequestObject) (tenant.CreateCredentialResponseObject, error) {
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
	return tenant.CreateCredential201JSONResponse{Body: body, Headers: tenant.CreateCredential201ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// UpdateCredential 在条件约束下修改租户凭据的元数据与共享范围。
func (s *Server) UpdateCredential(ctx context.Context, request tenant.UpdateCredentialRequestObject) (tenant.UpdateCredentialResponseObject, error) {
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
	return tenant.UpdateCredential200JSONResponse{Body: body, Headers: tenant.UpdateCredential200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// DeleteCredential 在条件约束下删除一个租户凭据，并支持强制解绑。
func (s *Server) DeleteCredential(ctx context.Context, request tenant.DeleteCredentialRequestObject) (tenant.DeleteCredentialResponseObject, error) {
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
	return tenant.DeleteCredential204Response{}, nil
}

// RotateCredential 在提供的 ETag 下替换租户凭据的秘密。
func (s *Server) RotateCredential(ctx context.Context, request tenant.RotateCredentialRequestObject) (tenant.RotateCredentialResponseObject, error) {
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
	requestHash, err := credentialRotationRequestHash(operationRotateCredential, request.TenantSlug, serviceUUID(request.CredentialId), request.Params.IfMatch, secret, request.Body.ResyncRepositories != nil && *request.Body.ResyncRepositories)
	if err != nil {
		return nil, service.ErrValidation
	}
	rotated, jobs, err := s.credentials.RotateTenant(ctx, principal, request.TenantSlug, serviceUUID(request.CredentialId), request.Params.IfMatch, service.CredentialRotation{
		Secret: secret, ResyncRepositories: request.Body.ResyncRepositories != nil && *request.Body.ResyncRepositories,
		IdempotencyKey: serviceUUID(api.Uuid(request.Params.IdempotencyKey)), RequestHash: requestHash,
	})
	if err != nil {
		return nil, err
	}
	body := api.CredentialRotationResult{
		Credential: credentialResponse(rotated),
		Etag:       credentialResponse(rotated).Etag,
		SyncJobs:   syncJobResponses(jobs),
	}
	return tenant.RotateCredential200JSONResponse{Body: body, Headers: tenant.RotateCredential200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// ListGlobalCredentials 为平台管理员返回平台凭据元数据。
func (s *Server) ListGlobalCredentials(ctx context.Context, request platform.ListGlobalCredentialsRequestObject) (platform.ListGlobalCredentialsResponseObject, error) {
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
	return platform.ListGlobalCredentials200JSONResponse(api.GlobalCredentialPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// CreateGlobalCredential 加密并持久化一个平台自有凭据。
func (s *Server) CreateGlobalCredential(ctx context.Context, request platform.CreateGlobalCredentialRequestObject) (platform.CreateGlobalCredentialResponseObject, error) {
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
	return platform.CreateGlobalCredential201JSONResponse{Body: body, Headers: platform.CreateGlobalCredential201ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// UpdateGlobalCredential 在条件约束下修改平台凭据的名称。
func (s *Server) UpdateGlobalCredential(ctx context.Context, request platform.UpdateGlobalCredentialRequestObject) (platform.UpdateGlobalCredentialResponseObject, error) {
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
	return platform.UpdateGlobalCredential200JSONResponse{Body: body, Headers: platform.UpdateGlobalCredential200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// DeleteGlobalCredential 在条件约束下删除一个平台凭据，并支持强制解绑。
func (s *Server) DeleteGlobalCredential(ctx context.Context, request platform.DeleteGlobalCredentialRequestObject) (platform.DeleteGlobalCredentialResponseObject, error) {
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
	return platform.DeleteGlobalCredential204Response{}, nil
}

// RotateGlobalCredential 在提供的 ETag 下替换平台凭据的秘密。
func (s *Server) RotateGlobalCredential(ctx context.Context, request platform.RotateGlobalCredentialRequestObject) (platform.RotateGlobalCredentialResponseObject, error) {
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
	requestHash, err := credentialRotationRequestHash(operationRotateGlobalCredential, "", serviceUUID(request.CredentialId), request.Params.IfMatch, secret, request.Body.ResyncRepositories != nil && *request.Body.ResyncRepositories)
	if err != nil {
		return nil, service.ErrValidation
	}
	rotated, jobs, err := s.credentials.RotateGlobal(ctx, principal, serviceUUID(request.CredentialId), request.Params.IfMatch, service.CredentialRotation{
		Secret: secret, ResyncRepositories: request.Body.ResyncRepositories != nil && *request.Body.ResyncRepositories,
		IdempotencyKey: serviceUUID(api.Uuid(request.Params.IdempotencyKey)), RequestHash: requestHash,
	})
	if err != nil {
		return nil, err
	}
	bodyCredential := globalCredentialResponse(rotated)
	body := api.GlobalCredentialRotationResult{Credential: bodyCredential, Etag: bodyCredential.Etag, SyncJobs: syncJobResponses(jobs)}
	return platform.RotateGlobalCredential200JSONResponse{Body: body, Headers: platform.RotateGlobalCredential200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// ListKnownHosts 返回租户已批准的 SSH 主机身份（不含存储的公钥材料）。
func (s *Server) ListKnownHosts(ctx context.Context, request tenant.ListKnownHostsRequestObject) (tenant.ListKnownHostsResponseObject, error) {
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
	return tenant.ListKnownHosts200JSONResponse(api.KnownHostPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// CreateKnownHost 校验并存储一个手动批准的 SSH 主机公钥。
func (s *Server) CreateKnownHost(ctx context.Context, request tenant.CreateKnownHostRequestObject) (tenant.CreateKnownHostResponseObject, error) {
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
	return tenant.CreateKnownHost201JSONResponse(knownHostResponse(created)), nil
}

// tenantCredentialInput 将租户凭据创建请求的联合体投影为服务层输入。
func tenantCredentialInput(body api.CredentialCreateRequest) (service.CredentialInput, error) {
	if sshInput, err := body.AsCredentialCreateRequest0(); err == nil && string(sshInput.Kind) == string(api.CredentialKindSshKey) {
		if sshInput.SshKey.PrivateKeyPem == "" {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{
			Name: sshInput.Name, Secret: service.CredentialSecret{Kind: string(api.CredentialKindSshKey), PrivateKey: sshInput.SshKey.PrivateKeyPem, Passphrase: optionalString(sshInput.SshKey.Passphrase)},
			SharedScope: optionalSharedScope0(sshInput.SharedScope), TeamIDs: apiUUIDs(sshInput.TeamIds),
		}, nil
	}
	if httpInput, err := body.AsCredentialCreateRequest1(); err == nil && string(httpInput.Kind) == string(api.CredentialKindHttpToken) {
		if httpInput.HttpToken.Token == "" {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{
			Name: httpInput.Name, Secret: service.CredentialSecret{Kind: string(api.CredentialKindHttpToken), HTTPUsername: httpInput.HttpToken.Username, HTTPToken: httpInput.HttpToken.Token},
			SharedScope: optionalSharedScope1(httpInput.SharedScope), TeamIDs: apiUUIDs(httpInput.TeamIds),
		}, nil
	}
	return service.CredentialInput{}, service.ErrValidation
}

// globalCredentialInput 将平台凭据创建请求的联合体投影为服务层输入。
func globalCredentialInput(body api.GlobalCredentialCreateRequest) (service.CredentialInput, error) {
	if sshInput, err := body.AsGlobalCredentialCreateRequest0(); err == nil && string(sshInput.Kind) == string(api.CredentialKindSshKey) {
		if sshInput.SshKey.PrivateKeyPem == "" {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{Name: sshInput.Name, SharedScope: string(api.CredentialSharedScopePrivate), Secret: service.CredentialSecret{
			Kind: string(api.CredentialKindSshKey), PrivateKey: sshInput.SshKey.PrivateKeyPem, Passphrase: optionalString(sshInput.SshKey.Passphrase),
		}}, nil
	}
	if httpInput, err := body.AsGlobalCredentialCreateRequest1(); err == nil && string(httpInput.Kind) == string(api.CredentialKindHttpToken) {
		if httpInput.HttpToken.Token == "" {
			return service.CredentialInput{}, service.ErrValidation
		}
		return service.CredentialInput{Name: httpInput.Name, SharedScope: string(api.CredentialSharedScopePrivate), Secret: service.CredentialSecret{
			Kind: string(api.CredentialKindHttpToken), HTTPUsername: httpInput.HttpToken.Username, HTTPToken: httpInput.HttpToken.Token,
		}}, nil
	}
	return service.CredentialInput{}, service.ErrValidation
}

// rotateSecret 将凭据轮换请求的秘密联合体投影为服务层秘密输入。
func rotateSecret(value api.CredentialRotateRequest_Secret) (service.CredentialSecret, error) {
	if sshInput, err := value.AsSshSecretInput(); err == nil && sshInput.PrivateKeyPem != "" {
		return service.CredentialSecret{Kind: string(api.CredentialKindSshKey), PrivateKey: sshInput.PrivateKeyPem, Passphrase: optionalString(sshInput.Passphrase)}, nil
	}
	if httpInput, err := value.AsHttpSecretInput(); err == nil && httpInput.Token != "" {
		return service.CredentialSecret{Kind: string(api.CredentialKindHttpToken), HTTPUsername: httpInput.Username, HTTPToken: httpInput.Token}, nil
	}
	return service.CredentialSecret{}, service.ErrValidation
}

// credentialResponse 将租户/平台凭据记录投影为租户可见的 API 形状。
func credentialResponse(record service.CredentialRecord) api.Credential {
	etagKind := etagKindCredential
	if record.IsGlobal {
		etagKind = etagKindGlobalCredential
	}
	return api.Credential{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKind, record.ID.String(), record.Revision), Name: record.Name,
		Kind: api.CredentialKind(record.Kind), Fingerprint: record.Encrypted.Fingerprint, SharedScope: api.CredentialSharedScope(record.SharedScope),
		TeamIds: uuidResponses(record.TeamIDs), IsGlobal: record.IsGlobal, CreatedBy: api.Uuid(record.CreatedBy),
		LastUsedAt: nullableTime(record.LastUsedAt), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// globalCredentialResponse 将平台凭据记录投影为平台管理员的 API 形状。
func globalCredentialResponse(record service.GlobalCredentialRecord) api.GlobalCredential {
	return api.GlobalCredential{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindGlobalCredential, record.ID.String(), record.Revision), Name: record.Name,
		Kind: api.CredentialKind(record.Kind), Fingerprint: record.Encrypted.Fingerprint, CreatedBy: api.Uuid(record.CreatedBy),
		LastUsedAt: nullableTime(record.LastUsedAt), Revision: int(record.Revision), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// knownHostResponse 将已批准主机记录投影为 API 形状。
func knownHostResponse(record service.KnownHostRecord) api.KnownHost {
	return api.KnownHost{Id: api.Uuid(record.ID), Host: record.Host, Port: int(record.Port), KeyType: api.KnownHostKeyType(record.KeyType), Fingerprint: record.Fingerprint, Source: api.KnownHostSource(record.Source), CreatedAt: record.CreatedAt}
}

// syncJobResponses 将凭据同步任务列表投影为 API 形状。
func syncJobResponses(jobs []service.CredentialSyncJob) []api.CredentialSyncJob {
	responses := make([]api.CredentialSyncJob, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, api.CredentialSyncJob{TenantSlug: api.Slug(job.TenantSlug), RepositoryId: api.Uuid(job.RepositoryID), JobId: api.Uuid(job.JobID), Deduplicated: job.Deduplicated})
	}
	return responses
}

// optionalSharedScope0 解包 SSH 创建联合体的共享范围（缺省为 private）。
func optionalSharedScope0(value *api.CredentialCreateRequest0SharedScope) string {
	if value == nil {
		return string(api.CredentialSharedScopePrivate)
	}
	return string(*value)
}

// optionalSharedScope1 解包 HTTP 创建联合体的共享范围（缺省为 private）。
func optionalSharedScope1(value *api.CredentialCreateRequest1SharedScope) string {
	if value == nil {
		return string(api.CredentialSharedScopePrivate)
	}
	return string(*value)
}

// apiUUIDs 将可空 API UUID 切片转换为服务层 UUID 切片（nil 透传）。
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

// optionalString 将可空字符串包装为指针（未指定或空视为 nil）。
func optionalString(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

// uuidResponses 将服务层 UUID 切片投影为 API UUID 切片。
func uuidResponses(values []uuid.UUID) []api.Uuid {
	responses := make([]api.Uuid, len(values))
	for index, value := range values {
		responses[index] = api.Uuid(value)
	}
	return responses
}

// credentialRotationRequestHash 计算凭据轮换请求的幂等摘要。
func credentialRotationRequestHash(operation, tenantSlug string, credentialID uuid.UUID, ifMatch string, secret service.CredentialSecret, resyncRepositories bool) ([]byte, error) {
	payload, err := json.Marshal(struct {
		Operation          string    `json:"operation"`
		TenantSlug         string    `json:"tenantSlug"`
		CredentialID       uuid.UUID `json:"credentialId"`
		IfMatch            string    `json:"ifMatch"`
		Kind               string    `json:"kind"`
		PrivateKey         string    `json:"privateKey,omitempty"`
		Passphrase         *string   `json:"passphrase,omitempty"`
		HTTPUsername       string    `json:"httpUsername,omitempty"`
		HTTPToken          string    `json:"httpToken,omitempty"`
		ResyncRepositories bool      `json:"resyncRepositories"`
	}{
		Operation: operation, TenantSlug: tenantSlug, CredentialID: credentialID, IfMatch: ifMatch,
		Kind: secret.Kind, PrivateKey: secret.PrivateKey, Passphrase: secret.Passphrase,
		HTTPUsername: secret.HTTPUsername, HTTPToken: secret.HTTPToken, ResyncRepositories: resyncRepositories,
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(payload)
	return digest[:], nil
}
