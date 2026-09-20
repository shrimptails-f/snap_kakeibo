package application

import (
	"context"
	"errors"
	"strings"

	common "snap_kakeibo/backend/internal/common/domain"
)

// CheckInput は認証状態確認ユースケースの入力。
type CheckInput struct {
	Authorization string
}

// CheckOutput は認証済み利用者として外部へ返せる情報。
type CheckOutput struct {
	UserID string
	Email  string
}

// CheckUsecase は Authorization ヘッダーの access token を検証する。
type CheckUsecase struct {
	AccessTokenVerifier AccessTokenVerifier
}

// CheckUsecaseInterface は HTTP 層が認証状態確認ユースケースへ依存するための契約。
type CheckUsecaseInterface interface {
	Check(ctx context.Context, in CheckInput) (CheckOutput, error)
}

var _ CheckUsecaseInterface = (*CheckUsecase)(nil)

// NewCheckUsecase は認証状態確認ユースケースを生成する。
func NewCheckUsecase(verifier AccessTokenVerifier) CheckUsecaseInterface {
	return &CheckUsecase{AccessTokenVerifier: verifier}
}

// Check は Bearer token を検証し、認証済み利用者を返す。
func (u CheckUsecase) Check(ctx context.Context, in CheckInput) (CheckOutput, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(in.Authorization, prefix) {
		return CheckOutput{}, ErrUnauthorized
	}
	raw := strings.TrimSpace(strings.TrimPrefix(in.Authorization, prefix))
	if raw == "" {
		return CheckOutput{}, ErrUnauthorized
	}
	user, err := u.AccessTokenVerifier.Verify(ctx, raw)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return CheckOutput{}, ErrUnauthorized
		}
		return CheckOutput{}, err
	}
	return checkOutput(user), nil
}

func checkOutput(user common.User) CheckOutput {
	return CheckOutput{UserID: user.ID.String(), Email: user.Email}
}
