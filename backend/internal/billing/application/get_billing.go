// Package application は請求参照のユースケースを提供する。
package application

import (
	"context"
	"fmt"
	"strings"

	"snap_kakeibo/backend/internal/billing/domain"
	"snap_kakeibo/backend/internal/library/logger"
)

// GetBillingInput は請求参照ユースケースの入力。UserID は認証済みの利用者、BillingID はパスパラメータ。
type GetBillingInput struct {
	UserID    string
	BillingID string
}

// GetBillingOutput は請求とその明細。Details は明細が無ければ空のスライス。
type GetBillingOutput struct {
	Billing domain.Billing
	Details []domain.BillingDetail
}

// GetBillingUsecase は利用者の請求 1 件を明細付きで返す。
type GetBillingUsecase struct {
	Billings BillingFinder
	Details  BillingDetailLister
}

// GetBillingUsecaseInterface は HTTP 層が請求参照ユースケースへ依存するための契約。
type GetBillingUsecaseInterface interface {
	Get(ctx context.Context, in GetBillingInput) (GetBillingOutput, error)
}

var _ GetBillingUsecaseInterface = (*GetBillingUsecase)(nil)

// NewGetBillingUsecase は請求参照ユースケースを生成する。
func NewGetBillingUsecase(billings BillingFinder, details BillingDetailLister) GetBillingUsecaseInterface {
	return &GetBillingUsecase{Billings: billings, Details: details}
}

// Get は請求を引いてから明細を引く。請求が無ければ明細は引かずに ErrBillingNotFound を返す。
func (u *GetBillingUsecase) Get(ctx context.Context, in GetBillingInput) (GetBillingOutput, error) {
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.BillingID) == "" {
		return GetBillingOutput{}, ErrInvalidInput
	}
	// ここから先のログ(dynamodb_get_item / dynamodb_query span)に user_id / billing_id が付く
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.BillingID(in.BillingID))
	billing, err := u.Billings.FindByID(ctx, in.UserID, in.BillingID)
	if err != nil {
		return GetBillingOutput{}, err
	}
	details, err := u.Details.ListByBilling(ctx, in.UserID, in.BillingID)
	if err != nil {
		return GetBillingOutput{}, fmt.Errorf("list billing details: %w", err)
	}
	return GetBillingOutput{Billing: billing, Details: details}, nil
}
