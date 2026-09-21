package infrastructure

// analysis_requests のキー構築。analyze-receipt / retry-analysis / list-analysis-requests が読み書きする形式と揃える(infra/stacks/storage.go のキー構成)。

// UserPK は利用者単位のパーティションキー。
func UserPK(userID string) string { return "USER#" + userID }

// AnalysisRequestSK は analysis_requests のソートキー。
func AnalysisRequestSK(requestID string) string { return "ANALYSIS_REQUEST#" + requestID }

// UserMonthPK は月ごとに一覧を引くための GSI1 パーティションキー(analysis_request_month_index)。
func UserMonthPK(userID, month string) string { return "USER#" + userID + "#MONTH#" + month }

// AnalysisRequestMonthSK は月内で作成順に並べるための GSI1 ソートキー。createdAt は RFC3339(UTC)。
func AnalysisRequestMonthSK(createdAt, requestID string) string {
	return "ANALYSIS_REQUEST_CREATED_AT#" + createdAt + "#" + requestID
}
