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

func TestValidateReadingSevenElevenInvoiceWithDiscountAndPostTotalBreakdown(t *testing.T) {
	t.Parallel()
	amounts := []int64{182, 173, 170, 396, 350, 173, 188, 175, 680, 138, 550, 648, 5}
	details := make([]ReadDetail, 0, len(amounts))
	for _, amount := range amounts {
		details = append(details, ReadDetail{Name: "商品", Amount: amount, Quantity: 1})
	}
	r := ReceiptReading{PurchaseDate: strPtr("2026-09-23"), Details: details,
		AmountCandidates: []AmountCandidate{
			{Amount: -20, Label: "値引額", Role: "discount", Position: 2},
			{Amount: -32, Label: "値引額", Role: "discount", Position: 4},
			{Amount: -20, Label: "値引額", Role: "discount", Position: 8},
			{Amount: -30, Label: "値引", Role: "discount", Position: 12},
			{Amount: 3828, Label: "商品代金", Role: "final_total", Position: 16},
			{Amount: -102, Label: "値引合計", Role: "discount", Position: 17},
			{Amount: -102, Label: "税率8%対象", Role: "final_total", Position: 18},
			{Amount: 3539, Label: "小計（税抜8%）", Role: "subtotal", Position: 19},
			{Amount: 283, Label: "消費税等（8%）", Role: "final_total", Position: 20},
			{Amount: 187, Label: "小計（税抜10%）", Role: "subtotal", Position: 21},
			{Amount: 18, Label: "消費税等（10%）", Role: "tax", Position: 22},
			{Amount: 4027, Label: "合計", Role: "final_total", Position: 25},
			{Amount: 3822, Label: "（税率8%対象）", Role: "final_total", Position: 30},
			{Amount: 205, Label: "（税率10%対象）", Role: "final_total", Position: 31},
			{Amount: 283, Label: "（内消費税等8%）", Role: "final_total", Position: 32},
			{Amount: 18, Label: "（内消費税等10%）", Role: "final_total", Position: 33},
			{Amount: 4027, Label: "PayPay支払", Role: "final_total", Position: 34},
		},
		TaxBreakdown: []TaxBreakdown{{Rate: intPtr(8), TaxableAmount: intPtr(3539), TaxAmount: intPtr(283), Mode: "external"}, {Rate: intPtr(10), TaxableAmount: intPtr(187), TaxAmount: intPtr(18), Mode: "external"}},
	}
	result, f := ValidateReading(r, time.Date(2026, 9, 23, 18, 0, 0, 0, time.FixedZone("JST", 9*3600)))
	if f != nil {
		t.Fatalf("failure=%v", f)
	}
	e := result.Evidence()
	if result.ReadAmount().Yen() != 4027 || e.Selected == nil || e.Selected.Label != "合計" || e.Status != "strong" || e.TaxMode != "external" {
		t.Fatalf("read amount=%d evidence=%+v", result.ReadAmount().Yen(), e)
	}
}

func TestValidateReadingDoesNotUseOnePartOfSplitPaymentAsTotal(t *testing.T) {
	t.Parallel()
	r := ReceiptReading{PurchaseDate: strPtr("2026-09-23"), AmountCandidates: []AmountCandidate{{Amount: 2000, Label: "PayPay支払", Role: "payment", Position: 20}, {Amount: 2027, Label: "現金支払", Role: "payment", Position: 21}}}
	_, failure := ValidateReading(r, time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC))
	if failure == nil || failure.Code() != FailureNoTotalAmount {
		t.Fatalf("failure=%v", failure)
	}
}

