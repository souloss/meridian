// Package service 实现 Meridian 业务用例与授权。
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

// PasswordHasher 创建并校验冻结的 Argon2id PHC 表示。
type PasswordHasher struct{}

// Hash 将明文密码转换为冻结的 Argon2id PHC 表示。
func (PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read password salt randomness: %w", err)
	}
	return encodePasswordHash(password, salt), nil
}

// Verify 对 PHC 校验符执行 Argon2id 派生与常量时间比较。
func (PasswordHasher) Verify(password, encoded string) bool {
	salt, expected, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemoryKiB, passwordParallelism, passwordHashBytes)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// VerifyDummy 执行与缺失登录身份相同的昂贵派生。
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

// TokenDigester 生成不透明承载值并仅存储密钥化摘要。
type TokenDigester struct {
	pepper [tokenPepperBytes]byte
}

// NewTokenDigester 校验未填充 base64url 编码的 32 字节部署 pepper。
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

// NewOpaqueToken 创建前缀加 32 随机字节、不带 base64 填充编码的令牌。
func (digester TokenDigester) NewOpaqueToken(prefix string) (plaintext string, digest []byte, err error) {
	randomBytes := make([]byte, opaqueTokenBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, fmt.Errorf("read token randomness: %w", err)
	}
	plaintext = prefix + base64.RawURLEncoding.EncodeToString(randomBytes)
	return plaintext, digester.Digest(plaintext), nil
}

// Digest 返回用于令牌查找的定宽 HMAC-SHA-256 摘要。
func (digester TokenDigester) Digest(plaintext string) []byte {
	mac := hmac.New(sha256.New, digester.pepper[:])
	_, _ = mac.Write([]byte(plaintext))
	return mac.Sum(nil)
}

// Matches 以常量时间比较提交的明文令牌与期望摘要。
func (digester TokenDigester) Matches(plaintext string, expectedDigest []byte) bool {
	return hmac.Equal(digester.Digest(plaintext), expectedDigest)
}
