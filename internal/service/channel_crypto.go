package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"uuid"
)

// 通道配置加密常量（值来源 domain.yaml credential.derivation 同族；info 独立避免跨用途键复用）。
const (
	// channelConfigKeyBytes 是通道配置加密密钥的字节长度。
	channelConfigKeyBytes = 32
	// channelConfigNonceBytes 是通道配置 AES-GCM nonce 的字节长度。
	channelConfigNonceBytes = 12
	// channelConfigHKDFInfo 是通道配置 HKDF 派生 info 常量。
	channelConfigHKDFInfo = "meridian-channel-config-v1"
)

// notificationChannelConfig 是通道加密配置的明文载荷（webhook 端点 + 秘密，email 邮箱）。
type notificationChannelConfig struct {
	Endpoint string `json:"endpoint,omitempty"`
	Secret   string `json:"secret,omitempty"`
}

// sealChannelConfig 使用活跃主密钥按行身份加密通道配置。
func (keyring CredentialKeyring) sealChannelConfig(tenantID uuid.UUID, channelID uuid.UUID, cfg notificationChannelConfig) ([]byte, error) {
	key, err := keyring.channelRowKey(tenantID, channelID, keyring.activeVersion)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create channel cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithNonceSize(block, channelConfigNonceBytes)
	if err != nil {
		return nil, fmt.Errorf("create channel AEAD: %w", err)
	}
	nonce := make([]byte, channelConfigNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read channel nonce randomness: %w", err)
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode channel config: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, payload, channelConfigAAD(tenantID, channelID, keyring.activeVersion))
	return append(append([]byte{byte(keyring.activeVersion)}, nonce...), ciphertext...), nil
}

// openChannelConfig 解密通道配置并校验其行身份绑定。
func (keyring CredentialKeyring) openChannelConfig(tenantID uuid.UUID, channelID uuid.UUID, encrypted []byte) (notificationChannelConfig, error) {
	if len(encrypted) < 1+channelConfigNonceBytes {
		return notificationChannelConfig{}, ErrCredentialSecretInvalid
	}
	version := int32(encrypted[0])
	nonce := encrypted[1 : 1+channelConfigNonceBytes]
	ciphertext := encrypted[1+channelConfigNonceBytes:]
	key, err := keyring.channelRowKey(tenantID, channelID, version)
	if err != nil {
		return notificationChannelConfig{}, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return notificationChannelConfig{}, fmt.Errorf("create channel cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithNonceSize(block, channelConfigNonceBytes)
	if err != nil {
		return notificationChannelConfig{}, fmt.Errorf("create channel AEAD: %w", err)
	}
	payload, err := aead.Open(nil, nonce, ciphertext, channelConfigAAD(tenantID, channelID, version))
	if err != nil {
		return notificationChannelConfig{}, fmt.Errorf("%w: decrypt channel config", ErrCredentialSecretInvalid)
	}
	var cfg notificationChannelConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return notificationChannelConfig{}, fmt.Errorf("%w: decode channel config", ErrCredentialSecretInvalid)
	}
	return cfg, nil
}

func (keyring CredentialKeyring) channelRowKey(tenantID uuid.UUID, channelID uuid.UUID, version int32) ([channelConfigKeyBytes]byte, error) {
	master, ok := keyring.keys[version]
	if !ok {
		return [channelConfigKeyBytes]byte{}, fmt.Errorf("%w: %d", ErrCredentialKeyUnavailable, version)
	}
	derived, err := hkdf.Key(sha256.New, master[:], []byte(tenantID.String()+"\x00"+channelID.String()), channelConfigHKDFInfo, channelConfigKeyBytes)
	if err != nil {
		return [channelConfigKeyBytes]byte{}, fmt.Errorf("derive channel config key: %w", err)
	}
	var key [channelConfigKeyBytes]byte
	copy(key[:], derived)
	return key, nil
}

func channelConfigAAD(tenantID uuid.UUID, channelID uuid.UUID, version int32) []byte {
	return []byte(strings.Join([]string{tenantID.String(), channelID.String(), strconv.FormatInt(int64(version), 10)}, "\x00"))
}
