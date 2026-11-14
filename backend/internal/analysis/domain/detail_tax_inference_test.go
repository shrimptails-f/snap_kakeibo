package domain

import (
	"math/rand"
	"reflect"
	"testing"
)

func inferenceReading(amounts []int64, taxes []TaxBreakdown, total int64) ReceiptReading {
	r := ReceiptReading{TaxBreakdown: taxes, AmountCandidates: []AmountCandidate{{Amount: total, Label: "合計", Role: "final_total"}}}
	for _, amount := range amounts {
		r.Details = append(r.Details, ReadDetail{Name: "商品", Amount: amount, Quantity: 1})
	}
	return r
}
func externalGroup(rate, base, tax int64) TaxBreakdown {
	return TaxBreakdown{Rate: &rate, TaxableAmount: &base, TaxAmount: &tax, Mode: "external"}
}
func seiyuReading() ReceiptReading {
	return inferenceReading([]int64{199, 516, 398, 369, 518, 6}, []TaxBreakdown{externalGroup(10, 2006, 200)}, 2206)
}
func mixedReading() ReceiptReading {
	return inferenceReading([]int64{100, 200}, []TaxBreakdown{externalGroup(8, 100, 8), externalGroup(10, 200, 20)}, 328)
}
func ambiguousReading() ReceiptReading {
	return inferenceReading([]int64{100, 100}, []TaxBreakdown{externalGroup(8, 100, 8), externalGroup(10, 100, 10)}, 218)
}
func familyReading() ReceiptReading {
	r := inferenceReading([]int64{190, 170, 158, 170, 140}, []TaxBreakdown{{Rate: intPtr(8), GrossAmount: intPtr(808), TaxAmount: intPtr(59), Mode: "included"}}, 808)
	r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Amount: 20, Role: "discount", Label: "値引合計"})
	r.TaxMarks = []TaxMark{{Mark: "軽", Rate: intPtr(8), Note: "軽は軽減税率8%対象"}}
	for i := range r.Details {
		r.Details[i].TaxMark = "軽"
	}
	return r
}

