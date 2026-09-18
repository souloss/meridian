package service

import (
	"context"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
	"uuid"
)

const (
	maxProducerProfileNameRunes   = 64
	maxProducerProfileExecRunes   = 512
	maxProducerProfileArgs        = 64
	producerTimeoutCommandDefault = 300
	producerTimeoutAIDefault      = 600
	// producerMemoryMiBDefault 是生产者内存上限的默认值（MiB），来源 domain.yaml limits.memoryMiB。
	producerMemoryMiBDefault = 1024
	// producerMemoryMiBMin 是生产者内存上限的最小值（MiB）。
	producerMemoryMiBMin = 64
	// producerMemoryMiBMax 是生产者内存上限的最大值（MiB）。
	producerMemoryMiBMax = 16384
	// producerCPUSecondsDefault 是生产者 CPU 时间配额的默认值（秒），来源 domain.yaml limits.cpuSeconds。
	producerCPUSecondsDefault = 600
	// producerCPUSecondsMin 是生产者 CPU 时间配额的最小值（秒）。
	producerCPUSecondsMin = 1
	// producerCPUSecondsMax 是生产者 CPU 时间配额的最大值（秒）。
	producerCPUSecondsMax = 3600
	// producerPidsDefault 是生产者进程数上限的默认值，来源 domain.yaml limits.pids。
	producerPidsDefault = 128
	// producerPidsMin 是生产者进程数上限的最小值。
	producerPidsMin = 1
	// producerPidsMax 是生产者进程数上限的最大值。
	producerPidsMax = 1024
)

// Producers 协调平台生产者配置与租户选择规则。
type Producers struct {
	store      ProducerStore
	identities IdentityStore
}

// NewProducers 构造由调用方持有持久化的生产者配置用例。
func NewProducers(store ProducerStore, identities IdentityStore) *Producers {
	return &Producers{store: store, identities: identities}
}

// ListAvailable 返回租户成员可见的、启用且依赖可用的生产者配置，可按 supportedKinds
// 包含所请求类别进行过滤。
func (producers *Producers) ListAvailable(ctx context.Context, actor Principal, tenantSlug, kind string) ([]ProducerProfileOption, error) {
	if _, err := producers.tenantMembership(ctx, actor, tenantSlug, scopeServiceWrite); err != nil {
		return nil, err
	}
	if kind != "" && !validKindID(kind) {
		return nil, ErrValidation
	}
	profiles, err := producers.store.ListAvailableProducerProfiles(ctx, kind)
	if err != nil {
		return nil, err
	}
	options := make([]ProducerProfileOption, 0, len(profiles))
	for _, profile := range profiles {
		options = append(options, ProducerProfileOption{
			ID: profile.ID, Name: profile.Name, Kind: profile.Kind,
			SupportedKinds: append([]string(nil), profile.SupportedKinds...),
			Network:        profile.Network, DependencyStatus: profile.DependencyStatus,
			UnavailableReason: profile.UnavailableReason,
		})
	}
	return options, nil
}

// Create 校验、探测可执行文件并插入一个平台生产者配置。
func (producers *Producers) Create(ctx context.Context, actor Principal, input NewProducerProfile) (ProducerProfile, error) {
	if !isPlatformAdministrator(actor) {
		return ProducerProfile{}, ErrNotFound
	}
	validated, err := validateNewProducerProfile(input)
	if err != nil {
		return ProducerProfile{}, err
	}
	validated.ID = uuid.NewV7()
	validated.DependencyStatus, validated.UnavailableReason = probeExecutable(validated.Executable)
	return producers.store.CreateProducerProfile(ctx, validated)
}

func (producers *Producers) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || producers.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := producers.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func validateNewProducerProfile(input NewProducerProfile) (NewProducerProfile, error) {
	if utf8.RuneCountInString(input.Name) > maxProducerProfileNameRunes || !slugPattern.MatchString(input.Name) {
		return NewProducerProfile{}, ErrValidation
	}
	if input.Kind != producerKindCommand && input.Kind != producerKindAI {
		return NewProducerProfile{}, ErrValidation
	}
	if !strings.HasPrefix(input.Executable, "/") || utf8.RuneCountInString(input.Executable) > maxProducerProfileExecRunes {
		return NewProducerProfile{}, ErrValidation
	}
	if len(input.Args) > maxProducerProfileArgs || len(input.SupportedKinds) == 0 {
		return NewProducerProfile{}, ErrValidation
	}
	for _, kind := range input.SupportedKinds {
		if !validKindID(kind) {
			return NewProducerProfile{}, ErrValidation
		}
	}
	if input.Network != producerNetworkNone && input.Network != producerNetworkInherit {
		return NewProducerProfile{}, ErrValidation
	}
	if input.TimeoutSec == 0 {
		if input.Kind == producerKindAI {
			input.TimeoutSec = producerTimeoutAIDefault
		} else {
			input.TimeoutSec = producerTimeoutCommandDefault
		}
	}
	if input.TimeoutSec < sourceTimeoutMinSec || input.TimeoutSec > sourceTimeoutMaxSec {
		return NewProducerProfile{}, ErrValidation
	}
	if input.MemoryMiB == 0 {
		input.MemoryMiB = producerMemoryMiBDefault
	}
	if input.MemoryMiB < producerMemoryMiBMin || input.MemoryMiB > producerMemoryMiBMax {
		return NewProducerProfile{}, ErrValidation
	}
	if input.CPUSeconds == 0 {
		input.CPUSeconds = producerCPUSecondsDefault
	}
	if input.CPUSeconds < producerCPUSecondsMin || input.CPUSeconds > producerCPUSecondsMax {
		return NewProducerProfile{}, ErrValidation
	}
	if input.Pids == 0 {
		input.Pids = producerPidsDefault
	}
	if input.Pids < producerPidsMin || input.Pids > producerPidsMax {
		return NewProducerProfile{}, ErrValidation
	}
	input.Args = slices.Clone(input.Args)
	input.EnvAllowlist = slices.Clone(input.EnvAllowlist)
	input.SupportedKinds = slices.Clone(input.SupportedKinds)
	return input, nil
}

func probeExecutable(executable string) (string, *string) {
	info, err := os.Stat(executable)
	if err != nil || info.IsDir() {
		reason := "executable path does not exist"
		return dependencyStatusUnavailable, &reason
	}
	if info.Mode().Perm()&0o111 == 0 {
		reason := "executable path is not executable"
		return dependencyStatusUnavailable, &reason
	}
	return dependencyStatusAvailable, nil
}

// validKindID 接受契约中注册的有限资产类别标识。
func validKindID(kind string) bool {
	if kind == "" || utf8.RuneCountInString(kind) > 64 || kind != strings.TrimSpace(kind) {
		return false
	}
	return true
}
