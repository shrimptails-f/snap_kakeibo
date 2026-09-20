package application

import (
	"context"
	"time"

	authdomain "snap_kakeibo/backend/internal/auth/domain"
	common "snap_kakeibo/backend/internal/common/domain"
)

// UserRepository はメールアドレスで利用者を取得する。
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (common.User, error)
}

// PasswordComparator はパスワードハッシュを照合する。
type PasswordComparator interface {
	Compare(hash, password string) error
}

// AccessTokenIssuer は access token を発行する。
type AccessTokenIssuer interface {
	Issue(ctx context.Context, user common.User) (token string, expiresIn int64, err error)
}

// RefreshTokenGenerator は保存用 digest を含む refresh token を生成する。
type RefreshTokenGenerator interface {
	Generate(user common.User, now time.Time, expiresAt time.Time) (raw string, token authdomain.RefreshToken, err error)
}

// RefreshTokenRepository は refresh token を永続化する。
type RefreshTokenRepository interface {
	Save(ctx context.Context, token authdomain.RefreshToken) error
}
