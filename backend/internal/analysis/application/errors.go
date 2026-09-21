package application

import "errors"

// ErrExpenseRejected は支出の登録内容が DynamoDB の制約(項目サイズや式の不正)に触れて拒否された場合に
// ExpenseRegistrar が返す。再解析しても解決しないので、usecase は解析依頼を INTERNAL の失敗として記録する。
var ErrExpenseRejected = errors.New("expense registration rejected")
