package domain

import "strings"

// AnalysisRequestID は解析依頼を一意に識別する値オブジェクト。
type AnalysisRequestID string

// NewAnalysisRequestID は空でない解析依頼 ID を生成する。
func NewAnalysisRequestID(value string) (AnalysisRequestID, bool) {
	value = strings.TrimSpace(value)
	return AnalysisRequestID(value), value != ""
}

// String は文字列表現を返す。
func (id AnalysisRequestID) String() string { return string(id) }

// ExpenseID は支出を一意に識別する値オブジェクト。
type ExpenseID string

// NewExpenseID は空でない支出 ID を生成する。
func NewExpenseID(value string) (ExpenseID, bool) {
	value = strings.TrimSpace(value)
	return ExpenseID(value), value != ""
}

// String は文字列表現を返す。
func (id ExpenseID) String() string { return string(id) }
