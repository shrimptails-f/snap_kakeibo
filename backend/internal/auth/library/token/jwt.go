// Package token は JWT と refresh token の発行を提供する。
package token

import (
	"context"
	"fmt"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"

	"github.com/golang-jwt/jwt/v5"
)

// SecretProvider は JWT 署名鍵を提供する。
type SecretProvider interface {
	Secret(ctx context.Context) (string, error)
}

// StaticSecretProvider は環境変数から取得済みの署名鍵を返す。
type StaticSecretProvider struct{ Value string }

// Secret は設定済みの署名鍵を返す。
func (p StaticSecretProvider) Secret(context.Context) (string, error) {
	if strings.TrimSpace(p.Value) == "" {
		return "", fmt.Errorf("JWT secret is empty")
	}
	return p.Value, nil
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
