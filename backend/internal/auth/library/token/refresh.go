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

// Digest は Generate が保存用に使う digest と同じ算出方法で raw token を変換する。
// refresh 時に受け取った raw token から保存済みトークンを探すために使う。
func (RefreshTokenGenerator) Digest(raw string) string { return Digest(raw) }

// Digest は raw refresh token の保存用 SHA-256 digest を返す。
func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
