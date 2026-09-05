package service

import (
	"errors"
	"testing"
	"uuid"
)

func TestCanonicalRepositoryURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "https defaults and host", input: "HTTPS://Git.Example.COM:443/Team/Repo.git/", want: "https://git.example.com/Team/Repo.git"},
		{name: "ssh defaults and path case", input: "ssh://Git.Example.COM:22/Team/Repo.git/", want: "ssh://git.example.com/Team/Repo.git"},
		{name: "ipv6 default port", input: "https://[2001:DB8::1]:443/Team/Repo.git/", want: "https://[2001:db8::1]/Team/Repo.git"},
		{name: "scp host only", input: "git@Git.Example.COM:Team/Repo.git/", want: "git@git.example.com:Team/Repo.git"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := canonicalRepositoryURL(test.input)
			if err != nil || got != test.want {
				t.Fatalf("canonicalRepositoryURL(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
	for _, input := range []string{
		"https://user:password@git.example.com/repo.git",
		"https://git.example.com/repo.git?token=secret",
		"https://git.example.com/repo.git#fragment",
		"ssh://git.example.com",
		"git.example.com:repo.git",
	} {
		if _, err := canonicalRepositoryURL(input); err == nil {
			t.Errorf("canonicalRepositoryURL(%q) unexpectedly succeeded", input)
		}
	}
}

func TestRepositoryValidationDefaultsAndRefRules(t *testing.T) {
	t.Parallel()

	validated, err := validateNewRepository(NewRepositoryInput{URL: "https://git.example.com/repo.git"})
	if err != nil {
		t.Fatalf("validateNewRepository() error = %v", err)
	}
	if validated.DefaultBranch != "main" || len(validated.BranchPolicy.BranchPatterns) != 1 || validated.FetchConfig.Depth == nil || *validated.FetchConfig.Depth != 50 {
		t.Fatalf("defaults = %#v", validated)
	}
	for _, ref := range []string{"", "release..bad", "release//bad", "release@{bad}", "release.lock", "release bad"} {
		if validGitRefName(ref) {
			t.Errorf("validGitRefName(%q) = true", ref)
		}
	}
	if !validBranchPolicy(RepositoryBranchPolicy{BranchPatterns: []string{"main", "release/*"}, TagPatterns: []string{}}) {
		t.Fatal("valid branch policy rejected a valid empty tag policy")
	}
	if validBranchPolicy(RepositoryBranchPolicy{BranchPatterns: []string{"!main"}}) {
		t.Fatal("negated branch glob unexpectedly accepted")
	}
	if validGitRefName("release+candidate") {
		t.Fatal("non-contract Git ref character unexpectedly accepted")
	}
	if validFetchConfig(RepositoryFetchConfig{KnownHostPolicy: "strict", PathAllow: []string{"service/**", "service/**"}}) {
		t.Fatal("duplicate path allow pattern unexpectedly accepted")
	}
	if validFetchConfig(RepositoryFetchConfig{KnownHostPolicy: "strict", PathAllow: []string{"../secrets"}}) {
		t.Fatal("path traversal pattern unexpectedly accepted")
	}
	if !validFetchConfig(RepositoryFetchConfig{KnownHostPolicy: "strict", PathIgnore: []string{"!generated/**"}}) {
		t.Fatal("gitignore negation pattern rejected")
	}
	if _, err := validateNewRepository(NewRepositoryInput{URL: "https://git.example.com:443/Team/Repo.git/", DefaultBranch: "main"}); err != nil {
		t.Fatalf("integration repository input rejected: %v", err)
	}
}

func TestRepositoryCredentialPointersPreserveExplicitNull(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("018f0b7a-3b54-7c2e-b7ef-3baf9709a2a1")
	var explicitNull *uuid.UUID
	local, global := repositoryCredentialPointers(&explicitNull, CredentialReference{})
	if local == nil || global == nil || *local != nil || *global != nil {
		t.Fatalf("explicit null pointers = %#v, %#v", local, global)
	}
	value := &id
	local, global = repositoryCredentialPointers(&value, CredentialReference{ID: id})
	if local == nil || global == nil || *local == nil || **local != id || *global != nil {
		t.Fatalf("local credential pointers = %#v, %#v", local, global)
	}
	if !errors.Is((&QuotaExceededError{Resource: "repositories", Current: 1, Limit: 1}), ErrQuotaExceeded) {
		t.Fatal("quota error does not unwrap to ErrQuotaExceeded")
	}
}
