package domain

import "time"

// Billing は検証済みのレシートから作る請求。明細の ID と作成時刻は usecase が採番する。
type Billing struct {
	ID          string
	UserID      string
	UploadID    string
	StoreName   string
	PurchasedAt string // YYYY-MM-DD。Validate を通ったレシートの購入日
	TotalAmount int64
	Details     []BillingDetail
	CreatedAt   time.Time
}

// BillingDetail は請求の明細 1 行。
type BillingDetail struct {
	ID       string
	Name     string
	Category string
	Amount   int64
	Quantity int64
}

// YearMonth は購入日の年月(YYYY-MM)。月次集計のキーに使う。
func (b Billing) YearMonth() string { return b.PurchasedAt[:len("2006-01")] }

// CategoryTotals はカテゴリごとの明細金額の合計。月次集計の category_total_<category> に加算する。
func (b Billing) CategoryTotals() map[string]int64 {
	totals := map[string]int64{}
	for _, d := range b.Details {
		totals[d.Category] += d.Amount
	}
	return totals
}
