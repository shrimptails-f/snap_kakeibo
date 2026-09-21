// Package application は家計簿コンテキストのユースケースを提供する。
package application

import (
	"context"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/library/logger"
)

// GetExpenseInput は支出取得ユースケースの入力。UserID は認証済みの利用者、ExpenseID はパスパラメータ。
type GetExpenseInput struct {
	UserID    string
	ExpenseID string
}

// GetExpenseOutput は支出明細を含む支出集約。
type GetExpenseOutput struct {
	Expense domain.Expense
}

// GetExpenseUsecase は利用者の支出 1 件を支出明細付きで返す。
type GetExpenseUsecase struct {
	Expenses ExpenseFinder
}

// GetExpenseUsecaseInterface は HTTP 層が支出取得ユースケースへ依存するための契約。
type GetExpenseUsecaseInterface interface {
	Get(ctx context.Context, in GetExpenseInput) (GetExpenseOutput, error)
}

var _ GetExpenseUsecaseInterface = (*GetExpenseUsecase)(nil)

// NewGetExpenseUsecase は支出取得ユースケースを生成する。
func NewGetExpenseUsecase(expenses ExpenseFinder) GetExpenseUsecaseInterface {
	return &GetExpenseUsecase{Expenses: expenses}
}

// Get は入力を識別子へ変換してから支出を引く。利用者または支出 ID が空なら ErrInvalidInput。
func (u *GetExpenseUsecase) Get(ctx context.Context, in GetExpenseInput) (GetExpenseOutput, error) {
	userID, ok := common.NewUserID(in.UserID)
	if !ok {
		return GetExpenseOutput{}, ErrInvalidInput
	}
	expenseID, ok := common.NewExpenseID(in.ExpenseID)
	if !ok {
		return GetExpenseOutput{}, ErrInvalidInput
	}
	// ここから先のログ(dynamodb_get_item / dynamodb_query span)に user_id / expense_id が付く
	ctx = logger.ContextWith(ctx, logger.UserID(userID.String()), logger.ExpenseID(expenseID.String()))
	expense, err := u.Expenses.FindByID(ctx, userID, expenseID)
	if err != nil {
		return GetExpenseOutput{}, err
	}
	return GetExpenseOutput{Expense: expense}, nil
}
