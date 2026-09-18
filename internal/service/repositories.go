package service

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/robfig/cron/v3"
)

const (
	maxRepositoryURLRunes   = 2048
	maxRepositoryRefRunes   = 255
	maxRepositoryPathRunes  = 512
	defaultRepositoryBranch = "main"
	defaultRepositoryDepth  = 50
	maxRepositoryNoteRunes  = 500
)

// Repositories 协调租户授权与仓库配置规则。
type Repositories struct {
	store      RepositoryStore
	identities IdentityStore
	now        func() time.Time
}

// NewRepositories 构造由调用方持有持久化的仓库用例。
func NewRepositories(store RepositoryStore, identities IdentityStore) *Repositories {
	return &Repositories{store: store, identities: identities, now: time.Now}
}

// List 返回租户内可见的活跃仓库。
func (repositories *Repositories) List(ctx context.Context, actor Principal, tenantSlug, query string, page, pageSize int) ([]RepositoryRecord, int64, error) {
	membership, err := repositories.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	items, total, err := repositories.store.ListRepositories(ctx, membership.TenantID, strings.TrimSpace(query), int32(pageSize), int32((page-1)*pageSize))
	if err != nil {
		return nil, 0, err
	}
	for index := range items {
		items[index].Capabilities = repositoryCapabilities(actor, membership.Role)
	}
	return items, total, nil
}

// Get 在应用租户边界后返回一个活跃仓库。
func (repositories *Repositories) Get(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID) (RepositoryRecord, error) {
	membership, err := repositories.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryRead)
	if err != nil {
		return RepositoryRecord{}, err
	}
	record, err := repositories.store.GetRepository(ctx, membership.TenantID, id)
	if err != nil {
		return RepositoryRecord{}, err
	}
	record.Capabilities = repositoryCapabilities(actor, membership.Role)
	return record, nil
}

// Create 校验、解析凭据、执行配额并插入一个仓库。
func (repositories *Repositories) Create(ctx context.Context, actor Principal, tenantSlug string, input NewRepositoryInput) (RepositoryRecord, error) {
	membership, err := repositories.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryWrite)
	if err != nil {
		return RepositoryRecord{}, err
	}
	validated, err := validateNewRepository(input)
	if err != nil {
		return RepositoryRecord{}, err
	}
	current, limit, err := repositories.countAndLimit(ctx, membership.TenantID)
	if err != nil {
		return RepositoryRecord{}, err
	}
	if current >= limit {
		return RepositoryRecord{}, &QuotaExceededError{Resource: quotaResourceRepositories, Current: current, Limit: limit}
	}
	refs, err := repositories.resolveCredential(ctx, membership, actor, validated.CredentialID)
	if err != nil {
		return RepositoryRecord{}, err
	}
	record, err := repositories.store.CreateRepository(ctx, NewRepository{
		TenantID: membership.TenantID, ID: uuid.NewV7(), URL: validated.URL, CanonicalURL: validated.CanonicalURL,
		CredentialID: credentialIDForStorage(refs), GlobalCredID: globalCredentialIDForStorage(refs), DefaultBranch: validated.DefaultBranch,
		BranchPolicy: validated.BranchPolicy, FetchConfig: validated.FetchConfig, SyncCron: validated.SyncCron, Note: validated.Note,
	})
	if err != nil {
		return RepositoryRecord{}, err
	}
	record.Capabilities = repositoryCapabilities(actor, membership.Role)
	return record, nil
}

