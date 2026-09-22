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

type imagePresigner struct {
	userID    common.UserID
	requestID common.AnalysisRequestID
	expires   time.Duration
	url       string
	err       error
}

func (p *imagePresigner) PresignGet(_ context.Context, userID common.UserID, requestID common.AnalysisRequestID, expires time.Duration) (string, error) {
	p.userID, p.requestID, p.expires = userID, requestID, expires
	return p.url, p.err
}

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
	images := &imagePresigner{url: "https://example.com/receipt.jpg"}
	out, err := application.NewGetExpenseUsecase(expenses, images).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if expenses.userID != "u1" || expenses.expenseID != "e1" {
		t.Errorf("FindByID args = %q / %q", expenses.userID, expenses.expenseID)
	}
	if out.Expense.ID() != "e1" || out.Expense.RecordedAmount().Yen() != 1200 || len(out.Expense.Details()) != 1 {
		t.Errorf("Get().Expense = %+v", out.Expense)
	}
	if out.ImageURL != images.url || images.userID != "u1" || images.requestID != "req1" || images.expires != 15*time.Minute {
		t.Errorf("image URL = %q, args = %q / %q / %s", out.ImageURL, images.userID, images.requestID, images.expires)
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
			if _, err := application.NewGetExpenseUsecase(expenses, &imagePresigner{}).Get(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
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
	if _, err := application.NewGetExpenseUsecase(expenses, &imagePresigner{}).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"}); !errors.Is(err, application.ErrExpenseNotFound) {
		t.Fatalf("Get() error = %v, want ErrExpenseNotFound", err)
	}
}

func TestGetPassesThroughRepositoryFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	if _, err := application.NewGetExpenseUsecase(&finder{err: cause}, &imagePresigner{}).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"}); !errors.Is(err, cause) {
		t.Fatalf("Get() error = %v, want %v", err, cause)
	}
}

func TestGetWrapsImageURLFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("signing failed")
	_, err := application.NewGetExpenseUsecase(&finder{expense: sampleExpense(t)}, &imagePresigner{err: cause}).Get(context.Background(), application.GetExpenseInput{UserID: "u1", ExpenseID: "e1"})
	if !errors.Is(err, cause) {
		t.Fatalf("Get() error = %v, want %v", err, cause)
	}
}
