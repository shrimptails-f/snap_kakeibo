package application

import (
	"context"
	"errors"
	"strings"

	"snap_kakeibo/backend/internal/library/timewrapper"
)

// LogoutInput は logout ユースケースの入力。
type LogoutInput struct {
	RefreshToken string
}

// LogoutUsecase は Cookie の refresh token を失効させる。
type LogoutUsecase struct {
	Digester            RefreshTokenDigester
	RefreshTokenRevoker RefreshTokenRevoker
	Clock               timewrapper.Interface
}

// LogoutUsecaseInterface は HTTP 層が logout ユースケースへ依存するための契約。
type LogoutUsecaseInterface interface {
	Logout(ctx context.Context, in LogoutInput) error
}

var _ LogoutUsecaseInterface = (*LogoutUsecase)(nil)

// NewLogoutUsecase は logout ユースケースを生成する。
func NewLogoutUsecase(digester RefreshTokenDigester, revoker RefreshTokenRevoker, clock timewrapper.Interface) LogoutUsecaseInterface {
	return &LogoutUsecase{Digester: digester, RefreshTokenRevoker: revoker, Clock: clock}
}

// Logout は refresh token を失効させる。Cookie がない場合や既に存在しない token に対しても成功する。
func (u LogoutUsecase) Logout(ctx context.Context, in LogoutInput) error {
	raw := strings.TrimSpace(in.RefreshToken)
	if raw == "" {
		return nil
	}
	err := u.RefreshTokenRevoker.Revoke(ctx, u.Digester.Digest(raw), u.Clock.Now().UTC())
	if errors.Is(err, ErrRefreshTokenNotFound) {
		return nil
	}
	return err
}
