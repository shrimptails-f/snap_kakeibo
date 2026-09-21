// Package domain は請求参照のドメインモデルを提供する。
// 請求は analyze-receipt(internal/analysis)が登録し、ここではそれを画面へ返すための read model を持つ。
package domain

// Billing は billings に登録された請求 1 件。金額は円の整数。
// PurchasedAt は YYYY-MM-DD、YearMonth はその年月(YYYY-MM)で、いずれも登録時に analyze-receipt が決めた値をそのまま持つ。
type Billing struct {
	ID          string
	UserID      string
	UploadID    string
	StoreName   string
	PurchasedAt string
	YearMonth   string
	// OriginalAmount は割引前、DiscountAmount は割引、FinalAmount は支払額。AI 解析の登録では Original = Final、Discount = 0。
	OriginalAmount int64
	DiscountAmount int64
	FinalAmount    int64
}

// BillingDetail は請求の明細 1 行。
type BillingDetail struct {
	ID       string
	Name     string
	Category string
	// CategorySource はカテゴリを決めた主体(AI 解析なら AI)。画面で編集できるようになったら USER が入る。
	CategorySource string
	Amount         int64
	Quantity       int64
}
