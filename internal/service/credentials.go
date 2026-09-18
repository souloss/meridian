package service

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	credentialNameLimit  = 64
	knownHostDefaultPort = 22
	// knownHostMaxBytes 是规范化主机名的最大字节长度。
	knownHostMaxBytes = 255
	// dnsLabelMaxBytes 是 DNS 主机名单个标签的最大字节长度。
	dnsLabelMaxBytes = 63
)

// Credentials 协调凭据的授权、加密与条件持久化。
type Credentials struct {
	store      CredentialStore
	identities IdentityStore
	keyring    CredentialKeyring
	probe      ConnectionProbe
	now        func() time.Time
}

// NewCredentials 构造由调用方持有持久化与密钥材料的凭据用例。
func NewCredentials(store CredentialStore, identities IdentityStore, keyring CredentialKeyring) *Credentials {
	return NewCredentialsWithProbe(store, identities, keyring, NewGitConnectionProbe())
}

// NewCredentialsWithProbe 构造带显式连接探测的凭据用例。测试与受控部署可注入确定性探测，
// 而不改变授权行为。
func NewCredentialsWithProbe(store CredentialStore, identities IdentityStore, keyring CredentialKeyring, probe ConnectionProbe) *Credentials {
	if probe == nil {
		probe = NewGitConnectionProbe()
	}
	return &Credentials{store: store, identities: identities, keyring: keyring, probe: probe, now: time.Now}
}

// ListTenant 返回认证租户成员可见的租户持有凭据。
func (credentials *Credentials) ListTenant(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]CredentialRecord, int64, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return credentials.store.ListCredentials(ctx, membership.TenantID, actor.User.ID, int32(pageSize), int32((page-1)*pageSize))
}

// CreateTenant 加密并持久化一个租户持有凭据，且不暴露其秘密。
func (credentials *Credentials) CreateTenant(ctx context.Context, actor Principal, tenantSlug string, input CredentialInput) (CredentialRecord, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialManage)
	if err != nil {
		return CredentialRecord{}, err
	}
	if err := validateCredentialInput(input); err != nil {
		return CredentialRecord{}, err
	}
	id := uuid.NewV7()
	encrypted, err := credentials.keyring.Encrypt(membership.TenantID.String(), id, input.Secret)
	if err != nil {
		return CredentialRecord{}, err
	}
	return credentials.store.CreateCredential(ctx, NewCredential{
		TenantID: membership.TenantID, ID: id, Name: input.Name, Kind: input.Secret.Kind,
		Encrypted: encrypted, SharedScope: input.SharedScope, TeamIDs: slices.Clone(input.TeamIDs), CreatedBy: actor.User.ID,
	})
}

// GetTenant 在应用与列表相同的可见性边界下返回一个租户凭据。
func (credentials *Credentials) GetTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID) (CredentialRecord, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialRead)
	if err != nil {
		return CredentialRecord{}, err
	}
	return credentials.store.GetCredential(ctx, membership.TenantID, id, actor.User.ID)
}

// UpdateTenant 对租户凭据应用仅元数据的条件更新。
func (credentials *Credentials) UpdateTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, etag string, patch CredentialPatch) (CredentialRecord, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialManage)
	if err != nil {
		return CredentialRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "credential", id)
	if err != nil {
		return CredentialRecord{}, ErrPrecondition
	}
	if err := validateCredentialPatch(patch); err != nil {
		return CredentialRecord{}, err
	}
	return credentials.store.UpdateCredential(ctx, UpdateCredential{
		TenantID: membership.TenantID, ID: id, ExpectedRevision: expectedRevision,
		Name: patch.Name, SharedScope: patch.SharedScope, TeamIDs: cloneOptionalUUIDs(patch.TeamIDs), UpdatedAt: credentials.now().UTC(),
	})
}

// DeleteTenant 条件删除一个租户凭据，可选地解除仓库引用绑定。
func (credentials *Credentials) DeleteTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, etag string, force bool) error {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialManage)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "credential", id)
	if err != nil {
		return ErrPrecondition
	}
	return credentials.store.DeleteCredential(ctx, membership.TenantID, id, expectedRevision, force, credentials.now().UTC())
}

