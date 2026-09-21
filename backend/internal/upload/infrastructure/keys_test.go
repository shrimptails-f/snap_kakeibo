package infrastructure

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/infrastructure"
)

// TestKeysMatchAnalysisFormat は analysis_requests を遷移させる analyze-receipt のキー形式と一致することを確認する。
// AnalysisRequestMonthSK は upload だけが書くので比較対象が無い。
func TestKeysMatchAnalysisFormat(t *testing.T) {
	t.Parallel()
	pairs := map[string][2]string{
		"UserPK":            {UserPK("u1"), infrastructure.UserPK("u1")},
		"AnalysisRequestSK": {AnalysisRequestSK("req1"), infrastructure.AnalysisRequestSK("req1")},
		"UserMonthPK":       {UserMonthPK("u1", "2026-09"), infrastructure.UserMonthPK("u1", "2026-09")},
	}
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, analysis = %q", name, pair[0], pair[1])
		}
	}
	if got := AnalysisRequestMonthSK("2026-09-18T12:00:00Z", "req1"); got != "ANALYSIS_REQUEST_CREATED_AT#2026-09-18T12:00:00Z#req1" {
		t.Errorf("AnalysisRequestMonthSK() = %q", got)
	}
}
