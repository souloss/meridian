package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// 凭据与通道配置加密共用的底层原语：HKDF-SHA256 按行派生密钥 + AES-256-GCM。
// 两条业务路径（credential_crypto.go / channel_crypto.go）刻意保留各自的
// HKDF info 串与 AAD 组成，仅共享此处与领域无关的加解密原语。
const (
	// aesGCMKeyBytes 是 AES-256 密钥的字节长度。
	aesGCMKeyBytes = 32
	// aesGCMNonceBytes 是 AES-GCM 标准 nonce 的字节长度。
	aesGCMNonceBytes = 12
)

// deriveRowKey 从主密钥与行盐经 HKDF-SHA256 派生出 AES-256 密钥。
// info 是用途分离常量，由调用方传入（凭据与通道各不相同）。
func deriveRowKey(master []byte, salt []byte, info string) ([aesGCMKeyBytes]byte, error) {
	derived, err := hkdf.Key(sha256.New, master, salt, info, aesGCMKeyBytes)
	if err != nil {
		return [aesGCMKeyBytes]byte{}, fmt.Errorf("derive row key: %w", err)
	}
	var key [aesGCMKeyBytes]byte
	copy(key[:], derived)
	return key, nil
}

// sealAESGCM 用随机 nonce 加密明文并返回 (nonce, ciphertext)。
// 返回的 nonce 需由调用方与密文一并持久化。
func sealAESGCM(key [aesGCMKeyBytes]byte, plaintext, aad []byte) ([]byte, []byte, error) {
	aead, err := newAESGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aesGCMNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("read nonce randomness: %w", err)
	}
	return nonce, aead.Seal(nil, nonce, plaintext, aad), nil
}

// openAESGCM 用给定 nonce 解密并校验密文的完整性。
func openAESGCM(key [aesGCMKeyBytes]byte, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := newAESGCM(key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

// newAESGCM 构造一个 AES-256-GCM AEAD 实例。
func newAESGCM(key [aesGCMKeyBytes]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	return cipher.NewGCMWithNonceSize(block, aesGCMNonceBytes)
}
