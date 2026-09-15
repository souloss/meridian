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

// JWTIssuer mints and verifies short-lived browser access tokens.
//
// Access tokens are stateless: the subject is the user UUID and every request
// still resolves the live user row so disabled or deleted users stop being
// honored immediately. The signing key is deployment-only and never leaves
// the process.
type JWTIssuer struct {
	key []byte
	now func() time.Time
}

// NewJWTIssuer validates an unpadded base64url-encoded 32-byte signing key.
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

// IssueAccessToken returns a signed HS256 access token and its remaining lifetime.
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

// VerifyAccessToken validates an HS256 token and returns its user subject.
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
