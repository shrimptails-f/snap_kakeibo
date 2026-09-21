package infrastructure

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/infrastructure"
	"snap_kakeibo/backend/internal/app"
)

// TestKeysMatchSharedFormat は請求を書く analyze-receipt と、まだ app パッケージを使っている箇所のキー形式と一致することを確認する。
// app 側が feature package へ移行し終わったら app との比較は外してよい。
func TestKeysMatchSharedFormat(t *testing.T) {
	t.Parallel()
	pairs := map[string][]string{
		"UserPK":    {UserPK("u1"), app.UserPK("u1"), infrastructure.UserPK("u1")},
		"BillingSK": {BillingSK("b1"), app.BillingSK("b1"), infrastructure.BillingSK("b1")},
		"DetailPK":  {DetailPK("u1", "b1"), app.DetailPK("u1", "b1"), infrastructure.DetailPK("u1", "b1")},
	}
	for name, values := range pairs {
		for _, other := range values[1:] {
			if values[0] != other {
				t.Errorf("%s = %q, shared = %q", name, values[0], other)
			}
		}
	}
}
