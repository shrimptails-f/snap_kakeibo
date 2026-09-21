package application

import (
	"context"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/domain"
)

// ExpenseFinder は利用者の支出を支出明細ごと集約として引く。
type ExpenseFinder interface {
	// FindByID は支出明細を含む支出集約を返す。利用者の支出に無ければ ErrExpenseNotFound。
	FindByID(ctx context.Context, userID common.UserID, expenseID domain.ExpenseID) (domain.Expense, error)
}