// RotateTenant 在 If-Match 下替换租户秘密，并返回任何入队的同步元数据。
func (credentials *Credentials) RotateTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, etag string, rotation CredentialRotation) (CredentialRecord, []CredentialSyncJob, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialManage)
	if err != nil {
		return CredentialRecord{}, nil, err
	}
	rotation.PrincipalType, rotation.PrincipalID = rotationPrincipal(actor)
	if err := validateCredentialRotationReplay(rotation); err != nil {
		return CredentialRecord{}, nil, err
	}
	if rotation.IdempotencyKey != uuid.Nil() {
		replayed, jobs, found, err := credentials.store.LookupCredentialRotation(ctx, membership.TenantID, rotation.PrincipalType, rotation.PrincipalID, rotation.IdempotencyKey, rotation.RequestHash)
		if err != nil {
			return CredentialRecord{}, nil, err
		}
		if found {
			return replayed, jobs, nil
		}
	}
	expectedRevision, err := parseRevisionETag(etag, "credential", id)
	if err != nil {
		return CredentialRecord{}, nil, ErrPrecondition
	}
	if err := validateCredentialSecret(rotation.Secret); err != nil {
		return CredentialRecord{}, nil, ErrValidation
	}
	current, err := credentials.store.GetCredential(ctx, membership.TenantID, id, actor.User.ID)
	if err != nil {
		return CredentialRecord{}, nil, err
	}
	if current.Kind != rotation.Secret.Kind {
		return CredentialRecord{}, nil, ErrValidation
	}
	fingerprint, err := credentials.keyring.Fingerprint(rotation.Secret)
	if err != nil {
		return CredentialRecord{}, nil, err
	}
	if fingerprint == current.Encrypted.Fingerprint {
		return CredentialRecord{}, nil, ErrValidation
	}
	encrypted, err := credentials.keyring.Encrypt(membership.TenantID.String(), id, rotation.Secret)
	if err != nil {
		return CredentialRecord{}, nil, err
	}
	updated, jobs, err := credentials.store.RotateCredential(ctx, RotateCredential{
		TenantID: membership.TenantID, ID: id, ExpectedRevision: expectedRevision,
		Encrypted: encrypted, ResyncRepositories: rotation.ResyncRepositories, UpdatedAt: credentials.now().UTC(),
		IdempotencyKey: rotation.IdempotencyKey, RequestHash: rotation.RequestHash,
		PrincipalType: rotation.PrincipalType, PrincipalID: rotation.PrincipalID,
	})
	if err != nil {
		return CredentialRecord{}, nil, err
	}
	return updated, jobs, nil
}

