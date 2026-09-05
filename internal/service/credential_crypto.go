package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"uuid"

	"golang.org/x/crypto/ssh"
)

const (
	credentialKeyBytes   = 32
	credentialNonceBytes = 12
	credentialHKDFInfo   = "meridian-credential-v1"
)

var (
	// ErrCredentialSecretInvalid means a credential secret failed structural or cryptographic validation.
	ErrCredentialSecretInvalid = errors.New("credential secret is invalid")
	// ErrCredentialKeyUnavailable means an encrypted row references no configured master-key version.
	ErrCredentialKeyUnavailable = errors.New("credential master-key version is unavailable")
)

// CredentialSecret is write-only material accepted by the credential use cases.
// Exactly one of SSH or HTTP is populated according to Kind.
type CredentialSecret struct {
	Kind         string
	PrivateKey   string
	Passphrase   *string
	HTTPUsername string
	HTTPToken    string
}

// EncryptedCredential is the storage projection of one encrypted secret.
type EncryptedCredential struct {
	Ciphertext  []byte
	Nonce       []byte
	KeyVersion  int32
	Fingerprint string
}

// KnownHostIdentity is the server-derived identity of an RFC 4253 public key blob.
type KnownHostIdentity struct {
	KeyType     string
	PublicKey   []byte
	Fingerprint string
}

// CredentialKeyring encrypts credential rows with per-row HKDF-derived AES-256-GCM keys.
type CredentialKeyring struct {
	activeVersion  int32
	keys           map[int32][credentialKeyBytes]byte
	fingerprintKey [credentialKeyBytes]byte
}

// NewCredentialKeyring validates active master and fingerprint keys.
func NewCredentialKeyring(masterKey string, activeVersion int32, fingerprintKey string) (CredentialKeyring, error) {
	master, err := decodeBase64Key(masterKey, base64.StdEncoding)
	if err != nil {
		return CredentialKeyring{}, fmt.Errorf("MASTER_KEY: %w", err)
	}
	fingerprint, err := decodeBase64Key(fingerprintKey, base64.RawURLEncoding)
	if err != nil {
		return CredentialKeyring{}, fmt.Errorf("CREDENTIAL_FINGERPRINT_KEY: %w", err)
	}
	if activeVersion < 1 {
		return CredentialKeyring{}, errors.New("MASTER_KEY_VERSION must be positive")
	}
	var masterArray, fingerprintArray [credentialKeyBytes]byte
	copy(masterArray[:], master)
	copy(fingerprintArray[:], fingerprint)
	return CredentialKeyring{
		activeVersion:  activeVersion,
		keys:           map[int32][credentialKeyBytes]byte{activeVersion: masterArray},
		fingerprintKey: fingerprintArray,
	}, nil
}

// NewCredentialKeyringWithPrevious adds validated previous master-key versions for decrypt-only use.
func NewCredentialKeyringWithPrevious(masterKey string, activeVersion int32, fingerprintKey string, previous map[int32]string) (CredentialKeyring, error) {
	keyring, err := NewCredentialKeyring(masterKey, activeVersion, fingerprintKey)
	if err != nil {
		return CredentialKeyring{}, err
	}
	for version, encoded := range previous {
		if version < 1 || version >= activeVersion {
			return CredentialKeyring{}, fmt.Errorf("previous master-key version %d must be positive and lower than active version %d", version, activeVersion)
		}
		decoded, err := decodeBase64Key(encoded, base64.StdEncoding)
		if err != nil {
			return CredentialKeyring{}, fmt.Errorf("MASTER_KEY_PREVIOUS_FILE version %d: %w", version, err)
		}
		var array [credentialKeyBytes]byte
		copy(array[:], decoded)
		if _, exists := keyring.keys[version]; exists {
			return CredentialKeyring{}, fmt.Errorf("duplicate master-key version %d", version)
		}
		keyring.keys[version] = array
	}
	return keyring, nil
}

