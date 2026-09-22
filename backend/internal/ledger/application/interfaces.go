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

// ReceiptImageURLPresigner は支出の元になったレシート画像を直接取得する期限付き URL を発行する。
type ReceiptImageURLPresigner interface {
	PresignGet(ctx context.Context, userID common.UserID, requestID common.AnalysisRequestID, expires time.Duration) (string, error)
}

// ExpenseRepository は支出集約の取得・月別一覧・保存を提供する。
type ExpenseRepository interface {
	ExpenseFinder
	FindByMonth(ctx context.Context, userID common.UserID, month domain.YearMonth) ([]domain.Expense, error)
	Save(ctx context.Context, expense domain.Expense, updatedAt time.Time) error
}

// MonthlyExpenseItem は月別支出画面へ返す、支出明細に複製された読み取りモデル。
type MonthlyExpenseItem struct {
	DetailID     domain.ExpenseDetailID
	ExpenseID    domain.ExpenseID
	Name         string
	Category     common.Category
	Amount       int64
	Quantity     int64
	Source       domain.RecordSource
	IsEdited     bool
	StoreName    string
	PurchaseDate string
}

// MonthlyExpenseLister は指定月の支出明細を金額降順で返す。
type MonthlyExpenseLister interface {
	ListByMonth(ctx context.Context, userID common.UserID, month domain.YearMonth) ([]MonthlyExpenseItem, error)
}

// MonthlySummaryItem は保存済み月次集計の読み取りモデル。
type MonthlySummaryItem struct {
	Summary   domain.MonthlySummary
	UpdatedAt time.Time
}

// MonthlySummaryLister は利用者の月次集計を対象月降順で返す。
type MonthlySummaryLister interface {
	List(ctx context.Context, userID common.UserID) ([]MonthlySummaryItem, error)
}

// MonthlySummaryRepository は version 付きの月次集計を読み書きする。
type MonthlySummaryRepository interface {
	Version(ctx context.Context, userID common.UserID, month domain.YearMonth) (int64, error)
	Save(ctx context.Context, summary domain.MonthlySummary, updatedAt time.Time) error
}
