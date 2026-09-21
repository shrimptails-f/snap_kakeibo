package domain

import (
	"strings"
	"testing"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
)

func strPtr(v string) *string { return &v }
func intPtr(v int64) *int64   { return &v }

func TestValidateReading(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.FixedZone("JST", 9*3600))
	tests := []struct {
		name string
		r    ReceiptReading
		code string
	}{
		{"valid", ReceiptReading{PurchaseDate: strPtr("2026-09-19"), ReadAmount: intPtr(1), Details: []ReadDetail{{Name: "品物", Amount: 0, Quantity: 1, Category: "food"}}}, ""},
		{"no date", ReceiptReading{ReadAmount: intPtr(1)}, FailureNoDate},
		{"impossible date", ReceiptReading{PurchaseDate: strPtr("2026-02-30"), ReadAmount: intPtr(1)}, FailureInvalidDate},
		{"future", ReceiptReading{PurchaseDate: strPtr("2026-09-20"), ReadAmount: intPtr(1)}, FailureInvalidDate},
		{"old", ReceiptReading{PurchaseDate: strPtr("2021-09-17"), ReadAmount: intPtr(1)}, FailureInvalidDate},
		{"no total", ReceiptReading{PurchaseDate: strPtr("2026-09-18")}, FailureNoTotalAmount},
		{"bad total", ReceiptReading{PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(10_000_001)}, FailureInvalidAmount},
		{"bad detail amount", ReceiptReading{PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(1), Details: []ReadDetail{{Name: "x", Amount: -1, Quantity: 1}}}, FailureInvalidAmount},
		{"bad detail quantity", ReceiptReading{PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(1), Details: []ReadDetail{{Name: "x", Amount: 1, Quantity: 0}}}, FailureInvalidAmount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, f := ValidateReading(tt.r, now)
			if tt.code == "" && f != nil {
				t.Fatalf("failure = %v", *f)
			}
			if tt.code != "" && (f == nil || f.Code() != tt.code) {
				t.Fatalf("failure=%v want %s", f, tt.code)
			}
		})
	}
}

func TestValidateReadingConvertsToAnalysisResult(t *testing.T) {
	t.Parallel()
	r := ReceiptReading{StoreName: strPtr(" 店 "), PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(1500), Details: []ReadDetail{
		{Name: "牛乳", Amount: 200, Quantity: 1, Category: "food"},
		{Name: "会食", Amount: 1300, Quantity: 1, Category: "social"},
	}}
	got, f := ValidateReading(r, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f != nil {
		t.Fatalf("failure = %v", *f)
	}
	if got.StoreName() != "店" || got.PurchaseDate().String() != "2026-09-18" || got.ReadAmount().Yen() != 1500 || !got.HasData() {
		t.Errorf("result = %+v", got)
	}
	details := got.Details()
	if len(details) != 2 || details[0].Category() != common.CategoryFood || details[1].Category() != common.CategorySocial || details[1].Amount().Yen() != 1300 {
		t.Errorf("details = %+v", details)
	}
}

func TestValidateReadingDropsEmptyAndTruncates(t *testing.T) {
	t.Parallel()
	r := ReceiptReading{StoreName: strPtr(strings.Repeat("店", 101)), PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(10), Details: []ReadDetail{{Name: " ", Amount: 1, Quantity: 1}, {Name: strings.Repeat("品", 101), Amount: 2, Quantity: 1, Category: "bad"}}}
	got, f := ValidateReading(r, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f != nil {
		t.Fatalf("failure = %v", *f)
	}
	details := got.Details()
	if len([]rune(got.StoreName())) != 100 || len(details) != 1 || len([]rune(details[0].Name())) != 100 || details[0].Category() != common.CategoryUnknown {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestValidateReadingWithoutDetailsHasNoData(t *testing.T) {
	t.Parallel()
	got, f := ValidateReading(ReceiptReading{PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(10)}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f != nil || got.HasData() {
		t.Fatalf("result = %+v failure = %v", got, f)
	}
}

func TestValidateReadingRejectsTooManyDetails(t *testing.T) {
	t.Parallel()
	details := make([]ReadDetail, 51)
	for i := range details {
		details[i] = ReadDetail{Name: "x", Amount: 1, Quantity: 1, Category: "food"}
	}
	_, f := ValidateReading(ReceiptReading{PurchaseDate: strPtr("2026-09-18"), ReadAmount: intPtr(1), Details: details}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f == nil || f.Code() != FailureTooManyDetails {
		t.Fatalf("failure=%v want %s", f, FailureTooManyDetails)
	}
}

func TestFailureReasonHelpers(t *testing.T) {
	t.Parallel()
	if f := AnalysisFailed(" 拒否 "); f.Code() != FailureAnalysisFailed || f.SafeMessage() != "拒否" {
		t.Errorf("AnalysisFailed() = %+v", f)
	}
	if f := InternalFailure("x"); f.Code() != FailureInternal {
		t.Errorf("InternalFailure() = %+v", f)
	}
}
