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
)

// Producers coordinates platform producer configuration and tenant selection rules.
type Producers struct {
	store      ProducerStore
	identities IdentityStore
}

// NewProducers constructs producer profile use cases with caller-owned persistence.
func NewProducers(store ProducerStore, identities IdentityStore) *Producers {
	return &Producers{store: store, identities: identities}
}

// ListAvailable returns enabled, dependency-available producer profiles visible
// to a tenant member, optionally filtered to profiles whose supportedKinds
// contain the requested kind.
func (producers *Producers) ListAvailable(ctx context.Context, actor Principal, tenantSlug, kind string) ([]ProducerProfileOption, error) {
	if _, err := producers.tenantMembership(ctx, actor, tenantSlug, "service:write"); err != nil {
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

// Create validates, probes the executable, and inserts one platform producer profile.
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
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, "*")) {
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
	if input.Network != "none" && input.Network != "inherit" {
		return NewProducerProfile{}, ErrValidation
	}
	if input.TimeoutSec == 0 {
		if input.Kind == producerKindAI {
			input.TimeoutSec = producerTimeoutAIDefault
		} else {
			input.TimeoutSec = producerTimeoutCommandDefault
		}
	}
	if input.TimeoutSec < 10 || input.TimeoutSec > 3600 {
		return NewProducerProfile{}, ErrValidation
	}
	if input.MemoryMiB == 0 {
		input.MemoryMiB = 1024
	}
	if input.MemoryMiB < 64 || input.MemoryMiB > 16384 {
		return NewProducerProfile{}, ErrValidation
	}
	if input.CPUSeconds == 0 {
		input.CPUSeconds = 600
	}
	if input.CPUSeconds < 1 || input.CPUSeconds > 3600 {
		return NewProducerProfile{}, ErrValidation
	}
	if input.Pids == 0 {
		input.Pids = 128
	}
	if input.Pids < 1 || input.Pids > 1024 {
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
		return "unavailable", &reason
	}
	if info.Mode().Perm()&0o111 == 0 {
		reason := "executable path is not executable"
		return "unavailable", &reason
	}
	return "available", nil
}

// validKindID accepts the finite asset kind identifiers registered in the contract.
func validKindID(kind string) bool {
	if kind == "" || utf8.RuneCountInString(kind) > 64 || kind != strings.TrimSpace(kind) {
		return false
	}
	return true
}
