package infrastructure

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/infrastructure"
)

// TestKeysMatchAnalysisFormat は upload_histories を遷移させる analyze-receipt のキー形式と一致することを確認する。
// UploadMonthSK は upload だけが書くので比較対象が無い。
func TestKeysMatchAnalysisFormat(t *testing.T) {
	t.Parallel()
	pairs := map[string][2]string{
		"UserPK":        {UserPK("u1"), infrastructure.UserPK("u1")},
		"UploadSK":      {UploadSK("up1"), infrastructure.UploadSK("up1")},
		"UploadMonthPK": {UploadMonthPK("u1", "2026-09"), infrastructure.UploadMonthPK("u1", "2026-09")},
	}
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, analysis = %q", name, pair[0], pair[1])
		}
	}
	if got := UploadMonthSK("2026-09-18T12:00:00Z", "up1"); got != "UPLOAD_CREATED_AT#2026-09-18T12:00:00Z#up1" {
		t.Errorf("UploadMonthSK() = %q", got)
	}
}