// ListGlobal 向平台管理员返回全部平台持有凭据元数据。
func (credentials *Credentials) ListGlobal(ctx context.Context, actor Principal, page, pageSize int) ([]GlobalCredentialRecord, int64, error) {
	if !isPlatformAdministrator(actor) {
		return nil, 0, ErrNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return credentials.store.ListGlobalCredentials(ctx, int32(pageSize), int32((page-1)*pageSize))
}

// CreateGlobal 加密并持久化一个平台持有凭据。
func (credentials *Credentials) CreateGlobal(ctx context.Context, actor Principal, input CredentialInput) (GlobalCredentialRecord, error) {
	if !isPlatformAdministrator(actor) {
		return GlobalCredentialRecord{}, ErrNotFound
	}
	if err := validateCredentialInput(input); err != nil {
		return GlobalCredentialRecord{}, err
	}
	id := uuid.NewV7()
	encrypted, err := credentials.keyring.Encrypt(credentialScopeGlobal, id, input.Secret)
	if err != nil {
		return GlobalCredentialRecord{}, err
	}
	return credentials.store.CreateGlobalCredential(ctx, NewGlobalCredential{
		ID: id, Name: input.Name, Kind: input.Secret.Kind, Encrypted: encrypted, CreatedBy: actor.User.ID,
	})
}

// UpdateGlobal 对平台凭据应用仅元数据的条件更新。
func (credentials *Credentials) UpdateGlobal(ctx context.Context, actor Principal, id uuid.UUID, etag, name string) (GlobalCredentialRecord, error) {
	if !isPlatformAdministrator(actor) {
		return GlobalCredentialRecord{}, ErrNotFound
	}
	expectedRevision, err := parseRevisionETag(etag, "global-credential", id)
	if err != nil {
		return GlobalCredentialRecord{}, ErrPrecondition
	}
	if err := validateCredentialName(name); err != nil {
		return GlobalCredentialRecord{}, err
	}
	return credentials.store.UpdateGlobalCredential(ctx, UpdateGlobalCredential{ID: id, ExpectedRevision: expectedRevision, Name: name, UpdatedAt: credentials.now().UTC()})
}

// DeleteGlobal 条件删除一个平台凭据，并可原子地解除引用绑定。
func (credentials *Credentials) DeleteGlobal(ctx context.Context, actor Principal, id uuid.UUID, etag string, force bool) error {
	if !isPlatformAdministrator(actor) {
		return ErrNotFound
	}
	expectedRevision, err := parseRevisionETag(etag, "global-credential", id)
	if err != nil {
		return ErrPrecondition
	}
	return credentials.store.DeleteGlobalCredential(ctx, id, expectedRevision, force, credentials.now().UTC())
}

// RotateGlobal 在 If-Match 下替换平台秘密，并仅返回安全的同步元数据。
func (credentials *Credentials) RotateGlobal(ctx context.Context, actor Principal, id uuid.UUID, etag string, rotation CredentialRotation) (GlobalCredentialRecord, []CredentialSyncJob, error) {
	if !isPlatformAdministrator(actor) {
		return GlobalCredentialRecord{}, nil, ErrNotFound
	}
	rotation.PrincipalType, rotation.PrincipalID = rotationPrincipal(actor)
	if err := validateCredentialRotationReplay(rotation); err != nil {
		return GlobalCredentialRecord{}, nil, err
	}
	if rotation.IdempotencyKey != uuid.Nil() {
		replayed, jobs, found, err := credentials.store.LookupGlobalCredentialRotation(ctx, rotation.PrincipalType, rotation.PrincipalID, rotation.IdempotencyKey, rotation.RequestHash)
		if err != nil {
			return GlobalCredentialRecord{}, nil, err
		}
		if found {
			return replayed, jobs, nil
		}
	}
	expectedRevision, err := parseRevisionETag(etag, "global-credential", id)
	if err != nil {
		return GlobalCredentialRecord{}, nil, ErrPrecondition
	}
	if err := validateCredentialSecret(rotation.Secret); err != nil {
		return GlobalCredentialRecord{}, nil, ErrValidation
	}
	current, err := credentials.store.GetGlobalCredential(ctx, id)
	if err != nil {
		return GlobalCredentialRecord{}, nil, err
	}
	if current.Kind != rotation.Secret.Kind {
		return GlobalCredentialRecord{}, nil, ErrValidation
	}
	fingerprint, err := credentials.keyring.Fingerprint(rotation.Secret)
	if err != nil {
		return GlobalCredentialRecord{}, nil, err
	}
	if fingerprint == current.Encrypted.Fingerprint {
		return GlobalCredentialRecord{}, nil, ErrValidation
	}
	encrypted, err := credentials.keyring.Encrypt(credentialScopeGlobal, id, rotation.Secret)
	if err != nil {
		return GlobalCredentialRecord{}, nil, err
	}
	return credentials.store.RotateGlobalCredential(ctx, RotateGlobalCredential{
		ID: id, ExpectedRevision: expectedRevision, Encrypted: encrypted,
		ResyncRepositories: rotation.ResyncRepositories, UpdatedAt: credentials.now().UTC(),
		IdempotencyKey: rotation.IdempotencyKey, RequestHash: rotation.RequestHash,
		PrincipalType: rotation.PrincipalType, PrincipalID: rotation.PrincipalID,
	})
}

func rotationPrincipal(actor Principal) (string, uuid.UUID) {
	if actor.Kind == PrincipalPAT {
		return string(actor.Kind), actor.TokenID
	}
	return string(actor.Kind), actor.User.ID
}

func validateCredentialRotationReplay(rotation CredentialRotation) error {
	if rotation.IdempotencyKey == uuid.Nil() {
		return nil
	}
	if len(rotation.RequestHash) != 32 || rotation.PrincipalType == "" || rotation.PrincipalID == uuid.Nil() {
		return ErrValidation
	}
	return nil
}

// ListKnownHosts 返回某租户已批准的 SSH 主机密钥身份。
func (credentials *Credentials) ListKnownHosts(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]KnownHostRecord, int64, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return credentials.store.ListKnownHosts(ctx, membership.TenantID, int32(pageSize), int32((page-1)*pageSize))
}