// Encrypt validates and encrypts one credential secret for its immutable row identity.
func (keyring CredentialKeyring) Encrypt(scopeID string, credentialID uuid.UUID, secret CredentialSecret) (EncryptedCredential, error) {
	if err := validateCredentialSecret(secret); err != nil {
		return EncryptedCredential{}, err
	}
	key, err := keyring.rowKey(scopeID, credentialID, secret.Kind, keyring.activeVersion)
	if err != nil {
		return EncryptedCredential{}, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return EncryptedCredential{}, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithNonceSize(block, credentialNonceBytes)
	if err != nil {
		return EncryptedCredential{}, fmt.Errorf("create credential AEAD: %w", err)
	}
	nonce := make([]byte, credentialNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedCredential{}, fmt.Errorf("read credential nonce randomness: %w", err)
	}
	payload, err := json.Marshal(secretEnvelope(secret))
	if err != nil {
		return EncryptedCredential{}, fmt.Errorf("encode credential secret: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, payload, credentialAAD(scopeID, credentialID, secret.Kind, keyring.activeVersion))
	fingerprint, err := keyring.Fingerprint(secret)
	if err != nil {
		return EncryptedCredential{}, err
	}
	return EncryptedCredential{Ciphertext: ciphertext, Nonce: nonce, KeyVersion: keyring.activeVersion, Fingerprint: fingerprint}, nil
}

// Decrypt verifies and decrypts one stored credential secret under its row-bound AAD.
func (keyring CredentialKeyring) Decrypt(scopeID string, credentialID uuid.UUID, kind string, encrypted EncryptedCredential) (CredentialSecret, error) {
	if len(encrypted.Nonce) != credentialNonceBytes || encrypted.KeyVersion < 1 {
		return CredentialSecret{}, ErrCredentialSecretInvalid
	}
	key, err := keyring.rowKey(scopeID, credentialID, kind, encrypted.KeyVersion)
	if err != nil {
		return CredentialSecret{}, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return CredentialSecret{}, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithNonceSize(block, credentialNonceBytes)
	if err != nil {
		return CredentialSecret{}, fmt.Errorf("create credential AEAD: %w", err)
	}
	payload, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, credentialAAD(scopeID, credentialID, kind, encrypted.KeyVersion))
	if err != nil {
		return CredentialSecret{}, fmt.Errorf("%w: decrypt credential secret", ErrCredentialSecretInvalid)
	}
	var envelope credentialSecretEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return CredentialSecret{}, fmt.Errorf("%w: decode credential secret", ErrCredentialSecretInvalid)
	}
	secret := envelope.secret()
	if secret.Kind != kind || validateCredentialSecret(secret) != nil {
		return CredentialSecret{}, ErrCredentialSecretInvalid
	}
	return secret, nil
}

// Fingerprint derives a stable non-secret credential identity for rotation comparison.
func (keyring CredentialKeyring) Fingerprint(secret CredentialSecret) (string, error) {
	if err := validateCredentialSecret(secret); err != nil {
		return "", err
	}
	if secret.Kind == "ssh_key" {
		signer, err := parsePrivateKey(secret)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(signer.PublicKey().Marshal())
		return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]), nil
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len([]byte(secret.HTTPUsername))))
	mac := hmac.New(sha256.New, keyring.fingerprintKey[:])
	_, _ = mac.Write(length[:])
	_, _ = mac.Write([]byte(secret.HTTPUsername))
	_, _ = mac.Write([]byte(secret.HTTPToken))
	return "HMAC-SHA256:" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// ParseKnownHostPublicKey strictly derives key type and fingerprint from an RFC 4253 blob.
