package domain

import (
	"errors"
	"math"
	"testing"

	common "snap_kakeibo/backend/internal/common/domain"
)

func TestRecordedAmountAllowsNegativeValue(t *testing.T) {
	t.Parallel()

	read, err := common.NewReadAmount(1_000)
	if err != nil {
		t.Fatalf("NewReadAmount() error = %v", err)
	}
	recorded, err := NewRecordedAmount(read, NewAdjustmentAmount(-1_200))
	if err != nil {
		t.Fatalf("NewRecordedAmount() error = %v", err)
	}
	if got := recorded.Yen(); got != -200 {
		t.Errorf("Yen() = %d, want -200", got)
	}
}

func TestRecordedAmountRejectsOverflow(t *testing.T) {
	t.Parallel()

	read, _ := common.NewReadAmount(1)
	if _, err := NewRecordedAmount(read, NewAdjustmentAmount(math.MaxInt64)); !errors.Is(err, ErrAmountOverflow) {
		t.Errorf("NewRecordedAmount() error = %v, want ErrAmountOverflow", err)
	}
}

func TestExpenseAdjustAmountCanMakeRecordedAmountNegative(t *testing.T) {
	t.Parallel()

	expense := newTestExpense(t, "expense-1", "2026-09-21", common.CategoryFood, 1_000)
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

	expense := newTestExpense(t, "expense-1", "2026-09-21", common.CategoryFood, 1_000)
	detailID := expense.Details()[0].ID()
	if err := expense.ChangeDetailCategory(detailID, common.CategorySocial); err != nil {
		t.Fatalf("ChangeDetailCategory() error = %v", err)
	}
	detail := expense.Details()[0]
	if detail.Category() != common.CategorySocial || detail.CategorySource() != CategorySourceUser {
		t.Errorf("detail category = %q, source = %q", detail.Category(), detail.CategorySource())
	}
}

func TestRebuildMonthlySummary(t *testing.T) {
	t.Parallel()

	expense1 := newTestExpense(t, "expense-1", "2026-09-21", common.CategoryFood, 1_000)
	expense2 := newTestExpense(t, "expense-2", "2026-09-22", common.CategorySocial, 2_000)
	if err := expense2.AdjustAmount(NewAdjustmentAmount(-500)); err != nil {
		t.Fatalf("AdjustAmount() error = %v", err)
	}
	userID, _ := common.NewUserID("user-1")
	month, _ := common.NewYearMonth("2026-09")
	summary, err := RebuildMonthlySummary(userID, month, []Expense{expense1, expense2}, 3)
	if err != nil {
		t.Fatalf("RebuildMonthlySummary() error = %v", err)
	}
	if summary.TotalRecordedAmount != 2_500 || summary.ExpenseCount != 2 || summary.DetailCount != 2 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.CategoryTotals[common.CategoryFood] != 1_000 || summary.CategoryTotals[common.CategorySocial] != 2_000 {
		t.Errorf("CategoryTotals = %+v", summary.CategoryTotals)
	}
}

func TestExpenseRejectsZeroQuantityOnChange(t *testing.T) {
	t.Parallel()

	expense := newTestExpense(t, "expense-1", "2026-09-21", common.CategoryFood, 1_000)
	detailID := expense.Details()[0].ID()
	amount, _ := common.NewDetailAmount(100)
	if err := expense.ChangeDetailAmount(detailID, amount, common.Quantity{}); !errors.Is(err, ErrInvalidExpenseDetail) {
		t.Errorf("ChangeDetailAmount() error = %v, want ErrInvalidExpenseDetail", err)
	}
}

func TestRestoreExpense(t *testing.T) {
	t.Parallel()

	expense := newTestExpense(t, "expense-1", "2026-09-21", common.CategoryFood, 1_000)
	restored, err := RestoreExpense(ExpenseState{
		ID: expense.ID(), UserID: expense.UserID(), SourceRequestID: expense.SourceRequestID(),
		StoreName: expense.StoreName(), PurchaseDate: expense.PurchaseDate(), ReadAmount: expense.ReadAmount(),
		Adjustment: NewAdjustmentAmount(-250), Details: expense.Details(), Edited: true,
	})
	if err != nil {
		t.Fatalf("RestoreExpense() error = %v", err)
	}
	if restored.RecordedAmount().Yen() != 750 || !restored.Edited() {
		t.Errorf("restored amount = %d, edited = %t", restored.RecordedAmount().Yen(), restored.Edited())
	}
}

func TestRebuildMonthlySummaryRejectsMismatchedMonth(t *testing.T) {
	t.Parallel()

	expense := newTestExpense(t, "expense-1", "2026-08-31", common.CategoryFood, 1_000)
	userID, _ := common.NewUserID("user-1")
	month, _ := common.NewYearMonth("2026-09")
	if _, err := RebuildMonthlySummary(userID, month, []Expense{expense}, 0); !errors.Is(err, ErrMonthlySummaryInputMismatch) {
		t.Errorf("RebuildMonthlySummary() error = %v, want ErrMonthlySummaryInputMismatch", err)
	}
}

func newTestExpense(t *testing.T, expenseValue, dateValue string, category common.Category, yen int64) Expense {
	t.Helper()
	expenseID, _ := common.NewExpenseID(expenseValue)
	userID, _ := common.NewUserID("user-1")
	requestID, _ := common.NewAnalysisRequestID("request-1")
	detailID, _ := NewExpenseDetailID(expenseValue + "-detail")
	date, _ := common.NewPurchaseDate(dateValue)
	readAmount, _ := common.NewReadAmount(yen)
	detailAmount, _ := common.NewDetailAmount(yen)
	quantity, _ := common.NewQuantity(1)
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