// Update 在仓库 ETag 下应用显式元数据变更。
func (repositories *Repositories) Update(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, etag string, patch RepositoryPatchInput) (RepositoryRecord, error) {
	membership, err := repositories.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryWrite)
	if err != nil {
		return RepositoryRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "repository", id)
	if err != nil {
		return RepositoryRecord{}, ErrPrecondition
	}
	validated, err := validateRepositoryPatch(patch)
	if err != nil {
		return RepositoryRecord{}, err
	}
	var refs CredentialReference
	if validated.CredentialID != nil {
		if *validated.CredentialID != nil {
			refs, err = repositories.resolveCredential(ctx, membership, actor, *validated.CredentialID)
			if err != nil {
				return RepositoryRecord{}, err
			}
		}
	}
	credentialValue, globalValue := repositoryCredentialPointers(validated.CredentialID, refs)
	record, err := repositories.store.UpdateRepository(ctx, UpdateRepository{
		TenantID: membership.TenantID, ID: id, ExpectedRevision: expectedRevision,
		CredentialID: credentialValue, GlobalCredID: globalValue, DefaultBranch: validated.DefaultBranch,
		BranchPolicy: validated.BranchPolicy, FetchConfig: validated.FetchConfig, SyncCron: validated.SyncCron,
		Note: validated.Note, UpdatedAt: repositories.now().UTC(),
	})
	if err != nil {
		return RepositoryRecord{}, err
	}
	record.Capabilities = repositoryCapabilities(actor, membership.Role)
	return record, nil
}

// Delete 在当前 ETag 下软删除一个仓库。
func (repositories *Repositories) Delete(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, etag string) error {
	membership, err := repositories.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryWrite)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "repository", id)
	if err != nil {
		return ErrPrecondition
	}
	return repositories.store.DeleteRepository(ctx, membership.TenantID, id, expectedRevision, repositories.now().UTC())
}

// NewRepositoryInput 是 HTTP 处理器使用的校验后边界输入。
type NewRepositoryInput struct {
	URL           string
	CredentialID  *uuid.UUID
	DefaultBranch string
	BranchPolicy  *RepositoryBranchPolicy
	FetchConfig   *RepositoryFetchConfig
	SyncCron      *string
	Note          *string
}

// RepositoryPatchInput 保留 JSON 中「显式 null」与「省略字段」的区分。
type RepositoryPatchInput struct {
	CredentialID  **uuid.UUID
	DefaultBranch *string
	BranchPolicy  *RepositoryBranchPolicy
	FetchConfig   *RepositoryFetchConfig
	SyncCron      **string
	Note          **string
}

type validatedRepository struct {
	URL           string
	CanonicalURL  string
	CredentialID  *uuid.UUID
	DefaultBranch string
	BranchPolicy  RepositoryBranchPolicy
	FetchConfig   RepositoryFetchConfig
	SyncCron      *string
	Note          *string
}

func validateNewRepository(input NewRepositoryInput) (validatedRepository, error) {
	canonical, err := canonicalRepositoryURL(input.URL)
	if err != nil {
		return validatedRepository{}, ErrValidation
	}
	defaultBranch := input.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = defaultRepositoryBranch
	}
	if !validGitRefName(defaultBranch) {
		return validatedRepository{}, ErrValidation
	}
	branchPolicy := defaultBranchPolicy(defaultBranch)
	if input.BranchPolicy != nil {
		branchPolicy = *input.BranchPolicy
	}
	fetchConfig := defaultFetchConfig()
	if input.FetchConfig != nil {
		fetchConfig = *input.FetchConfig
	}
	if err := validateRepositoryConfig(branchPolicy, fetchConfig, input.SyncCron, input.Note); err != nil {
		return validatedRepository{}, err
	}
	return validatedRepository{URL: strings.TrimSpace(input.URL), CanonicalURL: canonical, CredentialID: input.CredentialID, DefaultBranch: defaultBranch, BranchPolicy: branchPolicy, FetchConfig: fetchConfig, SyncCron: input.SyncCron, Note: input.Note}, nil
}

func validateRepositoryPatch(input RepositoryPatchInput) (RepositoryPatchInput, error) {
	if input.CredentialID == nil && input.DefaultBranch == nil && input.BranchPolicy == nil && input.FetchConfig == nil && input.SyncCron == nil && input.Note == nil {
		return RepositoryPatchInput{}, ErrValidation
	}
	if input.DefaultBranch != nil && !validGitRefName(*input.DefaultBranch) {
		return RepositoryPatchInput{}, ErrValidation
	}
	if input.BranchPolicy != nil && !validBranchPolicy(*input.BranchPolicy) {
		return RepositoryPatchInput{}, ErrValidation
	}
	if input.FetchConfig != nil && !validFetchConfig(*input.FetchConfig) {
		return RepositoryPatchInput{}, ErrValidation
	}
	if input.Note != nil && *input.Note != nil && utf8.RuneCountInString(**input.Note) > maxRepositoryNoteRunes {
		return RepositoryPatchInput{}, ErrValidation
	}
	if input.SyncCron != nil && *input.SyncCron != nil && !validCron(**input.SyncCron) {
		return RepositoryPatchInput{}, ErrValidation
	}
	return input, nil
}

