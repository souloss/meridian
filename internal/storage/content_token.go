package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"
	"uuid"
)

const (
	contentTokenVersion = 1
	maximumContentTTL   = 5 * time.Minute
	minimumSigningBytes = 32
)

// ErrInvalidContentToken is returned for malformed, forged, expired, or unsupported capabilities.
var ErrInvalidContentToken = errors.New("invalid content capability")

// ContentGrant binds one short-lived capability to a single immutable artifact.
type ContentGrant struct {
	// BlobDigest identifies the only blob authorized by this capability.
	BlobDigest string `json:"blobDigest"`
	// ArtifactKind identifies the authorized semantic artifact category.
	ArtifactKind string `json:"artifactKind"`
	// MediaType is the exact response Content-Type authorized by this capability.
	MediaType string `json:"mediaType"`
	// Disposition is either inline or attachment for Content-Disposition construction.
	Disposition string `json:"disposition"`
	// ExpiresAt is the UTC instant after which the capability is invalid.
	ExpiresAt time.Time `json:"expiresAt"`
	// ShareLinkID binds anonymous shared content to its database authorization record when present.
	ShareLinkID *uuid.UUID `json:"shareLinkId"`
}

type contentTokenPayload struct {
	// Version identifies the content-token wire format.
	Version int `json:"version"`
	// KeyID selects one configured verification key without exposing key material.
	KeyID string `json:"keyId"`
	ContentGrant
}

// ContentSigner issues and verifies HMAC-SHA-256 artifact capabilities.
type ContentSigner struct {
	activeKeyID string
	keys        map[string][]byte
	now         func() time.Time
}

// NewContentSigner constructs a rotation-aware signer from secret key bytes.
func NewContentSigner(activeKeyID string, keys map[string][]byte) (*ContentSigner, error) {
	if !validIdentifier(activeKeyID) || len(keys) == 0 {
		return nil, errors.New("content signer requires a valid active key identifier")
	}
	copied := make(map[string][]byte, len(keys))
	for keyID, key := range keys {
		if !validIdentifier(keyID) || len(key) < minimumSigningBytes {
			return nil, errors.New("content signing keys require valid identifiers and at least 32 bytes")
		}
		copied[keyID] = bytes.Clone(key)
	}
	if _, ok := copied[activeKeyID]; !ok {
		return nil, errors.New("active content signing key is absent")
	}
	return &ContentSigner{activeKeyID: activeKeyID, keys: copied, now: time.Now}, nil
}

// Issue signs one capability with a positive lifetime no longer than 300 seconds.
func (signer *ContentSigner) Issue(grant ContentGrant, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > maximumContentTTL {
		return "", errors.New("content capability TTL must be between one nanosecond and 300 seconds")
	}
	now := signer.now().UTC()
	grant.ExpiresAt = now.Add(ttl)
	if err := validateContentGrant(grant, now); err != nil {
		return "", err
	}
	payload, err := json.Marshal(contentTokenPayload{
		Version: contentTokenVersion, KeyID: signer.activeKeyID, BlobDigest: grant.BlobDigest,
		ArtifactKind: grant.ArtifactKind, MediaType: grant.MediaType, Disposition: grant.Disposition,
		ExpiresAt: grant.ExpiresAt, ShareLinkID: grant.ShareLinkID,
	})
	if err != nil {
		return "", fmt.Errorf("encode content capability: %w", err)
	}
	signature := signContentPayload(signer.keys[signer.activeKeyID], payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// Verify authenticates and validates one capability before callers perform authorization lookups.
func (signer *ContentSigner) Verify(token string) (ContentGrant, error) {
	payloadPart, signaturePart, ok := strings.Cut(token, ".")
	if !ok || strings.Contains(signaturePart, ".") {
		return ContentGrant{}, ErrInvalidContentToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return ContentGrant{}, ErrInvalidContentToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return ContentGrant{}, ErrInvalidContentToken
	}
	var untrusted contentTokenPayload
	if err := json.Unmarshal(payload, &untrusted); err != nil {
		return ContentGrant{}, ErrInvalidContentToken
	}
	key, knownKey := signer.keys[untrusted.KeyID]
	if !knownKey {
		key = make([]byte, minimumSigningBytes)
	}
	expected := signContentPayload(key, payload)
	if !hmac.Equal(signature, expected) || !knownKey || untrusted.Version != contentTokenVersion {
		return ContentGrant{}, ErrInvalidContentToken
	}
	now := signer.now().UTC()
	if err := validateContentGrant(untrusted.ContentGrant, now); err != nil {
		return ContentGrant{}, ErrInvalidContentToken
	}
	if untrusted.ExpiresAt.After(now.Add(maximumContentTTL)) {
		return ContentGrant{}, ErrInvalidContentToken
	}
	return untrusted.ContentGrant, nil
}

func signContentPayload(key, payload []byte) []byte {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	return digest.Sum(nil)
}

func validateContentGrant(grant ContentGrant, now time.Time) error {
	if !validDigest(grant.BlobDigest) || !validIdentifier(grant.ArtifactKind) {
		return ErrInvalidContentToken
	}
	if grant.Disposition != "inline" && grant.Disposition != "attachment" {
		return ErrInvalidContentToken
	}
	mediaType, _, err := mime.ParseMediaType(grant.MediaType)
	if err != nil || mediaType == "" || strings.ContainsAny(grant.MediaType, "\r\n") {
		return ErrInvalidContentToken
	}
	if grant.ExpiresAt.IsZero() || !grant.ExpiresAt.After(now) {
		return ErrInvalidContentToken
	}
	return nil
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || (index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return true
}
