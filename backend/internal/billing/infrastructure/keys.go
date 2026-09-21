package infrastructure

// billings / billing_details のキー構築。analyze-receipt(analysis/infrastructure)が書く形式と揃える(infra/stacks/storage.go のキー構成)。

// UserPK は利用者単位のパーティションキー(billings)。
func UserPK(userID string) string { return "USER#" + userID }

// BillingSK は billings のソートキー。
func BillingSK(billingID string) string { return "BILLING#" + billingID }

// DetailPK は billing_details のパーティションキー。請求単位で明細をまとめる。
func DetailPK(userID, billingID string) string { return "USER#" + userID + "#BILLING#" + billingID }