func TestInferDetailTaxes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		reading        func() ReceiptReading
		change         func(*ReceiptReading)
		status, reason string
		rates          []int64
	}{
		{name: "内外税不明を商品合計と税抜対象額から補完", reading: seiyuReading, change: func(r *ReceiptReading) { r.TaxBreakdown[0].Mode = "unknown" }, status: "unique", rates: []int64{10, 10, 10, 10, 10, 10}},
		{name: "内外税不明を税込対象額と値引きから補完", reading: familyReading, change: func(r *ReceiptReading) { r.TaxBreakdown[0].Mode = "unknown" }, status: "unique", rates: []int64{8, 8, 8, 8, 8}},
		{name: "内税混在の税込対象額", reading: func() ReceiptReading {
			return inferenceReading([]int64{108, 220}, []TaxBreakdown{{Rate: intPtr(8), GrossAmount: intPtr(108), TaxAmount: intPtr(8), Mode: "included"}, {Rate: intPtr(10), GrossAmount: intPtr(220), TaxAmount: intPtr(20), Mode: "included"}}, 328)
		}, status: "unique", rates: []int64{8, 10}},
		{name: "西友の全商品を10%へ補完", reading: seiyuReading, status: "unique", rates: []int64{10, 10, 10, 10, 10, 10}},
		{name: "ファミマの軽と税込対象額と値引き", reading: familyReading, status: "unique", rates: []int64{8, 8, 8, 8, 8}},
		{name: "異なる金額の混在税率は一意", reading: mixedReading, status: "unique", rates: []int64{8, 10}},
		{name: "同額商品は曖昧", reading: ambiguousReading, status: "ambiguous", rates: []int64{0, 0}},
		{name: "商品情報は候補を順位付けする", reading: ambiguousReading, change: func(r *ReceiptReading) {
			r.Details[0].Name = "パン"
			r.Details[0].Category = "food"
			r.Details[1].Name = "洗剤"
			r.Details[1].Category = "daily_goods"
		}, status: "estimated", rates: []int64{0, 0}},
		{name: "カテゴリが誤っても一意な金額制約を優先", reading: mixedReading, change: func(r *ReceiptReading) { r.Details[0].Category = "daily_goods"; r.Details[1].Category = "food" }, status: "unique", rates: []int64{8, 10}},
		{name: "印のある商品を固定して同額の曖昧性を解消", reading: ambiguousReading, change: func(r *ReceiptReading) {
			r.Details[0].TaxMark = "軽"
			r.TaxMarks = []TaxMark{{Mark: "軽", Rate: intPtr(8), Note: "軽:8%"}}
		}, status: "unique", rates: []int64{8, 10}},
		{name: "注記がない印だけで確定しない", reading: ambiguousReading, change: func(r *ReceiptReading) {
			r.Details[0].TaxMark = "軽"
			r.TaxMarks = []TaxMark{{Mark: "軽", Rate: intPtr(8)}}
		}, status: "ambiguous", rates: []int64{0, 0}},
		{name: "印と読取税率が矛盾", reading: familyReading, change: func(r *ReceiptReading) { r.Details[0].TaxRate = intPtr(10) }, status: "conflict", reason: "conflicting_marks"},
		{name: "同じ印に相反する注記", reading: familyReading, change: func(r *ReceiptReading) {
			r.TaxMarks = append(r.TaxMarks, TaxMark{Mark: "軽", Rate: intPtr(10), Note: "軽:10%"})
		}, status: "conflict", reason: "conflicting_marks"},
		{name: "商品が欠落", reading: seiyuReading, change: func(r *ReceiptReading) { r.Details = r.Details[:5] }, status: "conflict", reason: "amount_mismatch"},
		{name: "合計の読取誤り", reading: seiyuReading, change: func(r *ReceiptReading) { r.AmountCandidates[0].Amount++ }, status: "conflict", reason: "amount_mismatch"},
		{name: "税額の桁誤り", reading: seiyuReading, change: func(r *ReceiptReading) { r.TaxBreakdown[0].TaxAmount = intPtr(20) }, status: "conflict", reason: "inconsistent_breakdown"},
		{name: "内税と外税の矛盾", reading: seiyuReading, change: func(r *ReceiptReading) { r.Details[0].TaxMode = "included" }, status: "conflict", reason: "conflicting_mode"},
		{name: "明示税率と単一税率の矛盾", reading: seiyuReading, change: func(r *ReceiptReading) { r.Details[0].TaxRate = intPtr(8) }, status: "conflict", reason: "conflicting_rate"},
		{name: "重複した税率群", reading: mixedReading, change: func(r *ReceiptReading) { r.TaxBreakdown[1].Rate = intPtr(8) }, status: "conflict", reason: "inconsistent_breakdown"},
		{name: "金額に合う組み合わせなし", reading: mixedReading, change: func(r *ReceiptReading) { r.Details[0].Amount = 150; r.Details[1].Amount = 150 }, status: "conflict", reason: "no_solution"},
		{name: "根拠のない税抜対象額を推測しない", reading: seiyuReading, change: func(r *ReceiptReading) { r.TaxBreakdown[0].TaxableAmount = nil }, status: "unresolved", reason: "insufficient_evidence"},
		{name: "非対応税率を8か10へ強制しない", reading: mixedReading, change: func(r *ReceiptReading) { r.TaxBreakdown[0].Rate = intPtr(0) }, status: "unresolved", reason: "unsupported_tax_group"},
		{name: "個別値引きの帰属を利用", reading: mixedReading, change: func(r *ReceiptReading) {
			r.Details[0].Amount = 120
			r.Details[0].DiscountAmount = intPtr(20)
			r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Role: "discount", Amount: 20})
		}, status: "unique", rates: []int64{8, 10}},
		{name: "混在税率の値引き帰属不明", reading: mixedReading, change: func(r *ReceiptReading) {
			r.Details[0].Amount = 120
			r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Role: "discount", Amount: 20})
		}, status: "unresolved", reason: "unassigned_discount"},
		{name: "値引き帰属不明でも内外税の矛盾を検出", reading: mixedReading, change: func(r *ReceiptReading) {
			r.Details[0].Amount = 120
			r.Details[0].TaxMode = "included"
			r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Role: "discount", Amount: 20})
		}, status: "conflict", reason: "conflicting_mode"},
		{name: "値引き帰属不明でも明示税率の矛盾を検出", reading: mixedReading, change: func(r *ReceiptReading) {
			r.Details[0].Amount = 120
			r.Details[0].TaxRate = intPtr(5)
			r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Role: "discount", Amount: 20})
		}, status: "conflict", reason: "conflicting_rate"},
		{name: "個別と合計の値引きを二重計上しない", reading: familyReading, change: func(r *ReceiptReading) {
			r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Role: "discount", Label: "個別値引き", Amount: 20})
			r.Details[0].DiscountAmount = intPtr(20)
		}, status: "unique", rates: []int64{8, 8, 8, 8, 8}},
		{name: "税抜と税込対象額の矛盾", reading: seiyuReading, change: func(r *ReceiptReading) { r.TaxBreakdown[0].GrossAmount = intPtr(999) }, status: "conflict", reason: "inconsistent_breakdown"},
		{name: "印だけ確定し合計欄不足を保持", reading: familyReading, change: func(r *ReceiptReading) { r.TaxBreakdown = nil }, status: "unresolved", reason: "insufficient_evidence", rates: []int64{8, 8, 8, 8, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := tt.reading()
			if tt.change != nil {
				tt.change(&r)
			}
			e := InferDetailTaxes(r, ReconcileAmounts(r))
			if e.Inference.Status != tt.status || (tt.reason != "" && e.Inference.Reason != tt.reason) {
				t.Fatalf("inference=%+v", e.Inference)
			}
			if tt.rates != nil {
				for i, want := range tt.rates {
					got := e.DetailTaxes[i].Rate
					if (want == 0 && got != nil) || (want != 0 && (got == nil || *got != want)) {
						t.Errorf("detail %d=%+v want %d", i, e.DetailTaxes[i], want)
					}
				}
			}
			if tt.status == "estimated" {
				if *e.DetailTaxes[0].SuggestedRate != 8 || *e.DetailTaxes[1].SuggestedRate != 10 {
					t.Fatalf("suggestions=%+v", e.DetailTaxes)
				}
				for _, a := range AllocateDetailTaxes(r.Details, e) {
					if a != nil {
						t.Fatal("estimated tax was confirmed")
					}
				}
			}
			if tt.status == "conflict" {
				for _, a := range AllocateDetailTaxes(r.Details, e) {
					if a != nil {
						t.Fatal("conflicting tax was confirmed")
					}
				}
			}
			if tt.name == "ファミマの軽と税込対象額と値引き" {
				for i, a := range AllocateDetailTaxes(r.Details, e) {
					if a == nil || *a != r.Details[i].Amount {
						t.Fatal("included tax added again")
					}
				}
			}
			if tt.name == "西友の全商品を10%へ補完" {
				var sum int64
				for _, a := range AllocateDetailTaxes(r.Details, e) {
					if a == nil {
						t.Fatal("not allocated")
					}
					sum += *a
				}
				if sum != 2206 {
					t.Fatalf("sum=%d", sum)
				}
			}
		})
	}
}

