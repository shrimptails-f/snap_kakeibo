package application

import (
	"context"
	"errors"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

// ErrUnauthorized は refresh token が不正・失効・期限切れの場合に返す。
// 原因を呼び出し側へ区別して返すと token の存在が推測できるため、まとめて扱う。
var ErrUnauthorized = errors.New("unauthorized")

// RefreshInput は refresh ユースケースの入力。
type RefreshInput struct {
	RefreshToken string
}

// RefreshOutput は refresh ユースケースの出力。
type RefreshOutput struct {
	Tokens Tokens
	User   common.User
}

// RefreshUsecase は refresh token を検証し、新しい access token と refresh token へ差し替える。
// 使用済みの refresh token は失効させ、再利用できないようにする（rotation）。
type RefreshUsecase struct {
	Users                  UserRepository
	AccessTokens           AccessTokenIssuer
	Digester               RefreshTokenDigester
	RefreshTokenFinder     RefreshTokenFinder
	RefreshTokenRevoker    RefreshTokenRevoker
	RefreshTokens          RefreshTokenGenerator
	RefreshTokenRepository RefreshTokenRepository
	Clock                  timewrapper.Interface
	RefreshTokenTTL        time.Duration
}

// RefreshUsecaseInterface は HTTP 層が refresh ユースケースへ依存するための契約。
type RefreshUsecaseInterface interface {
	Refresh(ctx context.Context, in RefreshInput) (RefreshOutput, error)
}

var _ RefreshUsecaseInterface = (*RefreshUsecase)(nil)

// NewRefreshUsecase は refresh ユースケースを生成する。
func NewRefreshUsecase(users UserRepository, accessTokens AccessTokenIssuer, digester RefreshTokenDigester, finder RefreshTokenFinder, revoker RefreshTokenRevoker, refreshTokens RefreshTokenGenerator, refreshTokenRepository RefreshTokenRepository, clock timewrapper.Interface) RefreshUsecaseInterface {
	return &RefreshUsecase{
		Users:                  users,
		AccessTokens:           accessTokens,
		Digester:               digester,
		RefreshTokenFinder:     finder,
		RefreshTokenRevoker:    revoker,
		RefreshTokens:          refreshTokens,
		RefreshTokenRepository: refreshTokenRepository,
		Clock:                  clock,
		RefreshTokenTTL:        30 * 24 * time.Hour,
	}
}

// Refresh は有効な refresh token と引き換えに新しいトークン群を発行する。
func (u RefreshUsecase) Refresh(ctx context.Context, in RefreshInput) (RefreshOutput, error) {
	raw := strings.TrimSpace(in.RefreshToken)
	if raw == "" {
		return RefreshOutput{}, ErrUnauthorized
	}
	digest := u.Digester.Digest(raw)
	stored, err := u.RefreshTokenFinder.FindByDigest(ctx, digest)
	if errors.Is(err, ErrRefreshTokenNotFound) {
		return RefreshOutput{}, ErrUnauthorized
	}
	if err != nil {
		return RefreshOutput{}, err
	}
	now := u.Clock.Now().UTC()
	if !stored.Usable(now) {
		return RefreshOutput{}, ErrUnauthorized
	}
	user, err := u.Users.FindByEmail(ctx, stored.Email)
	if errors.Is(err, ErrUserNotFound) {
		return RefreshOutput{}, ErrUnauthorized
	}
	if err != nil {
		return RefreshOutput{}, err
	}
	if user.ID != stored.UserID {
		return RefreshOutput{}, ErrUnauthorized
	}
	// 新トークンを発行する前に旧トークンを失効させ、失効に失敗した場合は旧トークンを生かしたまま新トークンを配らない。
	if err := u.RefreshTokenRevoker.Revoke(ctx, digest, now); err != nil {
		return RefreshOutput{}, err
	}
	accessToken, expiresIn, err := u.AccessTokens.Issue(ctx, user)
	if err != nil {
		return RefreshOutput{}, err
	}
	rawRefreshToken, refreshToken, err := u.RefreshTokens.Generate(user, now, now.Add(u.RefreshTokenTTL))
	if err != nil {
		return RefreshOutput{}, err
	}
	if err := u.RefreshTokenRepository.Save(ctx, refreshToken); err != nil {
		return RefreshOutput{}, err
	}
	return RefreshOutput{Tokens: Tokens{
		AccessToken:           accessToken,
		ExpiresIn:             expiresIn,
		RefreshToken:          rawRefreshToken,
		RefreshTokenExpiresIn: int64(u.RefreshTokenTTL / time.Second),
	}, User: user}, nil
}
