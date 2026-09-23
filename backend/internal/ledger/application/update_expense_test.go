package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/library/ulid"
)

type expenseRepository struct {
	expense       domain.Expense
	saved         domain.Expense
	savedAt       time.Time
	monthExpenses []domain.Expense
}

func (r *expenseRepository) FindByID(context.Context, common.UserID, domain.ExpenseID) (domain.Expense, error) {
	return r.expense, nil
}
func (r *expenseRepository) FindByMonth(context.Context, common.UserID, domain.YearMonth) ([]domain.Expense, error) {
	return r.monthExpenses, nil
}
func (r *expenseRepository) Save(_ context.Context, expense domain.Expense, _ []domain.ExpenseDetail, at time.Time) error {
	r.saved, r.savedAt = expense, at
	return nil
}

type rebuildRecorder struct{ months []string }

func (r *rebuildRecorder) Rebuild(_ context.Context, in application.RebuildMonthlySummaryInput) (application.RebuildMonthlySummaryOutput, error) {
	r.months = append(r.months, in.YearMonth)
	return application.RebuildMonthlySummaryOutput{}, nil
}

func TestUpdateExpenseChangesEditableFieldsAndRebuildsBothMonths(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 22, 1, 2, 3, 0, time.UTC)
	repository := &expenseRepository{expense: sampleExpense(t)}
	rebuild := &rebuildRecorder{}
	usecase := application.NewUpdateExpenseUsecase(repository, rebuild, timewrapper.NewFixed(now), ulid.New(timewrapper.NewFixed(now)))
	out, err := usecase.Update(context.Background(), application.UpdateExpenseInput{
		UserID: "u1", ExpenseID: "e1", StoreName: "new store", PurchaseDate: "2026-10-01", AdjustmentAmount: -500,
		Details: []application.UpdateExpenseDetailInput{{DetailID: "d1", Name: "oat milk", Amount: 300, Quantity: 2, Category: "daily_goods"}},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.saved.StoreName() != "new store" || repository.saved.RecordedAmount().Yen() != 700 || !repository.saved.Edited() {
		t.Errorf("saved expense = %+v", repository.saved)
	}
	detail := repository.saved.Details()[0]
	if detail.Name() != "oat milk" || detail.Amount().Yen() != 300 || detail.Quantity().Int64() != 2 || detail.Category() != common.CategoryDailyGoods || detail.CategorySource() != domain.CategorySourceUser {
		t.Errorf("saved detail = %+v", detail)
	}
	if len(rebuild.months) != 2 || rebuild.months[0] != "2026-09" || rebuild.months[1] != "2026-10" {
		t.Errorf("rebuilt months = %v", rebuild.months)
	}
	if out.UpdatedAt != now || repository.savedAt != now {
		t.Errorf("updated times = %v / %v", out.UpdatedAt, repository.savedAt)
	}
}

func TestUpdateExpenseRejectsInvalidDetails(t *testing.T) {
	t.Parallel()
	for name, details := range map[string][]application.UpdateExpenseDetailInput{"empty": {}, "unknown ID": {{DetailID: "d2", Name: "x", Amount: 1, Quantity: 1, Category: "food"}}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repository := &expenseRepository{expense: sampleExpense(t)}
			_, err := application.NewUpdateExpenseUsecase(repository, &rebuildRecorder{}, timewrapper.NewFixed(time.Time{}), ulid.New(nil)).Update(context.Background(), application.UpdateExpenseInput{UserID: "u1", ExpenseID: "e1", StoreName: "store", PurchaseDate: "2026-09-18", Details: details})
			if !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestUpdateExpenseReplacesDetailAndRebuildsSummary(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 22, 1, 2, 3, 0, time.UTC)
	repository := &expenseRepository{expense: sampleExpense(t)}
	rebuild := &rebuildRecorder{}
	_, err := application.NewUpdateExpenseUsecase(repository, rebuild, timewrapper.NewFixed(now), ulid.New(timewrapper.NewFixed(now))).Update(context.Background(), application.UpdateExpenseInput{
		UserID: "u1", ExpenseID: "e1", StoreName: "store", PurchaseDate: "2026-09-18",
		Details: []application.UpdateExpenseDetailInput{{Name: "新しい品", Amount: 250, Quantity: 1, Category: "food"}},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	details := repository.saved.Details()
	if len(details) != 1 || details[0].ID() == "d1" || details[0].Name() != "新しい品" || details[0].Source() != domain.RecordSourceUser || details[0].CategorySource() != domain.CategorySourceUser {
		t.Errorf("saved details = %+v", details)
	}
	if len(rebuild.months) != 1 || rebuild.months[0] != "2026-09" {
		t.Errorf("rebuilt months = %v", rebuild.months)
	}
}

func TestUpdateExpenseConfirmsAndClearsDetailTax(t *testing.T) {
	t.Parallel()
	confirmed := true
	cleared := false
	base := application.UpdateExpenseInput{UserID: "u1", ExpenseID: "e1", StoreName: "store", PurchaseDate: "2026-09-18"}
	repository := &expenseRepository{expense: sampleExpense(t)}
	rebuild := &rebuildRecorder{}
	usecase := application.NewUpdateExpenseUsecase(repository, rebuild, timewrapper.NewFixed(time.Time{}), ulid.New(nil))
	detail := repository.expense.Details()[0]
	base.Details = []application.UpdateExpenseDetailInput{{DetailID: detail.ID().String(), Name: detail.Name(), Amount: detail.Amount().Yen(), Quantity: detail.Quantity().Int64(), Category: detail.Category().String(), TaxConfirmed: &confirmed, TaxRate: 8, TaxMode: "external", TaxIncludedAmount: detail.Amount().Yen() + 1}}
	if _, err := usecase.Update(context.Background(), base); err != nil {
		t.Fatalf("confirm tax: %v", err)
	}
	got := repository.saved.Details()[0]
	if got.TaxIncludedAmount() == nil || *got.TaxIncludedAmount() != base.Details[0].TaxIncludedAmount || got.TaxRate() == nil || *got.TaxRate() != 8 || got.TaxMode() != "external" || got.TaxAllocation() != "user_confirmed" || !got.Edited() {
		t.Fatalf("confirmed detail = %+v", got)
	}
	if len(rebuild.months) != 1 || rebuild.months[0] != "2026-09" {
		t.Fatalf("rebuild months = %v", rebuild.months)
	}
	repository.expense = repository.saved
	base.Details[0].TaxConfirmed = &cleared
	if _, err := usecase.Update(context.Background(), base); err != nil {
		t.Fatalf("clear tax: %v", err)
	}
	got = repository.saved.Details()[0]
	if got.TaxIncludedAmount() != nil || got.TaxRate() != nil || got.TaxMode() != "unknown" {
		t.Fatalf("cleared detail = %+v", got)
	}
}

func TestUpdateExpenseRejectsInconsistentManualTax(t *testing.T) {
	t.Parallel()
	confirmed := true
	repository := &expenseRepository{expense: sampleExpense(t)}
	detail := repository.expense.Details()[0]
	input := application.UpdateExpenseInput{UserID: "u1", ExpenseID: "e1", StoreName: "store", PurchaseDate: "2026-09-18", Details: []application.UpdateExpenseDetailInput{{DetailID: detail.ID().String(), Name: detail.Name(), Amount: detail.Amount().Yen(), Quantity: detail.Quantity().Int64(), Category: detail.Category().String(), TaxConfirmed: &confirmed, TaxRate: 8, TaxMode: "included", TaxIncludedAmount: detail.Amount().Yen() + 1}}}
	_, err := application.NewUpdateExpenseUsecase(repository, &rebuildRecorder{}, timewrapper.NewFixed(time.Time{}), ulid.New(nil)).Update(context.Background(), input)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
	}
}

type summaryRepository struct {
	versions  []int64
	saves     int
	failFirst bool
}

func (r *summaryRepository) Version(context.Context, common.UserID, domain.YearMonth) (int64, error) {
	v := int64(r.saves)
	r.versions = append(r.versions, v)
	return v, nil
}
func (r *summaryRepository) Save(context.Context, domain.MonthlySummary, time.Time) error {
	r.saves++
	if r.failFirst && r.saves == 1 {
		return application.ErrConcurrentUpdate
	}
	return nil
}

func TestRebuildMonthlySummaryRetriesVersionConflict(t *testing.T) {
	t.Parallel()
	expense := sampleExpense(t)
	repository := &expenseRepository{monthExpenses: []domain.Expense{expense}}
	summaries := &summaryRepository{failFirst: true}
	out, err := application.NewRebuildMonthlySummaryUsecase(repository, summaries, timewrapper.NewFixed(time.Time{})).Rebuild(context.Background(), application.RebuildMonthlySummaryInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summaries.saves != 2 || out.Summary.Version != 2 || out.Summary.TotalRecordedAmount != 1200 {
		t.Errorf("saves/output = %d / %+v", summaries.saves, out.Summary)
	}
}