func TestTaxSearchMatchesExhaustiveSmallReceipts(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewSource(105))
	for trial := 0; trial < 100; trial++ {
		n := 8
		amounts := make([]int64, n)
		details := make([]ReadDetail, n)
		taxes := make([]DetailTax, n)
		var total int64
		for i := range amounts {
			amounts[i] = int64(random.Intn(10))
			total += amounts[i]
			details[i] = ReadDetail{Category: []string{"food", "daily_goods", "unknown"}[random.Intn(3)]}
		}
		target := int64(random.Intn(int(total) + 1))
		groups := []taxGroup{{rate: 8, target: target}, {rate: 10, target: total - target}}
		got, _, limit := searchTaxAssignments(details, amounts, taxes, groups)
		var want []taxAssignment
		for mask := uint64(0); mask < 1<<n; mask++ {
			var sum int64
			score := 0
			for i, a := range amounts {
				rate := int64(10)
				if mask&(1<<i) == 0 {
					sum += a
					rate = 8
				}
				score += taxPreference(details[i], rate)
			}
			if sum == target {
				want = bestTwoAssignments(want, taxAssignment{mask: mask, score: score})
			}
		}
		if limit || !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d got=%v want=%v", trial, got, want)
		}
	}
}

func TestTaxSearchBounds(t *testing.T) {
	t.Parallel()
	t.Run("50明細の同額商品", func(t *testing.T) {
		r := inferenceReading(make([]int64, 50), []TaxBreakdown{externalGroup(8, 2500, 200), externalGroup(10, 2500, 250)}, 5450)
		for i := range r.Details {
			r.Details[i].Amount = 100
		}
		e := InferDetailTaxes(r, ReconcileAmounts(r))
		if e.Inference.Status != "ambiguous" || e.Inference.States > 10000 {
			t.Fatalf("inference=%+v", e.Inference)
		}
	})
	t.Run("異なる多数の部分和を上限で止める", func(t *testing.T) {
		amounts := make([]int64, 22)
		var total int64
		for i := range amounts {
			amounts[i] = 1 << i
			total += amounts[i]
		}
		base := total / 2
		r := inferenceReading(amounts, []TaxBreakdown{externalGroup(8, base, base*8/100), externalGroup(10, total-base, (total-base)*10/100)}, total+base*8/100+(total-base)*10/100)
		e := InferDetailTaxes(r, ReconcileAmounts(r))
		if e.Inference.Reason != "search_limit" {
			t.Fatalf("inference=%+v", e.Inference)
		}
		for _, d := range e.DetailTaxes {
			if d.Rate != nil || d.SuggestedRate != nil || d.Reason != "search_limit" {
				t.Fatalf("detail=%+v", d)
			}
		}
	})
}

