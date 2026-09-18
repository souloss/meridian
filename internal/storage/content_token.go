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

// 内容令牌（content capability）域常量。
const (
	// contentTokenVersion 是内容令牌线上格式的版本号。
	contentTokenVersion = 1
	// maximumContentTTL 是单次签发能力允许的最大有效期。
	maximumContentTTL = 5 * time.Minute
	// minimumSigningBytes 是签名密钥允许的最小字节长度。
	minimumSigningBytes = 32
	// maximumIdentifierLength 是签名键标识与工件类别的最大字符长度。
	maximumIdentifierLength = 64
	// dispositionInline 表示 Content-Disposition 的内联展示方式。
	dispositionInline = "inline"
	// dispositionAttachment 表示 Content-Disposition 的附件下载方式。
	dispositionAttachment = "attachment"
	// contentTokenSeparator 是内容令牌中载荷与签名之间的分隔符。
	contentTokenSeparator = "."
)

// 内容令牌签发与校验的包内哨兵错误。
var (
	// errContentSignerInvalidActiveKey 表示活动键标识非法或未提供任何密钥。
	errContentSignerInvalidActiveKey = errors.New("content signer requires a valid active key identifier")
	// errContentSigningKeyTooShort 表示存在标识非法或长度不足的签名密钥。
	errContentSigningKeyTooShort = errors.New("content signing keys require valid identifiers and at least 32 bytes")
	// errContentSignerActiveKeyAbsent 表示活动签名密钥不在已配置密钥集合中。
	errContentSignerActiveKeyAbsent = errors.New("active content signing key is absent")
	// errContentCapabilityTTLOutOfRange 表示能力有效期不在 (0, maximumContentTTL] 区间内。
	errContentCapabilityTTLOutOfRange = errors.New("content capability TTL must be between one nanosecond and 300 seconds")
)

// ErrInvalidContentToken 在能力畸形、伪造、过期或不支持时返回。
var ErrInvalidContentToken = errors.New("invalid content capability")

// ContentGrant 将一个短期能力绑定到单个不可变工件。
type ContentGrant struct {
	// BlobDigest 标识该能力唯一授权的 blob。
	BlobDigest string `json:"blobDigest"`
	// ArtifactKind 标识被授权的语义工件类别。
	ArtifactKind string `json:"artifactKind"`
	// MediaType 是该能力授权的精确响应 Content-Type。
	MediaType string `json:"mediaType"`
	// Disposition 是 inline 或 attachment，用于构造 Content-Disposition。
	Disposition string `json:"disposition"`
	// ExpiresAt 是能力失效之后的 UTC 时刻。
	ExpiresAt time.Time `json:"expiresAt"`
	// ShareLinkID 在存在时将匿名共享内容绑定到其数据库授权记录。
	ShareLinkID *uuid.UUID `json:"shareLinkId"`
}

// contentTokenPayload 是内容令牌的线上载荷结构。
type contentTokenPayload struct {
	// Version 标识内容令牌线上格式版本。
	Version int `json:"version"`
	// KeyID 在不暴露密钥材料的情况下选择一把已配置的校验密钥。
	KeyID string `json:"keyId"`
	ContentGrant
}

// ContentSigner 签发并校验 HMAC-SHA-256 工件能力。
type ContentSigner struct {
	activeKeyID string
	keys        map[string][]byte
	now         func() time.Time
}

// NewContentSigner 从密钥字节构造一个支持密钥轮换的签发器。
func NewContentSigner(activeKeyID string, keys map[string][]byte) (*ContentSigner, error) {
	if !validIdentifier(activeKeyID) || len(keys) == 0 {
		return nil, errContentSignerInvalidActiveKey
	}
	copied := make(map[string][]byte, len(keys))
	for keyID, key := range keys {
		if !validIdentifier(keyID) || len(key) < minimumSigningBytes {
			return nil, errContentSigningKeyTooShort
		}
		copied[keyID] = bytes.Clone(key)
	}
	if _, ok := copied[activeKeyID]; !ok {
		return nil, errContentSignerActiveKeyAbsent
	}
	return &ContentSigner{activeKeyID: activeKeyID, keys: copied, now: time.Now}, nil
}

// Issue 签发一个有效期不超过 maximumContentTTL 的正向能力。
func (signer *ContentSigner) Issue(grant ContentGrant, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > maximumContentTTL {
		return "", errContentCapabilityTTLOutOfRange
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
	return base64.RawURLEncoding.EncodeToString(payload) + contentTokenSeparator + base64.RawURLEncoding.EncodeToString(signature), nil
}

// Verify 在调用方执行授权查询之前认证并校验一个能力。
func (signer *ContentSigner) Verify(token string) (ContentGrant, error) {
	payloadPart, signaturePart, ok := strings.Cut(token, contentTokenSeparator)
	if !ok || strings.Contains(signaturePart, contentTokenSeparator) {
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

// signContentPayload 使用 HMAC-SHA-256 计算载荷签名。
func signContentPayload(key, payload []byte) []byte {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	return digest.Sum(nil)
}

// validateContentGrant 校验能力字段是否满足签发约束。
func validateContentGrant(grant ContentGrant, now time.Time) error {
	if !validDigest(grant.BlobDigest) || !validIdentifier(grant.ArtifactKind) {
		return ErrInvalidContentToken
	}
	if grant.Disposition != dispositionInline && grant.Disposition != dispositionAttachment {
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

// validIdentifier 判断一个字符串是否为合法的键标识符（小写字母数字开头，可含 . _ -）。
func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > maximumIdentifierLength {
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
