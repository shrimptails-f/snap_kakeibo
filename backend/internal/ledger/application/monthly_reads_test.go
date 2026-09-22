package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
)

type monthlyExpenseLister struct {
	userID common.UserID
	month  domain.YearMonth
	items  []application.MonthlyExpenseItem
	err    error
}

func (l *monthlyExpenseLister) ListByMonth(_ context.Context, userID common.UserID, month domain.YearMonth) ([]application.MonthlyExpenseItem, error) {
	l.userID, l.month = userID, month
	return l.items, l.err
}

type monthlySummaryLister struct {
	userID    common.UserID
	summaries []application.MonthlySummaryItem
	err       error
}

func (l *monthlySummaryLister) List(_ context.Context, userID common.UserID) ([]application.MonthlySummaryItem, error) {
	l.userID = userID
	return l.summaries, l.err
}

func TestListMonthExpensesValidatesAndReturnsRepositoryOrder(t *testing.T) {
	t.Parallel()
	detailID, _ := domain.NewExpenseDetailID("d1")
	expenseID, _ := common.NewExpenseID("e1")
	lister := &monthlyExpenseLister{items: []application.MonthlyExpenseItem{{DetailID: detailID, ExpenseID: expenseID, Amount: 500}}}
	out, err := application.NewListMonthExpensesUsecase(lister).List(context.Background(), application.ListMonthExpensesInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if lister.userID != "u1" || lister.month.String() != "2026-09" || out.YearMonth != "2026-09" || len(out.Items) != 1 || out.Items[0].Amount != 500 {
		t.Errorf("List() args/output = %q %q %+v", lister.userID, lister.month, out)
	}
}

func TestListMonthExpensesRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]application.ListMonthExpensesInput{"user": {YearMonth: "2026-09"}, "month format": {UserID: "u1", YearMonth: "2026-9"}, "month value": {UserID: "u1", YearMonth: "2026-13"}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := application.NewListMonthExpensesUsecase(&monthlyExpenseLister{}).List(context.Background(), input); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("List() error = %v", err)
			}
		})
	}
}

func TestGetMonthlySummariesReturnsRepositoryResult(t *testing.T) {
	t.Parallel()
	userID, _ := common.NewUserID("u1")
	month, _ := common.NewYearMonth("2026-09")
	want := application.MonthlySummaryItem{Summary: domain.MonthlySummary{UserID: userID, YearMonth: month, TotalRecordedAmount: 12000}, UpdatedAt: time.Now()}
	lister := &monthlySummaryLister{summaries: []application.MonthlySummaryItem{want}}
	out, err := application.NewGetMonthlySummariesUsecase(lister).Get(context.Background(), application.GetMonthlySummariesInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if lister.userID != "u1" || len(out.Summaries) != 1 || out.Summaries[0].Summary.TotalRecordedAmount != 12000 {
		t.Errorf("Get() = %+v", out)
	}
}

func TestGetMonthlySummariesRejectsMissingUser(t *testing.T) {
	t.Parallel()
	if _, err := application.NewGetMonthlySummariesUsecase(&monthlySummaryLister{}).Get(context.Background(), application.GetMonthlySummariesInput{}); !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("Get() error = %v", err)
	}
}