func ParseKnownHostPublicKey(encoded string) (KnownHostIdentity, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return KnownHostIdentity{}, fmt.Errorf("%w: public key is not standard base64", ErrCredentialSecretInvalid)
	}
	key, err := ssh.ParsePublicKey(decoded)
	if err != nil || !bytes.Equal(key.Marshal(), decoded) {
		return KnownHostIdentity{}, fmt.Errorf("%w: public key is not a complete RFC 4253 key", ErrCredentialSecretInvalid)
	}
	if key.Type() != ssh.KeyAlgoED25519 && key.Type() != ssh.KeyAlgoRSA {
		return KnownHostIdentity{}, fmt.Errorf("%w: unsupported SSH host key type %q", ErrCredentialSecretInvalid, key.Type())
	}
	digest := sha256.Sum256(decoded)
	return KnownHostIdentity{
		KeyType: key.Type(), PublicKey: bytes.Clone(decoded),
		Fingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]),
	}, nil
}

func (keyring CredentialKeyring) rowKey(scopeID string, credentialID uuid.UUID, kind string, version int32) ([credentialKeyBytes]byte, error) {
	master, ok := keyring.keys[version]
	if !ok {
		return [credentialKeyBytes]byte{}, fmt.Errorf("%w: %d", ErrCredentialKeyUnavailable, version)
	}
	derived, err := hkdf.Key(sha256.New, master[:], []byte(scopeID+"\x00"+credentialID.String()), credentialHKDFInfo, credentialKeyBytes)
	if err != nil {
		return [credentialKeyBytes]byte{}, fmt.Errorf("derive credential row key: %w", err)
	}
	var key [credentialKeyBytes]byte
	copy(key[:], derived)
	_ = kind
	return key, nil
}

func credentialAAD(scopeID string, credentialID uuid.UUID, kind string, version int32) []byte {
	return []byte(strings.Join([]string{scopeID, credentialID.String(), kind, strconv.FormatInt(int64(version), 10)}, "\x00"))
}

func decodeBase64Key(encoded string, encoding *base64.Encoding) ([]byte, error) {
	decoded, err := encoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("must be valid base64")
	}
	if len(decoded) != credentialKeyBytes {
		return nil, fmt.Errorf("must decode to exactly %d bytes", credentialKeyBytes)
	}
	return decoded, nil
}

type credentialSecretEnvelope struct {
	Kind         string  `json:"kind"`
	PrivateKey   string  `json:"privateKey,omitempty"`
	Passphrase   *string `json:"passphrase,omitempty"`
	HTTPUsername string  `json:"httpUsername,omitempty"`
	HTTPToken    string  `json:"httpToken,omitempty"`
}

func secretEnvelope(secret CredentialSecret) credentialSecretEnvelope {
	return credentialSecretEnvelope{
		Kind: secret.Kind, PrivateKey: secret.PrivateKey, Passphrase: secret.Passphrase,
		HTTPUsername: secret.HTTPUsername, HTTPToken: secret.HTTPToken,
	}
}

func (envelope credentialSecretEnvelope) secret() CredentialSecret {
	return CredentialSecret{
		Kind: envelope.Kind, PrivateKey: envelope.PrivateKey, Passphrase: envelope.Passphrase,
		HTTPUsername: envelope.HTTPUsername, HTTPToken: envelope.HTTPToken,
	}
}

func validateCredentialSecret(secret CredentialSecret) error {
	switch secret.Kind {
	case "ssh_key":
		if strings.TrimSpace(secret.PrivateKey) == "" {
			return fmt.Errorf("%w: SSH private key is required", ErrCredentialSecretInvalid)
		}
		if _, err := parsePrivateKey(secret); err != nil {
			return err
		}
	case "http_token":
		if strings.TrimSpace(secret.HTTPUsername) == "" || len([]byte(secret.HTTPUsername)) > 128 || len([]byte(secret.HTTPToken)) < 8 {
			return fmt.Errorf("%w: HTTP username and token are invalid", ErrCredentialSecretInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported credential kind %q", ErrCredentialSecretInvalid, secret.Kind)
	}
	return nil
}

func parsePrivateKey(secret CredentialSecret) (ssh.Signer, error) {
	privateKey := []byte(secret.PrivateKey)
	var signer ssh.Signer
	var err error
	if secret.Passphrase != nil {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(privateKey, []byte(*secret.Passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(privateKey)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: parse SSH private key", ErrCredentialSecretInvalid)
	}
	return signer, nil
}
