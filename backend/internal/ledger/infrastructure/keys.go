package infrastructure

import (
	"fmt"
	"math"
)

// expenses / expense_details のキー構築。analyze-receipt(analysis/infrastructure)が書く形式と揃える(infra/stacks/storage.go のキー構成)。

// UserPK は利用者単位のパーティションキー(expenses)。
func UserPK(userID string) string { return "USER#" + userID }

// ExpenseSK は expenses のソートキー。
func ExpenseSK(expenseID string) string { return "EXPENSE#" + expenseID }

// DetailPK は expense_details のパーティションキー。支出単位で支出明細をまとめる。
func DetailPK(userID, expenseID string) string { return "USER#" + userID + "#EXPENSE#" + expenseID }

func DetailSK(detailID string) string { return "DETAIL#" + detailID }

func UserMonthPK(userID, month string) string { return "USER#" + userID + "#MONTH#" + month }

func MonthSK(month string) string { return "MONTH#" + month }

func DetailMonthSK(amount int64, purchaseDate, detailID string) string {
	desc := int64(math.MaxInt32) - amount
	if desc < 0 {
		desc = 0
	}
	return fmt.Sprintf("DETAIL_AMOUNT#%010d#%s#%s", desc, purchaseDate, detailID)
}
