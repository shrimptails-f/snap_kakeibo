package infrastructure

import (
	"fmt"
	"math"
)

// DynamoDB のキー構築。upload / list-analysis-requests / get-expense が読み書きする形式と揃える(infra/stacks/storage.go のキー構成)。

// UserPK は利用者単位のパーティションキー(analysis_requests / expenses / monthly_summaries)。
func UserPK(userID string) string { return "USER#" + userID }

// AnalysisRequestSK は analysis_requests のソートキー。
func AnalysisRequestSK(requestID string) string { return "ANALYSIS_REQUEST#" + requestID }

// ExpenseSK は expenses のソートキー。
func ExpenseSK(expenseID string) string { return "EXPENSE#" + expenseID }

// MonthSK は monthly_summaries のソートキー。month は YYYY-MM。
func MonthSK(month string) string { return "MONTH#" + month }

// DetailPK は expense_details のパーティションキー。支出単位で支出明細をまとめる。
func DetailPK(userID, expenseID string) string { return "USER#" + userID + "#EXPENSE#" + expenseID }

// DetailSK は expense_details のソートキー。
func DetailSK(detailID string) string { return "DETAIL#" + detailID }

// UserMonthPK は月ごとに引くための GSI1 パーティションキー(expense_details.GSI1PK / analysis_requests.GSI1PK)。
func UserMonthPK(userID, month string) string { return "USER#" + userID + "#MONTH#" + month }

// DetailMonthSK は月内で金額の降順に並べるための GSI1 ソートキー(expense_details.GSI1SK)。
// 金額を MaxInt32 から引いてゼロ埋めすることで、文字列の昇順が金額の降順になる。
func DetailMonthSK(amount int64, purchaseDate, detailID string) string {
	desc := int64(math.MaxInt32) - amount
	if desc < 0 {
		desc = 0
	}
	return fmt.Sprintf("DETAIL_AMOUNT#%010d#%s#%s", desc, purchaseDate, detailID)
}
