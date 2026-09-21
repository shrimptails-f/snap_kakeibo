package domain

import (
	"errors"
	"math"
	"strings"

	common "snap_kakeibo/backend/internal/common/domain"
)

var (
	// ErrAmountOverflow は読取金額と調整額の加算が int64 の範囲を超えたことを表す。
	ErrAmountOverflow = errors.New("recorded amount overflows int64")
	// ErrInvalidCategorySource はカテゴリ決定元が AI と USER のいずれでもないことを表す。
	ErrInvalidCategorySource = errors.New("unknown category source")
)

// ExpenseDetailID は支出明細を一意に識別する値オブジェクト。
type ExpenseDetailID string

// NewExpenseDetailID は空でない支出明細IDを生成する。
func NewExpenseDetailID(value string) (ExpenseDetailID, bool) {
	value = strings.TrimSpace(value)
	return ExpenseDetailID(value), value != ""
}

// String は文字列表現を返す。
func (id ExpenseDetailID) String() string { return string(id) }

// AdjustmentAmount は利用者が家計簿上で加減する符号付き金額。
type AdjustmentAmount struct{ yen int64 }

// NewAdjustmentAmount は調整額を生成する。減額は負数、増額は正数で指定する。
func NewAdjustmentAmount(yen int64) AdjustmentAmount { return AdjustmentAmount{yen: yen} }

// Yen は円単位の値を返す。
func (a AdjustmentAmount) Yen() int64 { return a.yen }

// RecordedAmount は月次集計へ反映する符号付きの計上額。
type RecordedAmount struct{ yen int64 }

// NewRecordedAmount は読取金額と調整額から計上額を導出する。
func NewRecordedAmount(read common.ReadAmount, adjustment AdjustmentAmount) (RecordedAmount, error) {
	if adjustment.yen > 0 && read.Yen() > math.MaxInt64-adjustment.yen ||
		adjustment.yen < 0 && read.Yen() < math.MinInt64-adjustment.yen {
		return RecordedAmount{}, ErrAmountOverflow
	}
	return RecordedAmount{yen: read.Yen() + adjustment.yen}, nil
}

// Yen は円単位の値を返す。
func (a RecordedAmount) Yen() int64 { return a.yen }

// CategorySource はカテゴリを最後に決めた主体。
type CategorySource string

const (
	CategorySourceAI   CategorySource = "AI"
	CategorySourceUser CategorySource = "USER"
)

// NewCategorySource は定義済みのカテゴリ決定元を生成する。
func NewCategorySource(value string) (CategorySource, error) {
	source := CategorySource(value)
	if source != CategorySourceAI && source != CategorySourceUser {
		return "", ErrInvalidCategorySource
	}
	return source, nil
}

// String は保存に使用するカテゴリ決定元を返す。
func (s CategorySource) String() string { return string(s) }