func TestValidateReadingSeiyuWithSeparateTaxHeadings(t *testing.T) {
	t.Parallel()
	items := []ReadDetail{{Name: "キッチンタオル", Amount: 199, Quantity: 1}, {Name: "洗濯槽クリーナー", Amount: 516, Quantity: 2}, {Name: "雑巾", Amount: 398, Quantity: 2}, {Name: "マジックリン", Amount: 369, Quantity: 1}, {Name: "ネット", Amount: 518, Quantity: 2}, {Name: "レジ袋", Amount: 6, Quantity: 1}}
	for _, tt := range []struct{ name, subtotalLabel, taxableLabel, taxLabel, paymentLabel string }{
		{"headings attached", "小計 9点", "税抜金額対象 10% 9点", "消費税額 10% 9点", "支払い PayPay"},
		{"headings omitted", "9点", "10% 9点", "10% 9点", "PayPay"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := ReceiptReading{PurchaseDate: strPtr("2026-09-19"), Details: items, AmountCandidates: []AmountCandidate{
				{Amount: 2006, Label: tt.subtotalLabel, Role: "final_total", Position: 8},
				{Amount: 2006, Label: tt.taxableLabel, Role: "final_total", Position: 10},
				{Amount: 200, Label: tt.taxLabel, Role: "final_total", Position: 12},
				{Amount: 2206, Label: "合計", Role: "final_total", Position: 14},
				{Amount: 2206, Label: tt.paymentLabel, Role: "unknown", Position: 16},
			}, TaxBreakdown: []TaxBreakdown{{Rate: intPtr(10), TaxableAmount: intPtr(2006), TaxAmount: intPtr(200), Mode: "external"}}}
			result, f := ValidateReading(r, time.Date(2026, 9, 19, 20, 0, 0, 0, time.UTC))
			if f != nil {
				t.Fatalf("failure=%v", f)
			}
			e := result.Evidence()
			if result.ReadAmount().Yen() != 2206 || e.Selected == nil || e.Selected.Label != "合計" || e.Status != "strong" {
				t.Fatalf("read amount=%d evidence=%+v", result.ReadAmount().Yen(), e)
			}
		})
	}
}

func TestValidateReadingFamilyMartIncludedTaxAfterDiscount(t *testing.T) {
	t.Parallel()
	r := ReceiptReading{PurchaseDate: strPtr("2026-09-19"), Details: []ReadDetail{
		{Name: "パン", Amount: 190, Quantity: 1, TaxRate: intPtr(8), TaxMode: "included"},
		{Name: "パン", Amount: 170, Quantity: 1, TaxRate: intPtr(8), TaxMode: "included"},
		{Name: "パン", Amount: 158, Quantity: 1, TaxRate: intPtr(8), TaxMode: "included"},
		{Name: "カレー", Amount: 170, Quantity: 1, TaxRate: intPtr(8), TaxMode: "included"},
		{Name: "パン", Amount: 140, Quantity: 1, TaxRate: intPtr(8), TaxMode: "included"},
	}, AmountCandidates: []AmountCandidate{
		{Amount: 828, Label: "商品合計", Role: "final_total", Position: 20},
		{Amount: -20, Label: "値引合計", Role: "discount", Position: 21},
		{Amount: 808, Label: "合計", Role: "final_total", Position: 25},
		{Amount: 808, Label: "8%対象", Role: "final_total", Position: 30},
		{Amount: 59, Label: "内消費税等", Role: "final_total", Position: 31},
		{Amount: 808, Label: "PayPay支払", Role: "final_total", Position: 32},
	}, TaxBreakdown: []TaxBreakdown{{Rate: intPtr(8), TaxableAmount: nil, TaxAmount: intPtr(59), Mode: "included"}}}
	result, f := ValidateReading(r, time.Date(2026, 9, 19, 20, 0, 0, 0, time.UTC))
	if f != nil {
		t.Fatalf("failure=%v", f)
	}
	e := result.Evidence()
	if result.ReadAmount().Yen() != 808 || e.Selected == nil || e.Selected.Label != "合計" || e.Status != "strong" || e.TaxMode != "included" {
		t.Fatalf("read amount=%d evidence=%+v", result.ReadAmount().Yen(), e)
	}
}
