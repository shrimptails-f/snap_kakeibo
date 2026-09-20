package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	authdomain "snap_kakeibo/backend/internal/auth/domain"
	common "snap_kakeibo/backend/internal/common/domain"
)

// RefreshTokenGenerator は暗号学的乱数で refresh token を発行する。
type RefreshTokenGenerator struct{}

// Generate はクライアントへ返す raw token と、保存用の digest を生成する。
func (RefreshTokenGenerator) Generate(user common.User, now, expiresAt time.Time) (string, authdomain.RefreshToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", authdomain.RefreshToken{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	return value, authdomain.RefreshToken{
		Digest:     Digest(value),
		UserID:     user.ID,
		Email:      user.Email,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		LastUsedAt: now,
		Hint:       value[len(value)-8:],
	}, nil
}

// Digest は raw refresh token の保存用 SHA-256 digest を返す。
func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
