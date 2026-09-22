package application

import (
	"context"
	"fmt"
	"strings"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/library/logger"
)

type ListMonthExpensesInput struct {
	UserID    string
	YearMonth string
}

type ListMonthExpensesOutput struct {
	YearMonth string
	Items     []MonthlyExpenseItem
}

type ListMonthExpensesUsecaseInterface interface {
	List(ctx context.Context, in ListMonthExpensesInput) (ListMonthExpensesOutput, error)
}

type ListMonthExpensesUsecase struct{ Expenses MonthlyExpenseLister }

func NewListMonthExpensesUsecase(expenses MonthlyExpenseLister) ListMonthExpensesUsecaseInterface {
	return &ListMonthExpensesUsecase{Expenses: expenses}
}

func (u *ListMonthExpensesUsecase) List(ctx context.Context, in ListMonthExpensesInput) (ListMonthExpensesOutput, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return ListMonthExpensesOutput{}, ErrInvalidInput
	}
	month, err := common.NewYearMonth(in.YearMonth)
	if err != nil {
		return ListMonthExpensesOutput{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	userID, ok := common.NewUserID(in.UserID)
	if !ok {
		return ListMonthExpensesOutput{}, ErrInvalidInput
	}
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.String("year_month", month.String()))
	items, err := u.Expenses.ListByMonth(ctx, userID, domain.YearMonth(month))
	if err != nil {
		return ListMonthExpensesOutput{}, fmt.Errorf("list monthly expenses: %w", err)
	}
	return ListMonthExpensesOutput{YearMonth: month.String(), Items: items}, nil
}
