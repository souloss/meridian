package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenTTL       = 15 * time.Minute
	jwtSigningKeyBytes   = 32
	jwtTokenIDBytes      = 16
	jwtSubjectClaim      = "sub"
	jwtIssuedAtClaim     = "iat"
	jwtExpiresAtClaim    = "exp"
	jwtTokenIDClaim      = "jti"
	jwtSigningMethodName = "HS256"
)

// JWTIssuer 铸造并校验短时浏览器访问令牌。
//
// 访问令牌无状态：主体是用户 UUID，且每个请求仍解析实时用户行，使被禁用或删除的
// 用户立即停止被认可。签名密钥仅存在于部署环境且绝不离开进程。
type JWTIssuer struct {
	key []byte
	now func() time.Time
}

// NewJWTIssuer 校验未填充 base64url 编码的 32 字节签名密钥。
func NewJWTIssuer(encodedKey string) (*JWTIssuer, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("MERIDIAN_JWT_SIGNING_KEY must be unpadded base64url")
	}
	if len(decoded) != jwtSigningKeyBytes {
		return nil, fmt.Errorf("MERIDIAN_JWT_SIGNING_KEY decodes to %d bytes, want %d", len(decoded), jwtSigningKeyBytes)
	}
	return &JWTIssuer{key: decoded, now: time.Now}, nil
}

// IssueAccessToken 返回签名的 HS256 访问令牌及其剩余有效时长。
func (issuer *JWTIssuer) IssueAccessToken(userID uuid.UUID) (string, time.Duration, error) {
	now := issuer.now().UTC()
	expiresAt := now.Add(accessTokenTTL)
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		ID:        uuid.NewV7().String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(issuer.key)
	if err != nil {
		return "", 0, fmt.Errorf("sign access token: %w", err)
	}
	return signed, accessTokenTTL, nil
}

// VerifyAccessToken 校验 HS256 令牌并返回其用户主体。
func (issuer *JWTIssuer) VerifyAccessToken(plaintext string) (uuid.UUID, error) {
	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(plaintext, &claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwtSigningMethodName {
			return nil, fmt.Errorf("unexpected signing method %q", token.Method.Alg())
		}
		return issuer.key, nil
	}, jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return uuid.Nil(), ErrUnauthenticated
	}
	if !token.Valid {
		return uuid.Nil(), ErrUnauthenticated
	}
	subject, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil(), ErrUnauthenticated
	}
	return subject, nil
}