// CreateKnownHost 校验并持久化一个手动批准的 SSH 主机密钥。
func (credentials *Credentials) CreateKnownHost(ctx context.Context, actor Principal, tenantSlug, host string, port *int, publicKey string) (KnownHostRecord, error) {
	membership, err := credentials.tenantMembership(ctx, actor, tenantSlug, scopeCredentialManage)
	if err != nil {
		return KnownHostRecord{}, err
	}
	normalizedHost, err := normalizeKnownHost(host)
	if err != nil {
		return KnownHostRecord{}, ErrValidation
	}
	knownPort := knownHostDefaultPort
	if port != nil {
		knownPort = *port
	}
	if knownPort < knownHostPortMin || knownPort > knownHostPortMax {
		return KnownHostRecord{}, ErrValidation
	}
	identity, err := ParseKnownHostPublicKey(publicKey)
	if err != nil {
		return KnownHostRecord{}, ErrValidation
	}
	return credentials.store.CreateKnownHost(ctx, NewKnownHost{
		TenantID: membership.TenantID, ID: uuid.NewV7(), Host: normalizedHost, Port: int32(knownPort),
		KeyType: identity.KeyType, PublicKey: identity.PublicKey, Fingerprint: identity.Fingerprint,
		Source: knownHostSourceManual, CreatedBy: actor.User.ID,
	})
}

func (credentials *Credentials) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) {
			return Membership{}, ErrNotFound
		}
		if !slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || credentials.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := credentials.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, permission) {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func validateCredentialInput(input CredentialInput) error {
	if err := validateCredentialName(input.Name); err != nil {
		return err
	}
	if err := validateCredentialSecret(input.Secret); err != nil {
		return ErrValidation
	}
	if err := validateSharing(input.SharedScope, input.TeamIDs); err != nil {
		return err
	}
	return nil
}

func validateCredentialPatch(patch CredentialPatch) error {
	if patch.Name == nil && patch.SharedScope == nil && patch.TeamIDs == nil {
		return ErrValidation
	}
	if patch.Name != nil {
		if err := validateCredentialName(*patch.Name); err != nil {
			return err
		}
	}
	if patch.SharedScope != nil {
		if err := validateSharing(*patch.SharedScope, valueOrEmptyUUIDs(patch.TeamIDs)); err != nil {
			return err
		}
	} else if patch.TeamIDs != nil {
		if err := validateTeamIDs(*patch.TeamIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateCredentialName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > credentialNameLimit {
		return ErrValidation
	}
	return nil
}

func validateSharing(scope string, teamIDs []uuid.UUID) error {
	switch scope {
	case credentialSharedScopePrivate, credentialSharedScopeTenant:
		if len(teamIDs) != 0 {
			return ErrValidation
		}
	case credentialSharedScopeTeam:
		if err := validateTeamIDs(teamIDs); err != nil {
			return err
		}
	default:
		return ErrValidation
	}
	return nil
}

func validateTeamIDs(teamIDs []uuid.UUID) error {
	if len(teamIDs) == 0 {
		return ErrValidation
	}
	copyIDs := slices.Clone(teamIDs)
	slices.SortFunc(copyIDs, func(left, right uuid.UUID) int { return strings.Compare(left.String(), right.String()) })
	if len(slices.Compact(copyIDs)) != len(teamIDs) {
		return ErrValidation
	}
	for _, id := range teamIDs {
		if id == uuid.Nil() {
			return ErrValidation
		}
	}
	return nil
}

func validatePagination(page, pageSize int) error {
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return ErrValidation
	}
	return nil
}

func parseRevisionETag(value, kind string, id uuid.UUID) (int64, error) {
	prefix := fmt.Sprintf(`"%s:%s:`, kind, id)
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, `"`) {
		return 0, ErrParseEntityTag
	}
	revisionText := strings.TrimSuffix(strings.TrimPrefix(value, prefix), `"`)
	var revision int64
	if _, err := fmt.Sscan(revisionText, &revision); err != nil || revision < 1 {
		return 0, ErrParseEntityTagRevision
	}
	return revision, nil
}

func cloneOptionalUUIDs(value *[]uuid.UUID) *[]uuid.UUID {
	if value == nil {
		return nil
	}
	copyValue := slices.Clone(*value)
	return &copyValue
}

func valueOrEmptyUUIDs(value *[]uuid.UUID) []uuid.UUID {
	if value == nil {
		return nil
	}
	return *value
}

func normalizeKnownHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	host = strings.ToLower(host)
	if host == "" || len(host) > knownHostMaxBytes || strings.ContainsAny(host, "/?#\x00 \t\r\n") {
		return "", ErrInvalidHost
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return host, nil
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > dnsLabelMaxBytes || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", ErrInvalidDNSHost
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return "", ErrInvalidDNSHost
			}
		}
	}
	return host, nil
}