func canonicalRepositoryURL(remote string) (string, error) {
	if remote == "" || utf8.RuneCountInString(remote) > maxRepositoryURLRunes || remote != strings.TrimSpace(remote) {
		return "", ErrInvalidRepositoryURL
	}
	lowerRemote := strings.ToLower(remote)
	if strings.HasPrefix(lowerRemote, "https://") || strings.HasPrefix(lowerRemote, "ssh://") {
		if strings.ContainsAny(remote, "?#") {
			return "", ErrInvalidRepositoryURL
		}
		parsed, err := url.ParseRequestURI(remote)
		if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Path == "" {
			return "", ErrInvalidRepositoryURL
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		if (parsed.Scheme == "https" && parsed.Port() == strconv.Itoa(httpsDefaultPort)) || (parsed.Scheme == "ssh" && parsed.Port() == strconv.Itoa(knownHostDefaultPort)) {
			hostname := parsed.Hostname()
			if strings.Contains(hostname, ":") {
				parsed.Host = "[" + hostname + "]"
			} else {
				parsed.Host = hostname
			}
		}
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		if parsed.Path == "" {
			return "", ErrInvalidRepositoryURL
		}
		return parsed.String(), nil
	}
	if strings.ContainsAny(remote, "\x00\r\n\t ") || !strings.Contains(remote, "@") {
		return "", ErrInvalidSCPURL
	}
	parts := strings.SplitN(remote, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrInvalidSCPURL
	}
	user, host, found := strings.Cut(parts[0], "@")
	if !found || user == "" || host == "" || strings.ContainsAny(user, "/\\:@") || strings.ContainsAny(host, "/\\:@") {
		return "", ErrInvalidSCPURL
	}
	path := strings.TrimRight(parts[1], "/")
	if path == "" {
		return "", ErrInvalidSCPURL
	}
	return user + "@" + strings.ToLower(host) + ":" + path, nil
}

func defaultBranchPolicy(defaultBranch string) RepositoryBranchPolicy {
	return RepositoryBranchPolicy{BranchPatterns: []string{defaultBranch}, TagPatterns: []string{}}
}

func defaultFetchConfig() RepositoryFetchConfig {
	depth := defaultRepositoryDepth
	return RepositoryFetchConfig{Shallow: true, Depth: new(depth), Submodules: false, PathAllow: []string{}, PathIgnore: []string{}, KnownHostPolicy: knownHostPolicyStrict}
}

func validateRepositoryConfig(policy RepositoryBranchPolicy, fetch RepositoryFetchConfig, cron, note *string) error {
	if !validBranchPolicy(policy) || !validFetchConfig(fetch) {
		return ErrValidation
	}
	if cron != nil && strings.TrimSpace(*cron) == "" {
		return ErrValidation
	}
	if cron != nil && (utf8.RuneCountInString(*cron) > maxFilterTextRunes || !validCron(*cron)) {
		return ErrValidation
	}
	if note != nil && utf8.RuneCountInString(*note) > maxRepositoryNoteRunes {
		return ErrValidation
	}
	return nil
}

func validFetchConfig(fetch RepositoryFetchConfig) bool {
	if fetch.KnownHostPolicy != knownHostPolicyStrict && fetch.KnownHostPolicy != knownHostPolicyAcceptNew {
		return false
	}
	if fetch.Depth != nil && (*fetch.Depth < 1 || *fetch.Depth > maxRepositoryDepth) {
		return false
	}
	if fetch.Proxy != nil && (utf8.RuneCountInString(*fetch.Proxy) > maxRepositoryURLRunes || strings.TrimSpace(*fetch.Proxy) == "") {
		return false
	}
	return validUniqueRepositoryPaths(fetch.PathAllow, false) && validUniqueRepositoryPaths(fetch.PathIgnore, true)
}

func validCron(value string) bool {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(value)
	return err == nil
}

func validGitRefName(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > maxRepositoryRefRunes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || strings.ContainsRune("._/-", character)) {
			return false
		}
	}
	return !strings.Contains(value, "..") && !strings.Contains(value, "//") && !strings.Contains(value, "@{") && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, ".lock")
}

