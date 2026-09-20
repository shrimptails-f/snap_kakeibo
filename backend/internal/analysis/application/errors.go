package application

import "errors"

// ErrBillingRejected は請求の登録内容が DynamoDB の制約(項目サイズや式の不正)に触れて拒否された場合に
// BillingRegistrar が返す。再試行しても解決しないので、usecase はアップロードを INTERNAL の失敗として記録する。
var ErrBillingRejected = errors.New("billing registration rejected")
