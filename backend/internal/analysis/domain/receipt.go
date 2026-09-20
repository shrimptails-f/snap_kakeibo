// Package domain はレシート解析の entity、value object、不変条件を提供する。
package domain

import (
	"strings"
	"time"
)

// Categories は明細の固定カテゴリ。OpenAI の JSON Schema の enum と Validate の両方で使う。
var Categories = []string{"food", "daily_goods", "medical", "transport", "utilities", "entertainment", "clothing", "education", "other", "unknown"}

// CategoryUnknown は語彙にないカテゴリを置き換える値。
const CategoryUnknown = "unknown"

// Receipt は OpenAI が画像から読み取ったレシート。読めなかった項目は nil。
type Receipt struct {
	StoreName   *string  `json:"store_name"`
	PurchasedAt *string  `json:"purchased_at"`
	TotalAmount *int64   `json:"total_amount"`
	Details     []Detail `json:"details"`
}

// Detail はレシートの商品行。
type Detail struct {
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Quantity int64  `json:"quantity"`
	Category string `json:"category"`
}

// upload_histories.error_code に載せる失敗コード。
const (
	FailureNoDate         = "NO_DATE"
	FailureInvalidDate    = "INVALID_DATE"
	FailureNoTotalAmount  = "NO_TOTAL_AMOUNT"
	FailureInvalidAmount  = "INVALID_AMOUNT"
	FailureTooManyDetails = "TOO_MANY_DETAILS"
	FailureAnalysisFailed = "ANALYSIS_FAILED"
	FailureInternal       = "INTERNAL"
)

// Validate の上限値。
const (
	maxDetails             = 50
	maxStoreNameRunes      = 100
	maxDetailNameRunes     = 100
	maxAmount              = 10_000_000
	maxQuantity            = 999
	purchasedAtLayout      = "2006-01-02"
	purchasedAtMaxAgeYears = 5
)

// Failure は解析を再試行しても解決しない業務上の失敗。upload_histories に error_code / error_message として記録する。
// 一時的な失敗(ネットワークやレート制限)は Failure ではなく error として返し、キューの再配信に任せる。
type Failure struct {
	Code    string
	Message string
}

func (f *Failure) Error() string { return f.Code + ": " + f.Message }

// Validate はレシートの不変条件を確認し、保存できる形に正規化して返す。
// 購入日は now の翌日まで・5 年前までを許容する。空の明細名は捨て、語彙にないカテゴリは unknown にする。
func Validate(r Receipt, now time.Time) (Receipt, *Failure) {
	if r.PurchasedAt == nil {
		return r, &Failure{Code: FailureNoDate, Message: "購入日を取得できませんでした"}
	}
	d, err := time.ParseInLocation(purchasedAtLayout, *r.PurchasedAt, now.Location())
	if err != nil || d.After(dateOnly(now).AddDate(0, 0, 1)) || d.Before(dateOnly(now).AddDate(-purchasedAtMaxAgeYears, 0, 0)) {
		return r, &Failure{Code: FailureInvalidDate, Message: "購入日が有効な範囲ではありません"}
	}
	if r.TotalAmount == nil {
		return r, &Failure{Code: FailureNoTotalAmount, Message: "合計金額を取得できませんでした"}
	}
	if *r.TotalAmount < 1 || *r.TotalAmount > maxAmount {
		return r, &Failure{Code: FailureInvalidAmount, Message: "合計金額が有効な範囲ではありません"}
	}
	if len(r.Details) > maxDetails {
		return r, &Failure{Code: FailureTooManyDetails, Message: "明細件数が50件を超えています"}
	}
	if r.StoreName != nil {
		s := truncate(*r.StoreName, maxStoreNameRunes)
		r.StoreName = &s
	}
	valid := make([]Detail, 0, len(r.Details))
	for _, d := range r.Details {
		if d.Amount < 0 || d.Amount > maxAmount || d.Quantity < 1 || d.Quantity > maxQuantity {
			return r, &Failure{Code: FailureInvalidAmount, Message: "明細の金額または数量が有効な範囲ではありません"}
		}
		d.Name = truncate(strings.TrimSpace(d.Name), maxDetailNameRunes)
		if d.Name == "" {
			continue
		}
		if !ValidCategory(d.Category) {
			d.Category = CategoryUnknown
		}
		valid = append(valid, d)
	}
	r.Details = valid
	return r, nil
}

// ValidCategory は s が Categories に含まれるなら true。
func ValidCategory(s string) bool {
	for _, c := range Categories {
		if s == c {
			return true
		}
	}
	return false
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}
