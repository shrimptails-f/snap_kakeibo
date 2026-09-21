// Package token は JWT と refresh token の発行を提供する。
package token

import (
	"context"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	common "snap_kakeibo/backend/internal/common/domain"

	"github.com/golang-jwt/jwt/v5"
)

// SecretProvider は JWT 署名鍵を提供する。
type SecretProvider interface {
	Secret(ctx context.Context) (string, error)
}

// JWTVerifier は HS256 の access token を検証する。
type JWTVerifier struct {
	Secrets SecretProvider
	Issuer  string
	Clock   interface{ Now() time.Time }
}

type accessTokenClaims struct {
	UserID string `json:"sub"`
	Email  string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// Verify は署名・issuer・期限を検証し、token の利用者情報を返す。
func (v JWTVerifier) Verify(ctx context.Context, raw string) (common.User, error) {
	secret, err := v.Secrets.Secret(ctx)
	if err != nil {
		return common.User{}, err
	}
	claims := &accessTokenClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(parsed *jwt.Token) (any, error) {
		if parsed.Method != jwt.SigningMethodHS256 {
			return nil, application.ErrUnauthorized
		}
		return []byte(secret), nil
	}, jwt.WithIssuer(v.Issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(v.Clock.Now))
	if err != nil || !parsed.Valid {
		return common.User{}, application.ErrUnauthorized
	}
	userID, ok := common.NewUserID(claims.UserID)
	if !ok {
		return common.User{}, application.ErrUnauthorized
	}
	return common.User{ID: userID, Email: claims.Email}, nil
}

// JWTIssuer は HS256 の access token を発行する。
type JWTIssuer struct {
	Secrets SecretProvider
	Issuer  string
	Clock   interface{ Now() time.Time }
	TTL     time.Duration
}

// Issue は user の access token と有効秒数を返す。
func (i JWTIssuer) Issue(ctx context.Context, user common.User) (string, int64, error) {
	secret, err := i.Secrets.Secret(ctx)
	if err != nil {
		return "", 0, err
	}
	now := i.Clock.Now().UTC()
	claims := jwt.MapClaims{
		"iss":   i.Issuer,
		"sub":   user.ID.String(),
		"email": user.Email,
		"iat":   now.Unix(),
		"exp":   now.Add(i.TTL).Unix(),
	}
	encoded, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", 0, err
	}
	return encoded, int64(i.TTL / time.Second), nil
}
