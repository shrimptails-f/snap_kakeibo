package application

import (
	"context"
	"errors"
	"time"

	authdomain "snap_kakeibo/backend/internal/auth/domain"
	common "snap_kakeibo/backend/internal/common/domain"
)

// ErrTooManyLoginAttempts はログイン試行が短時間の上限に達した場合に返す。
var ErrTooManyLoginAttempts = errors.New("too many login attempts")

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

// LoginAttemptLimiter はログイン試行を識別子ごとに制限する。
// subject には IP アドレスや正規化済みメールアドレスを渡す。
type LoginAttemptLimiter interface {
	Allow(ctx context.Context, subject string) (bool, error)
}
