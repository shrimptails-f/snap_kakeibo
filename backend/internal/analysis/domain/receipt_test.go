package domain

import (
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestValidate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.FixedZone("JST", 9*3600))
	tests := []struct {
		name string
		r    Receipt
		code string
	}{
		{"valid", Receipt{PurchasedAt: ptr("2026-09-19"), TotalAmount: ptr(int64(1)), Details: []Detail{{Name: "品物", Amount: 0, Quantity: 1, Category: "food"}}}, ""},
		{"no date", Receipt{TotalAmount: ptr(int64(1))}, FailureNoDate},
		{"impossible date", Receipt{PurchasedAt: ptr("2026-02-30"), TotalAmount: ptr(int64(1))}, FailureInvalidDate},
		{"future", Receipt{PurchasedAt: ptr("2026-09-20"), TotalAmount: ptr(int64(1))}, FailureInvalidDate},
		{"old", Receipt{PurchasedAt: ptr("2021-09-17"), TotalAmount: ptr(int64(1))}, FailureInvalidDate},
		{"no total", Receipt{PurchasedAt: ptr("2026-09-18")}, FailureNoTotalAmount},
		{"bad total", Receipt{PurchasedAt: ptr("2026-09-18"), TotalAmount: ptr(int64(10_000_001))}, FailureInvalidAmount},
		{"bad detail", Receipt{PurchasedAt: ptr("2026-09-18"), TotalAmount: ptr(int64(1)), Details: []Detail{{Name: "x", Amount: -1, Quantity: 1}}}, FailureInvalidAmount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, f := Validate(tt.r, now)
			if tt.code == "" && f != nil {
				t.Fatal(f)
			}
			if tt.code != "" && (f == nil || f.Code != tt.code) {
				t.Fatalf("failure=%v want %s", f, tt.code)
			}
		})
	}
}

func TestValidateDropsEmptyAndTruncates(t *testing.T) {
	t.Parallel()
	r := Receipt{StoreName: ptr(strings.Repeat("店", 101)), PurchasedAt: ptr("2026-09-18"), TotalAmount: ptr(int64(10)), Details: []Detail{{Name: " ", Amount: 1, Quantity: 1}, {Name: strings.Repeat("品", 101), Amount: 2, Quantity: 1, Category: "bad"}}}
	got, f := Validate(r, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f != nil {
		t.Fatal(f)
	}
	if len([]rune(*got.StoreName)) != 100 || len(got.Details) != 1 || len([]rune(got.Details[0].Name)) != 100 || got.Details[0].Category != "unknown" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestValidateRejectsTooManyDetails(t *testing.T) {
	t.Parallel()
	details := make([]Detail, 51)
	for i := range details {
		details[i] = Detail{Name: "x", Amount: 1, Quantity: 1, Category: "food"}
	}
	_, f := Validate(Receipt{PurchasedAt: ptr("2026-09-18"), TotalAmount: ptr(int64(1)), Details: details}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f == nil || f.Code != FailureTooManyDetails {
		t.Fatalf("failure=%v want %s", f, FailureTooManyDetails)
	}
}

func TestBillingYearMonthAndCategoryTotals(t *testing.T) {
	t.Parallel()
	b := Billing{PurchasedAt: "2026-09-18", Details: []BillingDetail{
		{Category: "food", Amount: 100}, {Category: "food", Amount: 50}, {Category: "other", Amount: 1},
	}}
	if got := b.YearMonth(); got != "2026-09" {
		t.Errorf("YearMonth() = %q", got)
	}
	totals := b.CategoryTotals()
	if totals["food"] != 150 || totals["other"] != 1 || len(totals) != 2 {
		t.Errorf("CategoryTotals() = %v", totals)
	}
}
