package domain

import "testing"

func TestExpenseAdjustAmountCanMakeRecordedAmountNegative(t *testing.T) {
	t.Parallel()

	expense := newTestExpense(t, "expense-1", "2026-09-21", CategoryFood, 1_000)
	if err := expense.AdjustAmount(NewAdjustmentAmount(-1_200)); err != nil {
		t.Fatalf("AdjustAmount() error = %v", err)
	}
	if got := expense.RecordedAmount().Yen(); got != -200 {
		t.Errorf("RecordedAmount() = %d, want -200", got)
	}
	if !expense.Edited() {
		t.Error("Edited() = false, want true")
	}
}

func TestExpenseChangeDetailCategoryMarksUserSource(t *testing.T) {
	t.Parallel()

	expense := newTestExpense(t, "expense-1", "2026-09-21", CategoryFood, 1_000)
	detailID := expense.Details()[0].ID()
	if err := expense.ChangeDetailCategory(detailID, CategorySocial); err != nil {
		t.Fatalf("ChangeDetailCategory() error = %v", err)
	}
	detail := expense.Details()[0]
	if detail.Category() != CategorySocial || detail.CategorySource() != CategorySourceUser {
		t.Errorf("detail category = %q, source = %q", detail.Category(), detail.CategorySource())
	}
}

func TestRebuildMonthlySummary(t *testing.T) {
	t.Parallel()

	expense1 := newTestExpense(t, "expense-1", "2026-09-21", CategoryFood, 1_000)
	expense2 := newTestExpense(t, "expense-2", "2026-09-22", CategorySocial, 2_000)
	if err := expense2.AdjustAmount(NewAdjustmentAmount(-500)); err != nil {
		t.Fatalf("AdjustAmount() error = %v", err)
	}
	userID, _ := NewUserID("user-1")
	month, _ := NewYearMonth("2026-09")
	summary := RebuildMonthlySummary(userID, month, []Expense{expense1, expense2}, 3)
	if summary.TotalRecordedAmount != 2_500 || summary.ExpenseCount != 2 || summary.DetailCount != 2 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.CategoryTotals[CategoryFood] != 1_000 || summary.CategoryTotals[CategorySocial] != 2_000 {
		t.Errorf("CategoryTotals = %+v", summary.CategoryTotals)
	}
}

func newTestExpense(t *testing.T, expenseValue, dateValue string, category Category, yen int64) Expense {
	t.Helper()
	expenseID, _ := NewExpenseID(expenseValue)
	userID, _ := NewUserID("user-1")
	requestID, _ := NewAnalysisRequestID("request-1")
	detailID, _ := NewExpenseDetailID(expenseValue + "-detail")
	date, _ := NewPurchaseDate(dateValue)
	readAmount, _ := NewReadAmount(yen)
	detailAmount, _ := NewDetailAmount(yen)
	quantity, _ := NewQuantity(1)
	detail, err := NewExpenseDetail(detailID, "品物", detailAmount, quantity, category, CategorySourceAI)
	if err != nil {
		t.Fatalf("NewExpenseDetail() error = %v", err)
	}
	expense, err := NewExpense(expenseID, userID, requestID, "店", date, readAmount, []ExpenseDetail{detail})
	if err != nil {
		t.Fatalf("NewExpense() error = %v", err)
	}
	return expense
}