// Issue #105 の実画像を目視転記した商品行。画像・店舗住所等はリポジトリへ保存しない。
func TestFamilyMartIssue105PrintedRows(t *testing.T) {
	t.Parallel()
	r := inferenceReading([]int64{159, 186, 183, 171, 640, 372, 372, 798, 645, 268, 198}, []TaxBreakdown{{Rate: intPtr(8), GrossAmount: intPtr(3982), TaxAmount: intPtr(294), Mode: "unknown"}}, 3982)
	r.TaxMarks = []TaxMark{{Mark: "軽", Rate: intPtr(8), Note: "「軽」は軽減税率対象商品です。"}}
	r.AmountCandidates = append(r.AmountCandidates, AmountCandidate{Amount: 10, Role: "discount", Label: "値引合計"})
	for i := range r.Details {
		r.Details[i].TaxMark = "軽"
	}
	r.Details[0].DiscountAmount = intPtr(10)
	e := InferDetailTaxes(r, ReconcileAmounts(r))
	if e.Inference.Status != "unique" || e.TaxMode != "included" {
		t.Fatalf("inference=%+v", e)
	}
	for i, d := range e.DetailTaxes {
		if d.OriginalAmount == nil || *d.OriginalAmount != r.Details[i].Amount || d.OriginalName != r.Details[i].Name {
			t.Fatalf("original detail %d lost", i)
		}
		if d.Rate == nil || *d.Rate != 8 || d.Mode != "included" || d.Reason != "printed_mark" || d.OriginalRate != nil {
			t.Fatalf("detail %d=%+v", i, d)
		}
	}
	var sum int64
	for _, amount := range AllocateDetailTaxes(r.Details, e) {
		if amount == nil {
			t.Fatal("not confirmed")
		}
		sum += *amount
	}
	if sum != 3992 {
		t.Fatalf("printed sum=%d; discount must remain separate", sum)
	}
	if e.Taxes[0].OriginalMode != "unknown" || r.TaxBreakdown[0].Mode != "unknown" {
		t.Fatal("original reading mutated or lost")
	}
}

func BenchmarkInferDetailTaxes50Rows(b *testing.B) {
	r := inferenceReading(make([]int64, 50), []TaxBreakdown{externalGroup(8, 2500, 200), externalGroup(10, 2500, 250)}, 5450)
	for i := range r.Details {
		r.Details[i].Amount = 100
	}
	for b.Loop() {
		InferDetailTaxes(r, ReconcileAmounts(r))
	}
}
