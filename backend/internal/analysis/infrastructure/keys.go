package infrastructure

import (
	"fmt"
	"math"
)

// DynamoDB のキー構築。upload / list-uploads / get-billing が読み書きする形式と揃える(infra/stacks/storage.go のキー構成)。

// UserPK は利用者単位のパーティションキー(upload_histories / billings / monthly_summaries)。
func UserPK(userID string) string { return "USER#" + userID }

// UploadSK は upload_histories のソートキー。
func UploadSK(uploadID string) string { return "UPLOAD#" + uploadID }

// BillingSK は billings のソートキー。
func BillingSK(billingID string) string { return "BILLING#" + billingID }

// MonthSK は monthly_summaries のソートキー。month は YYYY-MM。
func MonthSK(month string) string { return "MONTH#" + month }

// DetailPK は billing_details のパーティションキー。請求単位で明細をまとめる。
func DetailPK(userID, billingID string) string { return "USER#" + userID + "#BILLING#" + billingID }

// DetailSK は billing_details のソートキー。
func DetailSK(detailID string) string { return "DETAIL#" + detailID }

// UploadMonthPK は月ごとに引くための GSI1 パーティションキー(billing_details.GSI1PK)。
func UploadMonthPK(userID, month string) string { return "USER#" + userID + "#MONTH#" + month }

// DetailMonthSK は月内で金額の降順に並べるための GSI1 ソートキー(billing_details.GSI1SK)。
// 金額を MaxInt32 から引いてゼロ埋めすることで、文字列の昇順が金額の降順になる。
func DetailMonthSK(amount int64, purchasedAt, detailID string) string {
	desc := int64(math.MaxInt32) - amount
	if desc < 0 {
		desc = 0
	}
	return fmt.Sprintf("DETAIL_AMOUNT#%010d#%s#%s", desc, purchasedAt, detailID)
}
