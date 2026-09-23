package domain

import "testing"

func taxPointer(value int64) *int64 { return &value }

func TestAllocateDetailTaxes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		details  []ReadDetail
		evidence AmountEvidence
		want     []*int64
	}{
		{
			name:     "外税の印字税額を端数込みで配分する",
			details:  []ReadDetail{{Amount: 100}, {Amount: 101}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Rate: taxPointer(10), Mode: "external"}, {Rate: taxPointer(10), Mode: "external"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(10), TaxableAmount: taxPointer(201), TaxAmount: taxPointer(20), Mode: "external"}}},
			want:     []*int64{taxPointer(110), taxPointer(111)},
		},
		{
			name:     "二税率を別々の印字税額で配分する",
			details:  []ReadDetail{{Amount: 100}, {Amount: 200}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Rate: taxPointer(8), Mode: "external"}, {Rate: taxPointer(10), Mode: "external"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(8), TaxableAmount: taxPointer(100), TaxAmount: taxPointer(8), Mode: "external"}, {Rate: taxPointer(10), TaxableAmount: taxPointer(200), TaxAmount: taxPointer(20), Mode: "external"}}},
			want:     []*int64{taxPointer(108), taxPointer(220)},
		},
		{
			name:     "単一税率の値引き後の課税対象額を確認する",
			details:  []ReadDetail{{Amount: 100}, {Amount: 100}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Rate: taxPointer(10), Mode: "external"}, {Rate: taxPointer(10), Mode: "external"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(10), TaxableAmount: taxPointer(180), TaxAmount: taxPointer(18), Mode: "external"}}, Candidates: []AmountCandidate{{Label: "個別値引", Amount: 10}, {Label: "値引合計", Amount: 20}}},
			want:     []*int64{taxPointer(109), taxPointer(109)},
		},
		{
			name:     "二税率の値引き後の課税対象額を確認する",
			details:  []ReadDetail{{Amount: 100}, {Amount: 200}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Rate: taxPointer(8), Mode: "external"}, {Rate: taxPointer(10), Mode: "external"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(8), TaxableAmount: taxPointer(90), TaxAmount: taxPointer(7), Mode: "external"}, {Rate: taxPointer(10), TaxableAmount: taxPointer(190), TaxAmount: taxPointer(19), Mode: "external"}}, Candidates: []AmountCandidate{{Label: "値引合計", Amount: 20}}},
			want:     []*int64{taxPointer(107), taxPointer(219)},
		},
		{
			name:     "内税へ税額を再加算しない",
			details:  []ReadDetail{{Amount: 828}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Rate: taxPointer(8), Mode: "included"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(8), TaxAmount: taxPointer(59), Mode: "included"}}},
			want:     []*int64{taxPointer(828)},
		},
		{
			name:     "商品別税率が不明なら換算しない",
			details:  []ReadDetail{{Amount: 3828}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Mode: "unknown"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(8), TaxableAmount: taxPointer(3539), TaxAmount: taxPointer(283), Mode: "external"}}},
			want:     []*int64{nil},
		},
		{
			name:     "値引きの根拠がない差額を税と推測しない",
			details:  []ReadDetail{{Amount: 2006}},
			evidence: AmountEvidence{DetailTaxes: []DetailTax{{Rate: taxPointer(10), Mode: "external"}}, Taxes: []TaxBreakdown{{Rate: taxPointer(10), TaxableAmount: taxPointer(1900), TaxAmount: taxPointer(190), Mode: "external"}}},
			want:     []*int64{nil},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := AllocateDetailTaxes(test.details, test.evidence)
			if len(got) != len(test.want) {
				t.Fatalf("length = %d, want %d", len(got), len(test.want))
			}
			for i := range got {
				if (got[i] == nil) != (test.want[i] == nil) || (got[i] != nil && *got[i] != *test.want[i]) {
					t.Errorf("detail %d = %v, want %v", i, got[i], test.want[i])
				}
			}
		})
	}
}
