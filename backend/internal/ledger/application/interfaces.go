package application

import (
	"context"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/domain"
)

// ExpenseFinder は利用者の支出を支出明細ごと集約として引く。
type ExpenseFinder interface {
	// FindByID は支出明細を含む支出集約を返す。利用者の支出に無ければ ErrExpenseNotFound。
	FindByID(ctx context.Context, userID common.UserID, expenseID domain.ExpenseID) (domain.Expense, error)
}

// ExpenseRepository は支出集約の取得・月別一覧・保存を提供する。
type ExpenseRepository interface {
	ExpenseFinder
	FindByMonth(ctx context.Context, userID common.UserID, month domain.YearMonth) ([]domain.Expense, error)
	Save(ctx context.Context, expense domain.Expense, updatedAt time.Time) error
}

// MonthlySummaryRepository は version 付きの月次集計を読み書きする。
type MonthlySummaryRepository interface {
	Version(ctx context.Context, userID common.UserID, month domain.YearMonth) (int64, error)
	Save(ctx context.Context, summary domain.MonthlySummary, updatedAt time.Time) error
}
