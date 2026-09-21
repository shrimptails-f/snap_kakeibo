package infrastructure

import "testing"

// TestDetailMonthSK は金額の降順が文字列の昇順になるゼロ埋めと、MaxInt32 を超える金額でも負にならないことを確認する。
// 他 feature とのキー形式の一致は、書く側(ここ)を正として upload / billing の keys_test.go が比較する。
func TestDetailMonthSK(t *testing.T) {
	t.Parallel()
	if got := DetailMonthSK(1000, "2026-09-18", "d1"); got != "DETAIL_AMOUNT#2147482647#2026-09-18#d1" {
		t.Errorf("DetailMonthSK() = %q", got)
	}
	if got := DetailMonthSK(1<<40, "2026-09-18", "d1"); got != "DETAIL_AMOUNT#0000000000#2026-09-18#d1" {
		t.Errorf("DetailMonthSK(overflow) = %q", got)
	}
	// 金額が大きいほど SK が小さく(先に)なる
	if DetailMonthSK(2000, "2026-09-18", "d1") >= DetailMonthSK(1000, "2026-09-18", "d1") {
		t.Error("DetailMonthSK should order larger amounts first")
	}
}
