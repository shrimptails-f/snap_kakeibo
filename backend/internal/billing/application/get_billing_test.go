package application_test

import (
	"context"
	"errors"
	"testing"

	"snap_kakeibo/backend/internal/billing/application"
	"snap_kakeibo/backend/internal/billing/domain"
)

type finder struct {
	userID, billingID string
	billing           domain.Billing
	err               error
}

func (f *finder) FindByID(_ context.Context, userID, billingID string) (domain.Billing, error) {
	f.userID, f.billingID = userID, billingID
	if f.err != nil {
		return domain.Billing{}, f.err
	}
	return f.billing, nil
}

type detailLister struct {
	called  bool
	details []domain.BillingDetail
	err     error
}

func (l *detailLister) ListByBilling(_ context.Context, _, _ string) ([]domain.BillingDetail, error) {
	l.called = true
	if l.err != nil {
		return nil, l.err
	}
	return l.details, nil
}

// fixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type fixture struct {
	billings *finder
	details  *detailLister
}

func newFixture() *fixture {
	return &fixture{
		billings: &finder{billing: domain.Billing{ID: "b1", UserID: "u1", UploadID: "up1", StoreName: "store", PurchasedAt: "2026-09-18", YearMonth: "2026-09", OriginalAmount: 1200, FinalAmount: 1200}},
		details: &detailLister{details: []domain.BillingDetail{
			{ID: "d1", Name: "milk", Category: "food", CategorySource: "AI", Amount: 200, Quantity: 1},
			{ID: "d2", Name: "bread", Category: "food", CategorySource: "AI", Amount: 1000, Quantity: 2},
		}},
	}
}

func (f *fixture) build() application.GetBillingUsecaseInterface {
	return application.NewGetBillingUsecase(f.billings, f.details)
}

func TestGetReturnsBillingWithDetails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	out, err := f.build().Get(context.Background(), application.GetBillingInput{UserID: "u1", BillingID: "b1"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if f.billings.userID != "u1" || f.billings.billingID != "b1" {
		t.Errorf("FindByID args = %q / %q", f.billings.userID, f.billings.billingID)
	}
	if out.Billing != f.billings.billing {
		t.Errorf("Get().Billing = %+v, want %+v", out.Billing, f.billings.billing)
	}
	if len(out.Details) != 2 || out.Details[0] != f.details.details[0] || out.Details[1] != f.details.details[1] {
		t.Errorf("Get().Details = %+v, want %+v", out.Details, f.details.details)
	}
}

func TestGetRejectsMissingIDs(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.GetBillingInput{
		"missing user":    {BillingID: "b1"},
		"missing billing": {UserID: "u1", BillingID: " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			if _, err := f.build().Get(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("Get() error = %v, want ErrInvalidInput", err)
			}
			if f.billings.userID != "" || f.details.called {
				t.Errorf("no lookups expected: %+v %+v", f.billings, f.details)
			}
		})
	}
}

func TestGetDoesNotListDetailsWhenBillingIsMissing(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.billings.err = application.ErrBillingNotFound
	_, err := f.build().Get(context.Background(), application.GetBillingInput{UserID: "u1", BillingID: "b1"})
	if !errors.Is(err, application.ErrBillingNotFound) {
		t.Fatalf("Get() error = %v, want ErrBillingNotFound", err)
	}
	if f.details.called {
		t.Error("ListByBilling should not be called when the billing is missing")
	}
}

func TestGetDoesNotListDetailsWhenFindFails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.billings.err = errors.New("boom")
	if _, err := f.build().Get(context.Background(), application.GetBillingInput{UserID: "u1", BillingID: "b1"}); err == nil || f.details.called {
		t.Fatalf("Get() error = %v, details called = %v", err, f.details.called)
	}
}

func TestGetWrapsDetailFailure(t *testing.T) {
	t.Parallel()
	f := newFixture()
	cause := errors.New("boom")
	f.details.err = cause
	if _, err := f.build().Get(context.Background(), application.GetBillingInput{UserID: "u1", BillingID: "b1"}); !errors.Is(err, cause) {
		t.Fatalf("Get() error = %v, want wrapped %v", err, cause)
	}
}
