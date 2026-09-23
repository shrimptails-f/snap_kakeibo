package domain

import (
	"testing"
	"time"
)

func TestValidateReadingChoosesPrintedPaymentTotal(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		candidates []AmountCandidate
		taxes      []TaxBreakdown
		details    []ReadDetail
		want       int64
		status     string
	}{
		{"seven eleven tax line", []AmountCandidate{{Amount: 8, Label: "8%消費税額", Role: "final_total", Position: 4}, {Amount: 100, Label: "8%対象額", Role: "taxable", Position: 3}, {Amount: 108, Label: "お支払合計", Role: "final_total", Position: 9}}, []TaxBreakdown{{Rate: intPtr(8), TaxableAmount: intPtr(100), TaxAmount: intPtr(8), Mode: "external"}}, []ReadDetail{{Name: "食品", Amount: 100, Quantity: 1, TaxRate: intPtr(8), TaxMode: "external"}}, 108, "strong"},
		{"included tax", []AmountCandidate{{Amount: 100, Label: "小計", Role: "subtotal", Position: 2}, {Amount: 7, Label: "内税額", Role: "tax", Position: 3}, {Amount: 100, Label: "合計", Role: "final_total", Position: 5}}, []TaxBreakdown{{Rate: intPtr(8), TaxableAmount: intPtr(93), TaxAmount: intPtr(7), Mode: "included"}}, []ReadDetail{{Name: "食品", Amount: 100, Quantity: 1, TaxRate: intPtr(8), TaxMode: "included"}}, 100, "strong"},
		{"mixed rates and rounded printed taxes", []AmountCandidate{{Amount: 151, Label: "小計", Role: "subtotal", Position: 3}, {Amount: 4, Label: "8%税額", Role: "tax", Position: 4}, {Amount: 10, Label: "10%税額", Role: "tax", Position: 5}, {Amount: 165, Label: "合計", Role: "final_total", Position: 8}}, []TaxBreakdown{{Rate: intPtr(8), TaxAmount: intPtr(4), Mode: "external"}, {Rate: intPtr(10), TaxAmount: intPtr(10), Mode: "external"}}, []ReadDetail{{Name: "食品", Amount: 51, Quantity: 1, TaxRate: intPtr(8), TaxMode: "external"}, {Name: "雑貨", Amount: 100, Quantity: 1, TaxRate: intPtr(10), TaxMode: "external"}}, 165, "strong"},
		{"discount", []AmountCandidate{{Amount: 200, Label: "小計", Role: "subtotal", Position: 2}, {Amount: 20, Label: "値引", Role: "discount", Position: 3}, {Amount: 14, Label: "消費税額", Role: "tax", Position: 4}, {Amount: 194, Label: "お買上げ計", Role: "final_total", Position: 7}}, []TaxBreakdown{{Rate: intPtr(8), TaxAmount: intPtr(14), Mode: "external"}}, []ReadDetail{{Name: "食品", Amount: 200, Quantity: 1}}, 194, "strong"},
		{"missing product", []AmountCandidate{{Amount: 270, Label: "合計", Role: "final_total", Position: 9}, {Amount: 300, Label: "お預り", Role: "deposit", Position: 10}}, nil, []ReadDetail{{Name: "食品", Amount: 100, Quantity: 1}}, 270, "strong"},
		{"competing totals", []AmountCandidate{{Amount: 100, Label: "合計", Role: "final_total", Position: 8}, {Amount: 120, Label: "合計", Role: "final_total", Position: 9}}, nil, nil, 120, "weak"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, f := ValidateReading(ReceiptReading{PurchaseDate: strPtr("2026-09-18"), AmountCandidates: tt.candidates, TaxBreakdown: tt.taxes, Details: tt.details}, now)
			if f != nil {
				t.Fatalf("failure=%v", f)
			}
			if got.ReadAmount().Yen() != tt.want || got.Evidence().Status != tt.status {
				t.Fatalf("amount=%d evidence=%+v", got.ReadAmount().Yen(), got.Evidence())
			}
			if tt.name == "mixed rates and rounded printed taxes" && got.Evidence().TaxMode != "external" {
				t.Fatalf("tax mode=%s", got.Evidence().TaxMode)
			}
			if got.Evidence().Selected == nil {
				t.Fatal("missing selected candidate")
			}
		})
	}
}

func TestValidateReadingRejectsTaxOnly(t *testing.T) {
	t.Parallel()
	_, f := ValidateReading(ReceiptReading{PurchaseDate: strPtr("2026-09-18"), AmountCandidates: []AmountCandidate{{Amount: 8, Label: "8%税額", Role: "final_total", Position: 10}}}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f == nil || f.Code() != FailureNoTotalAmount {
		t.Fatalf("failure=%v", f)
	}
}

func TestValidateReadingRejectsSubtotalOnly(t *testing.T) {
	t.Parallel()
	_, f := ValidateReading(ReceiptReading{PurchaseDate: strPtr("2026-09-18"), AmountCandidates: []AmountCandidate{{Amount: 100, Label: "小計", Role: "subtotal", Position: 8}}}, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC))
	if f == nil || f.Code() != FailureNoTotalAmount {
		t.Fatalf("failure=%v", f)
	}
}

func TestReconcileAmountsReportsMixedTaxMode(t *testing.T) {
	t.Parallel()
	e := ReconcileAmounts(ReceiptReading{TaxBreakdown: []TaxBreakdown{{Mode: "included"}, {Mode: "external"}}})
	if e.TaxMode != "mixed" {
		t.Fatalf("tax mode=%s", e.TaxMode)
	}
}
