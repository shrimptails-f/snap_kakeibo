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

// RefreshTokenDigester は raw refresh token から保存キーに使う digest を計算する。
// RefreshTokenGenerator が保存時に使う算出方法と一致していなければならない。
type RefreshTokenDigester interface {
	Digest(raw string) string
}

// RefreshTokenRepository は refresh token を永続化する。
type RefreshTokenRepository interface {
	Save(ctx context.Context, token authdomain.RefreshToken) error
}

// RefreshTokenFinder は digest をキーに保存済みの refresh token を取得する。
// 見つからない場合は ErrRefreshTokenNotFound を返す。
type RefreshTokenFinder interface {
	FindByDigest(ctx context.Context, digest string) (authdomain.RefreshToken, error)
}

// RefreshTokenRevoker は保存済みの refresh token を失効させる。
type RefreshTokenRevoker interface {
	Revoke(ctx context.Context, digest string, revokedAt time.Time) error
}

// RefreshTokenBulkRevoker は利用者の refresh token をまとめて失効させる。
// 全端末ログアウトやパスワード変更時に使う。
type RefreshTokenBulkRevoker interface {
	RevokeAllByUser(ctx context.Context, userID common.UserID, revokedAt time.Time) error
}

// LoginAttemptLimiter はログイン試行を識別子ごとに制限する。
// subject には IP アドレスや正規化済みメールアドレスを渡す。
type LoginAttemptLimiter interface {
	Allow(ctx context.Context, subject string) (bool, error)
}
