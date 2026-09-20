package application

import "errors"

var (
	// ErrInvalidCredentials はメールアドレスまたはパスワードが不正な場合に返す。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnauthorized は refresh token が不正・失効・期限切れの場合に返す。
	// 原因を呼び出し側へ区別して返すと token の存在が推測できるため、まとめて扱う。
	ErrUnauthorized = errors.New("unauthorized")
	// ErrTooManyLoginAttempts はログイン試行が短時間の上限に達した場合に返す。
	ErrTooManyLoginAttempts = errors.New("too many login attempts")
	// ErrUserNotFound はリポジトリが該当ユーザーを見つけられなかった場合に返す。
	ErrUserNotFound = errors.New("user not found")
	// ErrRefreshTokenNotFound はリポジトリが該当 refresh token を見つけられなかった場合に返す。
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
)
