package infrastructure

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/infrastructure"
	"snap_kakeibo/backend/internal/app"
)

// TestKeysMatchSharedFormat は analyze-receipt と、他の Lambda がまだ使っている app パッケージのキー形式と一致することを確認する。
// app 側が feature package へ移行し終わったら app との比較は外してよい。
func TestKeysMatchSharedFormat(t *testing.T) {
	t.Parallel()
	pairs := map[string][]string{
		"UserPK":        {UserPK("u1"), app.UserPK("u1"), infrastructure.UserPK("u1")},
		"UploadSK":      {UploadSK("up1"), app.UploadSK("up1"), infrastructure.UploadSK("up1")},
		"UploadMonthPK": {UploadMonthPK("u1", "2026-09"), app.UploadMonthPK("u1", "2026-09"), infrastructure.UploadMonthPK("u1", "2026-09")},
		"UploadMonthSK": {UploadMonthSK("2026-09-18T12:00:00Z", "up1"), app.UploadMonthSK("2026-09-18T12:00:00Z", "up1")},
	}
	for name, values := range pairs {
		for _, other := range values[1:] {
			if values[0] != other {
				t.Errorf("%s = %q, shared = %q", name, values[0], other)
			}
		}
	}
}
