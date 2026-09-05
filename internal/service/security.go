// Package service implements Meridian business use cases and authorization.
package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMemoryKiB   = 64 * 1024
	passwordIterations  = 3
	passwordParallelism = 1
	passwordSaltBytes   = 16
	passwordHashBytes   = 32
	tokenPepperBytes    = 32
	opaqueTokenBytes    = 32
)

var dummyPasswordHash = sync.OnceValue(func() string {
	salt := make([]byte, passwordSaltBytes)
	return encodePasswordHash("meridian-invalid-credential", salt)
})

// PasswordHasher creates and verifies the frozen Argon2id PHC representation.
type PasswordHasher struct{}

// Hash converts a plaintext password to the frozen Argon2id PHC representation.
func (PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read password salt randomness: %w", err)
	}
	return encodePasswordHash(password, salt), nil
}

// Verify performs an Argon2id derivation and constant-time comparison against a PHC verifier.
func (PasswordHasher) Verify(password, encoded string) bool {
	salt, expected, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemoryKiB, passwordParallelism, passwordHashBytes)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// VerifyDummy performs the same expensive derivation used for a missing login identity.
func (hasher PasswordHasher) VerifyDummy(password string) {
	_ = hasher.Verify(password, dummyPasswordHash())
}

func encodePasswordHash(password string, salt []byte) string {
	hash := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemoryKiB, passwordParallelism, passwordHashBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		passwordMemoryKiB,
		passwordIterations,
		passwordParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func parsePasswordHash(encoded string) ([]byte, []byte, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return nil, nil, false
	}
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", passwordMemoryKiB, passwordIterations, passwordParallelism) {
		return nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != passwordSaltBytes {
		return nil, nil, false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) != passwordHashBytes {
		return nil, nil, false
	}
	return salt, hash, true
}

// TokenDigester generates opaque bearer values and stores only keyed digests.
type TokenDigester struct {
	pepper [tokenPepperBytes]byte
}

// NewTokenDigester validates an unpadded base64url-encoded 32-byte deployment pepper.
func NewTokenDigester(encodedPepper string) (TokenDigester, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encodedPepper)
	if err != nil {
		return TokenDigester{}, errors.New("MERIDIAN_TOKEN_PEPPER must be unpadded base64url")
	}
	if len(decoded) != tokenPepperBytes {
		return TokenDigester{}, fmt.Errorf("MERIDIAN_TOKEN_PEPPER decodes to %d bytes, want %d", len(decoded), tokenPepperBytes)
	}
	var digester TokenDigester
	copy(digester.pepper[:], decoded)
	return digester, nil
}

// NewOpaqueToken creates a prefix plus 32 random bytes encoded without base64 padding.
func (digester TokenDigester) NewOpaqueToken(prefix string) (plaintext string, digest []byte, err error) {
	randomBytes := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, fmt.Errorf("read token randomness: %w", err)
	}
	plaintext = prefix + base64.RawURLEncoding.EncodeToString(randomBytes)
	return plaintext, digester.Digest(plaintext), nil
}

// Digest returns the fixed-width HMAC-SHA-256 digest used for token lookup.
func (digester TokenDigester) Digest(plaintext string) []byte {
	mac := hmac.New(sha256.New, digester.pepper[:])
	_, _ = mac.Write([]byte(plaintext))
	return mac.Sum(nil)
}

// Matches compares a submitted plaintext token with an expected digest in constant time.
func (digester TokenDigester) Matches(plaintext string, expectedDigest []byte) bool {
	return hmac.Equal(digester.Digest(plaintext), expectedDigest)
}
