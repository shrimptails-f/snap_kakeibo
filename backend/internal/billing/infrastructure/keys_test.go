package infrastructure

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/infrastructure"
)

// TestKeysMatchAnalysisFormat は請求を書く analyze-receipt のキー形式と一致することを確認する。
func TestKeysMatchAnalysisFormat(t *testing.T) {
	t.Parallel()
	pairs := map[string][2]string{
		"UserPK":    {UserPK("u1"), infrastructure.UserPK("u1")},
		"BillingSK": {BillingSK("b1"), infrastructure.BillingSK("b1")},
		"DetailPK":  {DetailPK("u1", "b1"), infrastructure.DetailPK("u1", "b1")},
	}
	for name, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, analysis = %q", name, pair[0], pair[1])
		}
	}
}
