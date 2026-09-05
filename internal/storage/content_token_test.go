package storage

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestContentSignerIssuesBoundShortLivedCapability(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 14, 0, 0, 0, time.UTC)
	signer, err := NewContentSigner("current", map[string][]byte{
		"current":  bytes.Repeat([]byte{0x11}, minimumSigningBytes),
		"previous": bytes.Repeat([]byte{0x22}, minimumSigningBytes),
	})
	if err != nil {
		t.Fatalf("construct content signer: %v", err)
	}
	signer.now = func() time.Time { return now }
	shareID := uuid.NewV7()
	want := ContentGrant{
		BlobDigest: strings.Repeat("a", digestChars), ArtifactKind: "openapi.normalized",
		MediaType: "application/yaml", Disposition: "inline", ShareLinkID: &shareID,
	}
	token, err := signer.Issue(want, 5*time.Minute)
	if err != nil {
		t.Fatalf("issue content capability: %v", err)
	}
	if strings.Count(token, ".") != 1 {
		t.Fatalf("token format = %q, want payload.signature", token)
	}
	got, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("verify content capability: %v", err)
	}
	want.ExpiresAt = now.Add(5 * time.Minute)
	if got.BlobDigest != want.BlobDigest || got.ArtifactKind != want.ArtifactKind || got.MediaType != want.MediaType || got.Disposition != want.Disposition || !got.ExpiresAt.Equal(want.ExpiresAt) || got.ShareLinkID == nil || *got.ShareLinkID != shareID {
		t.Fatalf("verified grant = %#v, want %#v", got, want)
	}
}

func TestContentSignerRejectsTamperingExpiryAndExcessTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	signer, err := NewContentSigner("current", map[string][]byte{"current": bytes.Repeat([]byte{0x33}, minimumSigningBytes)})
	if err != nil {
		t.Fatalf("construct content signer: %v", err)
	}
	signer.now = func() time.Time { return now }
	grant := ContentGrant{
		BlobDigest: strings.Repeat("b", digestChars), ArtifactKind: "diff.snapshot",
		MediaType: "application/json", Disposition: "attachment",
	}
	if _, err := signer.Issue(grant, maximumContentTTL+time.Nanosecond); err == nil {
		t.Fatal("capability exceeding maximum TTL unexpectedly issued")
	}
	token, err := signer.Issue(grant, time.Minute)
	if err != nil {
		t.Fatalf("issue content capability: %v", err)
	}
	tampered := "A" + token[1:]
	if _, err := signer.Verify(tampered); !errors.Is(err, ErrInvalidContentToken) {
		t.Fatalf("tampered token error = %v", err)
	}
	signer.now = func() time.Time { return now.Add(time.Minute) }
	if _, err := signer.Verify(token); !errors.Is(err, ErrInvalidContentToken) {
		t.Fatalf("expired token error = %v", err)
	}
}

func TestContentSignerValidatesKeyAndArtifactInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewContentSigner("missing", map[string][]byte{"current": bytes.Repeat([]byte{1}, minimumSigningBytes)}); err == nil {
		t.Fatal("missing active signing key unexpectedly accepted")
	}
	if _, err := NewContentSigner("current", map[string][]byte{"current": bytes.Repeat([]byte{1}, minimumSigningBytes-1)}); err == nil {
		t.Fatal("short signing key unexpectedly accepted")
	}
	signer, err := NewContentSigner("current", map[string][]byte{"current": bytes.Repeat([]byte{1}, minimumSigningBytes)})
	if err != nil {
		t.Fatalf("construct content signer: %v", err)
	}
	for _, grant := range []ContentGrant{
		{BlobDigest: "../bad", ArtifactKind: "openapi", MediaType: "application/json", Disposition: "inline"},
		{BlobDigest: strings.Repeat("c", digestChars), ArtifactKind: "Bad Kind", MediaType: "application/json", Disposition: "inline"},
		{BlobDigest: strings.Repeat("c", digestChars), ArtifactKind: "openapi", MediaType: "text/plain\r\nX-Bad: true", Disposition: "inline"},
		{BlobDigest: strings.Repeat("c", digestChars), ArtifactKind: "openapi", MediaType: "application/json", Disposition: "download"},
	} {
		if _, err := signer.Issue(grant, time.Minute); !errors.Is(err, ErrInvalidContentToken) {
			t.Errorf("invalid grant %#v error = %v, want ErrInvalidContentToken", grant, err)
		}
	}
}

func TestContentSignerVerifiesCapabilitiesAcrossKeyRotation(t *testing.T) {
	t.Parallel()
	previousKey := bytes.Repeat([]byte{0x44}, minimumSigningBytes)
	previous, err := NewContentSigner("previous", map[string][]byte{"previous": previousKey})
	if err != nil {
		t.Fatalf("construct previous signer: %v", err)
	}
	now := time.Date(2026, 9, 5, 16, 0, 0, 0, time.UTC)
	previous.now = func() time.Time { return now }
	token, err := previous.Issue(ContentGrant{
		BlobDigest: strings.Repeat("d", digestChars), ArtifactKind: "openapi",
		MediaType: "application/json", Disposition: "inline",
	}, time.Minute)
	if err != nil {
		t.Fatalf("issue previous capability: %v", err)
	}
	current, err := NewContentSigner("current", map[string][]byte{
		"current": bytes.Repeat([]byte{0x55}, minimumSigningBytes), "previous": previousKey,
	})
	if err != nil {
		t.Fatalf("construct rotated signer: %v", err)
	}
	current.now = func() time.Time { return now }
	if _, err := current.Verify(token); err != nil {
		t.Fatalf("verify previous capability after rotation: %v", err)
	}
}
