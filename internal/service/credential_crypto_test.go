package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"uuid"

	"golang.org/x/crypto/ssh"
)

func TestCredentialKeyringEncryptsAndBindsSecrets(t *testing.T) {
	t.Parallel()

	master := base64.StdEncoding.EncodeToString(bytesOf(0x11, credentialKeyBytes))
	fingerprintKey := base64.RawURLEncoding.EncodeToString(bytesOf(0x22, credentialKeyBytes))
	keyring, err := NewCredentialKeyring(master, 7, fingerprintKey)
	if err != nil {
		t.Fatalf("create keyring: %v", err)
	}
	secret := CredentialSecret{Kind: "http_token", HTTPUsername: "build", HTTPToken: "token-value"}
	credentialID := uuid.NewV7()
	encrypted, err := keyring.Encrypt("tenant:acme", credentialID, secret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	if strings.Contains(string(encrypted.Ciphertext), secret.HTTPToken) || len(encrypted.Nonce) != credentialNonceBytes {
		t.Fatalf("encrypted projection leaked plaintext or nonce has wrong size: %#v", encrypted)
	}
	decrypted, err := keyring.Decrypt("tenant:acme", credentialID, secret.Kind, encrypted)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if decrypted != secret {
		t.Fatalf("decrypted secret = %#v, want %#v", decrypted, secret)
	}
	if _, err := keyring.Decrypt("tenant:other", credentialID, secret.Kind, encrypted); err == nil {
		t.Fatal("decrypt accepted a different tenant scope")
	}
	if _, err := keyring.Decrypt("tenant:acme", uuid.NewV7(), secret.Kind, encrypted); err == nil {
		t.Fatal("decrypt accepted a different credential ID")
	}
}

func TestCredentialKeyringSupportsPreviousMasterVersion(t *testing.T) {
	t.Parallel()

	fingerprintKey := base64.RawURLEncoding.EncodeToString(bytesOf(0x33, credentialKeyBytes))
	oldMaster := base64.StdEncoding.EncodeToString(bytesOf(0x44, credentialKeyBytes))
	newMaster := base64.StdEncoding.EncodeToString(bytesOf(0x55, credentialKeyBytes))
	old, err := NewCredentialKeyring(oldMaster, 1, fingerprintKey)
	if err != nil {
		t.Fatalf("create old keyring: %v", err)
	}
	credentialID := uuid.NewV7()
	encrypted, err := old.Encrypt("global", credentialID, CredentialSecret{Kind: "http_token", HTTPUsername: "svc", HTTPToken: "secret-token"})
	if err != nil {
		t.Fatalf("encrypt old secret: %v", err)
	}
	rotated, err := NewCredentialKeyringWithPrevious(newMaster, 2, fingerprintKey, map[int32]string{1: oldMaster})
	if err != nil {
		t.Fatalf("create rotated keyring: %v", err)
	}
	if _, err := rotated.Decrypt("global", credentialID, "http_token", encrypted); err != nil {
		t.Fatalf("decrypt with previous key: %v", err)
	}
}

func TestCredentialFingerprintsAreStableAndNonSecret(t *testing.T) {
	t.Parallel()

	fingerprintKey := base64.RawURLEncoding.EncodeToString(bytesOf(0x66, credentialKeyBytes))
	keyring, err := NewCredentialKeyring(base64.StdEncoding.EncodeToString(bytesOf(0x77, credentialKeyBytes)), 1, fingerprintKey)
	if err != nil {
		t.Fatalf("create keyring: %v", err)
	}
	first, err := keyring.Fingerprint(CredentialSecret{Kind: "http_token", HTTPUsername: "svc", HTTPToken: "secret-token"})
	if err != nil {
		t.Fatalf("fingerprint first token: %v", err)
	}
	second, err := keyring.Fingerprint(CredentialSecret{Kind: "http_token", HTTPUsername: "svc", HTTPToken: "rotated-token"})
	if err != nil {
		t.Fatalf("fingerprint rotated token: %v", err)
	}
	if first == second || strings.Contains(first, "secret-token") || !strings.HasPrefix(first, "HMAC-SHA256:") {
		t.Fatalf("unexpected HTTP fingerprints: %q, %q", first, second)
	}
}

func TestParseKnownHostPublicKeyDerivesOnlyServerFields(t *testing.T) {
	t.Parallel()

	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	sshKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatalf("construct SSH public key: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(sshKey.Marshal())
	identity, err := ParseKnownHostPublicKey(encoded)
	if err != nil {
		t.Fatalf("parse known host key: %v", err)
	}
	if identity.KeyType != ssh.KeyAlgoED25519 || len(identity.PublicKey) == 0 || !strings.HasPrefix(identity.Fingerprint, "SHA256:") {
		t.Fatalf("derived host identity = %#v", identity)
	}
	if _, err := ParseKnownHostPublicKey(base64.StdEncoding.EncodeToString([]byte("not-an-rfc4253-key"))); err == nil {
		t.Fatal("accepted malformed known host key")
	}
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}
