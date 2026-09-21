package domain

import (
	"errors"

	common "snap_kakeibo/backend/internal/common/domain"
)

var (
	// ErrMonthlySummaryInputMismatch は対象利用者または対象月と異なる支出が渡されたことを表す。
	ErrMonthlySummaryInputMismatch = errors.New("expense does not belong to monthly summary owner and month")
)

// CategoryTotals はカテゴリごとの明細金額を保持する。
type CategoryTotals map[Category]int64

// MonthlySummary は家計簿コンテキストの月次読み取りモデル。
type MonthlySummary struct {
	UserID              common.UserID
	YearMonth           YearMonth
	TotalRecordedAmount int64
	ExpenseCount        int64
	DetailCount         int64
	CategoryTotals      CategoryTotals
	Version             int64
}

// RebuildMonthlySummary は支出を正本として対象月の月次集計を再構築する。
func RebuildMonthlySummary(userID common.UserID, yearMonth YearMonth, expenses []Expense, version int64) (MonthlySummary, error) {
	summary := MonthlySummary{UserID: userID, YearMonth: yearMonth, CategoryTotals: make(CategoryTotals), Version: version}
	for _, category := range common.Categories() {
		summary.CategoryTotals[category] = 0
	}
	for _, expense := range expenses {
		if expense.userID != userID || expense.purchasedAt.YearMonth() != yearMonth {
			return MonthlySummary{}, ErrMonthlySummaryInputMismatch
		}
		summary.TotalRecordedAmount += expense.recordedAmount.Yen()
		summary.ExpenseCount++
		summary.DetailCount += int64(len(expense.details))
		for _, detail := range expense.details {
			summary.CategoryTotals[detail.category] += detail.amount.Yen()
		}
	}
	return summary, nil
}
