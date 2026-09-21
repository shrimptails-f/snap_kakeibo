package infrastructure

// upload_histories のキー構築。analyze-receipt / retry-upload / list-uploads が読み書きする形式と揃える(infra/stacks/storage.go のキー構成)。

// UserPK は利用者単位のパーティションキー。
func UserPK(userID string) string { return "USER#" + userID }

// UploadSK は upload_histories のソートキー。
func UploadSK(uploadID string) string { return "UPLOAD#" + uploadID }

// UploadMonthPK は月ごとに一覧を引くための GSI1 パーティションキー(upload_month_index)。
func UploadMonthPK(userID, month string) string { return "USER#" + userID + "#MONTH#" + month }

// UploadMonthSK は月内で作成順に並べるための GSI1 ソートキー。createdAt は RFC3339(UTC)。
func UploadMonthSK(createdAt, uploadID string) string {
	return "UPLOAD_CREATED_AT#" + createdAt + "#" + uploadID
}
