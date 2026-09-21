package application_test

import (
	"context"
	"errors"
	"testing"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
)

type finder struct {
	userID    common.UserID
	expenseID domain.ExpenseID
	called    bool
	expense   domain.Expense
	err       error
}

func (f *finder) FindByID(_ context.Context, userID common.UserID, expenseID domain.ExpenseID) (domain.Expense, error) {
	f.called, f.userID, f.expenseID = true, userID, expenseID
	if f.err != nil {
		return domain.Expense{}, f.err
	}
	return f.expense, nil
}

func sampleExpense(t *testing.T) domain.Expense {
	t.Helper()
	expenseID, _ := common.NewExpenseID("e1")
	userID, _ := common.NewUserID("u1")
	requestID, _ := common.NewAnalysisRequestID("req1")
	date, _ := common.NewPurchaseDate("2026-09-18")
	readAmount, _ := common.NewReadAmount(1200)
	amount, _ := common.NewDetailAmount(200)
	quantity, _ := common.NewQuantity(1)
	detailID, _ := domain.NewExpenseDetailID("d1")
	detail, err := domain.NewExpenseDetail(detailID, "milk", amount, quantity, common.CategoryFood, domain.CategorySourceAI)
	if err != nil {
		t.Fatalf("NewExpenseDetail() error = %v", err)
	}
	expense, err := domain.NewExpense(expenseID, userID, requestID, "store", date, readAmount, []domain.ExpenseDetail{detail})
	if err != nil {
		t.Fatalf("NewExpense() error = %v", err)
	}
	return expense
}

func TestGetReturnsExpenseWithDetails(t *testing.T) {
	t.Parallel()
	expenses := &finder{expense: sampleExpense(t)}
	out, err := application.NewGetExpenseUsecase(expenses).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if expenses.userID != "u1" || expenses.expenseID != "e1" {
		t.Errorf("FindByID args = %q / %q", expenses.userID, expenses.expenseID)
	}
	if out.Expense.ID() != "e1" || out.Expense.RecordedAmount().Yen() != 1200 || len(out.Expense.Details()) != 1 {
		t.Errorf("Get().Expense = %+v", out.Expense)
	}
}

func TestGetRejectsMissingIDs(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.GetExpenseInput{
		"missing user":    {ExpenseID: "e1"},
		"missing expense": {UserID: "u1", ExpenseID: " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			expenses := &finder{}
			if _, err := application.NewGetExpenseUsecase(expenses).Get(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("Get() error = %v, want ErrInvalidInput", err)
			}
			if expenses.called {
				t.Errorf("no lookup expected for %+v", in)
			}
		})
	}
}

func TestGetPassesThroughNotFound(t *testing.T) {
	t.Parallel()
	expenses := &finder{err: application.ErrExpenseNotFound}
	if _, err := application.NewGetExpenseUsecase(expenses).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"}); !errors.Is(err, application.ErrExpenseNotFound) {
		t.Fatalf("Get() error = %v, want ErrExpenseNotFound", err)
	}
}

func TestGetPassesThroughRepositoryFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	if _, err := application.NewGetExpenseUsecase(&finder{err: cause}).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"}); !errors.Is(err, cause) {
		t.Fatalf("Get() error = %v, want %v", err, cause)
	}
}
