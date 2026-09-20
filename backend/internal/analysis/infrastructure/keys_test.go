package infrastructure

import (
	"testing"

	"snap_kakeibo/backend/internal/app"
)

// TestKeysMatchSharedFormat は他の Lambda がまだ使っている app パッケージのキー形式と一致することを確認する。
// app 側が feature package へ移行し終わったらこのテストは外してよい。
func TestKeysMatchSharedFormat(t *testing.T) {
	t.Parallel()
	pairs := map[string][2]string{
		"UserPK":                 {UserPK("u1"), app.UserPK("u1")},
		"UploadSK":               {UploadSK("up1"), app.UploadSK("up1")},
		"BillingSK":              {BillingSK("b1"), app.BillingSK("b1")},
		"MonthSK":                {MonthSK("2026-09"), app.MonthSK("2026-09")},
		"DetailPK":               {DetailPK("u1", "b1"), app.DetailPK("u1", "b1")},
		"DetailSK":               {DetailSK("d1"), app.DetailSK("d1")},
		"UploadMonthPK":          {UploadMonthPK("u1", "2026-09"), app.UploadMonthPK("u1", "2026-09")},
		"DetailMonthSK":          {DetailMonthSK(1234, "2026-09-18", "d1"), app.DetailMonthSK(1234, "2026-09-18", "d1")},
		"DetailMonthSK overflow": {DetailMonthSK(1<<40, "2026-09-18", "d1"), app.DetailMonthSK(1<<40, "2026-09-18", "d1")},
	}
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, app = %q", name, pair[0], pair[1])
		}
	}
	if got := DetailMonthSK(1000, "2026-09-18", "d1"); got != "DETAIL_AMOUNT#2147482647#2026-09-18#d1" {
		t.Errorf("DetailMonthSK() = %q", got)
	}
}