func validBranchPolicy(policy RepositoryBranchPolicy) bool {
	if len(policy.BranchPatterns) == 0 {
		return false
	}
	for _, pattern := range policy.BranchPatterns {
		if !validGitRefGlob(pattern) {
			return false
		}
	}
	for _, pattern := range policy.TagPatterns {
		if !validGitRefGlob(pattern) {
			return false
		}
	}
	return true
}

func validGitRefGlob(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > maxRepositoryRefRunes || strings.HasPrefix(value, "!") || strings.Contains(value, "@{") || strings.Contains(value, "//") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" {
			return false
		}
		name := strings.NewReplacer("*", "", "?", "").Replace(component)
		if name != "" && !validGitRefName(name) {
			return false
		}
	}
	return true
}

func validUniqueRepositoryPaths(values []string, allowNegation bool) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists || !validRepositoryPathPattern(value, allowNegation) {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validRepositoryPathPattern(value string, allowNegation bool) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRepositoryPathRunes || strings.ContainsAny(value, "\x00\r\n\\") {
		return false
	}
	if strings.HasPrefix(value, "!") {
		if !allowNegation {
			return false
		}
		value = strings.TrimPrefix(value, "!")
	}
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.Contains(value, "//") || strings.Contains(value, "..") {
		return false
	}
	depth := 0
	for _, character := range value {
		switch character {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func (repositories *Repositories) countAndLimit(ctx context.Context, tenantID uuid.UUID) (int64, int64, error) {
	return repositories.store.CountRepositories(ctx, tenantID)
}

func (repositories *Repositories) resolveCredential(ctx context.Context, membership Membership, actor Principal, id *uuid.UUID) (CredentialReference, error) {
	if id == nil {
		return CredentialReference{}, nil
	}
	refs, found, err := repositories.store.ResolveCredentialReference(ctx, membership.TenantID, actor.User.ID, *id)
	if err != nil {
		return CredentialReference{}, err
	}
	if !found {
		return CredentialReference{}, ErrNotFound
	}
	return refs, nil
}

func credentialIDForStorage(reference CredentialReference) *uuid.UUID {
	if reference.IsGlobal || reference.ID == uuid.Nil() {
		return nil
	}
	return new(reference.ID)
}

func globalCredentialIDForStorage(reference CredentialReference) *uuid.UUID {
	if !reference.IsGlobal || reference.ID == uuid.Nil() {
		return nil
	}
	return new(reference.ID)
}

func repositoryCredentialPointers(input **uuid.UUID, reference CredentialReference) (**uuid.UUID, **uuid.UUID) {
	if input == nil {
		return nil, nil
	}
	var localID, globalID *uuid.UUID
	if *input != nil {
		if reference.IsGlobal {
			globalID = new(reference.ID)
		} else {
			localID = new(reference.ID)
		}
	}
	return new(localID), new(globalID)
}

func (repositories *Repositories) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || repositories.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := repositories.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func repositoryCapabilities(actor Principal, role string) []string {
	capabilities := []string{scopeRepositoryRead}
	if roleAllows(role, scopeRepositoryWrite) {
		capabilities = append(capabilities, scopeRepositoryWrite)
	}
	if actor.Kind == PrincipalPAT && !slices.Contains(actor.Scopes, scopeRepositoryWrite) && !slices.Contains(actor.Scopes, scopeWildcard) {
		return []string{scopeRepositoryRead}
	}
	return capabilities
}
