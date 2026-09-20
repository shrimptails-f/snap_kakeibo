// Package application は認証のユースケースを提供する。
package application

import (
	"context"
	"errors"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

var (
	// ErrInvalidCredentials はメールアドレスまたはパスワードが不正な場合に返す。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserNotFound はリポジトリが該当ユーザーを見つけられなかった場合に返す。
	ErrUserNotFound = errors.New("user not found")
)

// LoginInput はログインユースケースの入力。
type LoginInput struct {
	Email    string
	Password string
}

// Tokens はログイン成功時にクライアントへ返すトークン群。
type Tokens struct {
	AccessToken           string
	ExpiresIn             int64
	RefreshToken          string
	RefreshTokenExpiresIn int64
}

// LoginOutput はログインユースケースの出力。
type LoginOutput struct {
	Tokens Tokens
	User   common.User
}

// LoginUsecase はメールアドレスとパスワードによるログインを実行する。
type LoginUsecase struct {
	Users                  UserRepository
	Passwords              PasswordComparator
	AccessTokens           AccessTokenIssuer
	RefreshTokens          RefreshTokenGenerator
	RefreshTokenRepository RefreshTokenRepository
	Clock                  timewrapper.Interface
	RefreshTokenTTL        time.Duration
}

// LoginUsecaseInterface は HTTP 層がログインユースケースへ依存するための契約。
type LoginUsecaseInterface interface {
	Login(ctx context.Context, in LoginInput) (LoginOutput, error)
}

var _ LoginUsecaseInterface = (*LoginUsecase)(nil)

// NewLoginUsecase はログインユースケースを生成する。
func NewLoginUsecase(users UserRepository, passwords PasswordComparator, accessTokens AccessTokenIssuer, refreshTokens RefreshTokenGenerator, refreshTokenRepository RefreshTokenRepository, clock timewrapper.Interface) LoginUsecaseInterface {
	return &LoginUsecase{
		Users:                  users,
		Passwords:              passwords,
		AccessTokens:           accessTokens,
		RefreshTokens:          refreshTokens,
		RefreshTokenRepository: refreshTokenRepository,
		Clock:                  clock,
		RefreshTokenTTL:        30 * 24 * time.Hour,
	}
}

// Login は認証成功後に access token と refresh token を発行する。
func (u LoginUsecase) Login(ctx context.Context, in LoginInput) (LoginOutput, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return LoginOutput{}, ErrInvalidCredentials
	}
	user, err := u.Users.FindByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		return LoginOutput{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginOutput{}, err
	}
	if user.ID == "" || user.PasswordHash == "" || u.Passwords.Compare(user.PasswordHash, in.Password) != nil {
		return LoginOutput{}, ErrInvalidCredentials
	}
	accessToken, expiresIn, err := u.AccessTokens.Issue(ctx, user)
	if err != nil {
		return LoginOutput{}, err
	}
	now := u.Clock.Now().UTC()
	rawRefreshToken, refreshToken, err := u.RefreshTokens.Generate(user, now, now.Add(u.RefreshTokenTTL))
	if err != nil {
		return LoginOutput{}, err
	}
	if err := u.RefreshTokenRepository.Save(ctx, refreshToken); err != nil {
		return LoginOutput{}, err
	}
	return LoginOutput{Tokens: Tokens{
		AccessToken:           accessToken,
		ExpiresIn:             expiresIn,
		RefreshToken:          rawRefreshToken,
		RefreshTokenExpiresIn: int64(u.RefreshTokenTTL / time.Second),
	}, User: user}, nil
}
