package analyze

import (
	"errors"
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
		{"no date", Receipt{TotalAmount: ptr(int64(1))}, "NO_DATE"},
		{"impossible date", Receipt{PurchasedAt: ptr("2026-02-30"), TotalAmount: ptr(int64(1))}, "INVALID_DATE"},
		{"future", Receipt{PurchasedAt: ptr("2026-09-20"), TotalAmount: ptr(int64(1))}, "INVALID_DATE"},
		{"old", Receipt{PurchasedAt: ptr("2021-09-17"), TotalAmount: ptr(int64(1))}, "INVALID_DATE"},
		{"no total", Receipt{PurchasedAt: ptr("2026-09-18")}, "NO_TOTAL_AMOUNT"},
		{"bad total", Receipt{PurchasedAt: ptr("2026-09-18"), TotalAmount: ptr(int64(10_000_001))}, "INVALID_AMOUNT"},
		{"bad detail", Receipt{PurchasedAt: ptr("2026-09-18"), TotalAmount: ptr(int64(1)), Details: []Detail{{Name: "x", Amount: -1, Quantity: 1}}}, "INVALID_AMOUNT"},
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

func TestParseResponse(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","Text":"{\"store_name\":null,\"purchased_at\":\"2026-09-18\",\"total_amount\":100,\"details\":[]}"}]}]}`)
	// Responses API uses lowercase "text"; encoding/json matches field name Text case-insensitively.
	got, resp, err := ParseResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "resp_1" || *got.TotalAmount != 100 {
		t.Fatalf("got=%+v resp=%+v", got, resp)
	}
	_, _, err = ParseResponse([]byte(`{"status":"completed","output":[{"content":[{"type":"refusal","refusal":"no"}]}]}`))
	var f *Failure
	if !errors.As(err, &f) || f.Code != "ANALYSIS_FAILED" {
		t.Fatalf("err=%v", err)
	}
	_, _, err = ParseResponse([]byte(`{"status":"failed"}`))
	if !errors.Is(err, ErrTemporary) {
		t.Fatalf("err=%v", err)
	}
}
