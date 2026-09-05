package service

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPasswordHasherRoundTrip(t *testing.T) {
	t.Parallel()

	hasher := PasswordHasher{}
	encoded, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatalf("password hash has unexpected parameters: %q", encoded)
	}
	if !hasher.Verify("correct horse battery staple", encoded) {
		t.Fatal("correct password did not verify")
	}
	if hasher.Verify("wrong password", encoded) {
		t.Fatal("incorrect password verified")
	}
}

func TestTokenDigesterFormatAndMatching(t *testing.T) {
	t.Parallel()

	pepper := base64.RawURLEncoding.EncodeToString(make([]byte, tokenPepperBytes))
	digester, err := NewTokenDigester(pepper)
	if err != nil {
		t.Fatalf("create token digester: %v", err)
	}
	plaintext, digest, err := digester.NewOpaqueToken(patTokenPrefix)
	if err != nil {
		t.Fatalf("create PAT: %v", err)
	}
	if !validOpaqueToken(plaintext, patTokenPrefix) {
		t.Fatalf("PAT has invalid format: %q", plaintext)
	}
	if len(digest) != 32 {
		t.Fatalf("digest length = %d, want 32", len(digest))
	}
	if !digester.Matches(plaintext, digest) {
		t.Fatal("plaintext token did not match its digest")
	}
	if digester.Matches(plaintext+"x", digest) {
		t.Fatal("modified token matched digest")
	}
}

func TestTokenDigesterRejectsInvalidPepper(t *testing.T) {
	t.Parallel()

	for _, pepper := range []string{"", "not-base64!", base64.RawURLEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := NewTokenDigester(pepper); err == nil {
			t.Errorf("accepted invalid pepper %q", pepper)
		}
	}
}
