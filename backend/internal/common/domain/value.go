package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

const (
	dateLayout      = "2006-01-02"
	yearMonthLayout = "2006-01"
	maxAmount       = int64(10_000_000)
	maxQuantity     = int64(999)
)

var (
	// ErrInvalidPurchaseDate は実在する YYYY-MM-DD でない購入日を表す。
	ErrInvalidPurchaseDate = errors.New("purchase date must be a real date in YYYY-MM-DD format")
	// ErrInvalidYearMonth は YYYY-MM でない対象月を表す。
	ErrInvalidYearMonth = errors.New("year month must be YYYY-MM format")
	// ErrInvalidReadAmount は画像解析で読み取れる範囲外の金額を表す。
	ErrInvalidReadAmount = errors.New("read amount must be between 1 and 10000000 yen")
	// ErrInvalidDetailAmount は支出明細で扱える範囲外の金額を表す。
	ErrInvalidDetailAmount = errors.New("detail amount must be between 0 and 10000000 yen")
	// ErrInvalidQuantity は支出明細で扱える範囲外の数量を表す。
	ErrInvalidQuantity = errors.New("quantity must be between 1 and 999")
	// ErrAmountOverflow は読取金額と調整額の加算が int64 の範囲を超えたことを表す。
	ErrAmountOverflow = errors.New("recorded amount overflows int64")
	// ErrInvalidCategory は定義されていないカテゴリを表す。
	ErrInvalidCategory = errors.New("unknown expense category")
	// ErrInvalidCategorySource はカテゴリ決定元が AI と USER のいずれでもないことを表す。
	ErrInvalidCategorySource = errors.New("unknown category source")
)

// PurchaseDate は時刻を含まない実在する購入日。
type PurchaseDate struct{ value time.Time }

// NewPurchaseDate は YYYY-MM-DD 形式の購入日を生成する。
func NewPurchaseDate(value string) (PurchaseDate, error) {
	t, err := time.Parse(dateLayout, value)
	if err != nil || t.Format(dateLayout) != value {
		return PurchaseDate{}, ErrInvalidPurchaseDate
	}
	return PurchaseDate{value: t}, nil
}

// String は YYYY-MM-DD 形式を返す。
func (d PurchaseDate) String() string { return d.value.Format(dateLayout) }

// Time は UTC の日付を返す。
func (d PurchaseDate) Time() time.Time { return d.value }

// YearMonth は購入日が属する対象月を返す。
func (d PurchaseDate) YearMonth() YearMonth { return YearMonth{value: d.value.Format(yearMonthLayout)} }

// YearMonth は暦月を表す値オブジェクト。
type YearMonth struct{ value string }

// NewYearMonth は YYYY-MM 形式の対象月を生成する。
func NewYearMonth(value string) (YearMonth, error) {
	t, err := time.Parse(yearMonthLayout, value)
	if err != nil || t.Format(yearMonthLayout) != value {
		return YearMonth{}, ErrInvalidYearMonth
	}
	return YearMonth{value: value}, nil
}

// String は YYYY-MM 形式を返す。
func (m YearMonth) String() string { return m.value }

// ReadAmount はレシートから読み取った最終的な支払合計。
type ReadAmount struct{ yen int64 }

// NewReadAmount は画像解析で許容する正の読取金額を生成する。
func NewReadAmount(yen int64) (ReadAmount, error) {
	if yen < 1 || yen > maxAmount {
		return ReadAmount{}, ErrInvalidReadAmount
	}
	return ReadAmount{yen: yen}, nil
}

// Yen は円単位の値を返す。
func (a ReadAmount) Yen() int64 { return a.yen }

// AdjustmentAmount は利用者が家計簿上で加減する符号付き金額。
type AdjustmentAmount struct{ yen int64 }

// NewAdjustmentAmount は調整額を生成する。減額は負数、増額は正数で指定する。
func NewAdjustmentAmount(yen int64) AdjustmentAmount { return AdjustmentAmount{yen: yen} }

// Yen は円単位の値を返す。
func (a AdjustmentAmount) Yen() int64 { return a.yen }

// RecordedAmount は月次集計へ反映する符号付きの計上額。
type RecordedAmount struct{ yen int64 }

// NewRecordedAmount は読取金額と調整額から計上額を導出する。
func NewRecordedAmount(read ReadAmount, adjustment AdjustmentAmount) (RecordedAmount, error) {
	if adjustment.yen > 0 && read.yen > math.MaxInt64-adjustment.yen ||
		adjustment.yen < 0 && read.yen < math.MinInt64-adjustment.yen {
		return RecordedAmount{}, ErrAmountOverflow
	}
	return RecordedAmount{yen: read.yen + adjustment.yen}, nil
}

// Yen は円単位の値を返す。
func (a RecordedAmount) Yen() int64 { return a.yen }

// DetailAmount は数量反映後の支出明細1行の金額。
type DetailAmount struct{ yen int64 }

// NewDetailAmount は許容範囲内の明細金額を生成する。
func NewDetailAmount(yen int64) (DetailAmount, error) {
	if yen < 0 || yen > maxAmount {
		return DetailAmount{}, ErrInvalidDetailAmount
	}
	return DetailAmount{yen: yen}, nil
}

// Yen は円単位の値を返す。
func (a DetailAmount) Yen() int64 { return a.yen }

// Quantity は支出明細の数量。
type Quantity struct{ value int64 }

// NewQuantity は許容範囲内の数量を生成する。
func NewQuantity(value int64) (Quantity, error) {
	if value < 1 || value > maxQuantity {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{value: value}, nil
}

// Int64 は数量を返す。
func (q Quantity) Int64() int64 { return q.value }

// Category は支出明細の用途分類。
type Category string

const (
	CategoryFood          Category = "food"
	CategoryDailyGoods    Category = "daily_goods"
	CategoryMedical       Category = "medical"
	CategoryTransport     Category = "transport"
	CategoryUtilities     Category = "utilities"
	CategoryEntertainment Category = "entertainment"
	CategorySocial        Category = "social"
	CategoryClothing      Category = "clothing"
	CategoryEducation     Category = "education"
	CategoryOther         Category = "other"
	CategoryUnknown       Category = "unknown"
)

var categories = [...]Category{
	CategoryFood, CategoryDailyGoods, CategoryMedical, CategoryTransport,
	CategoryUtilities, CategoryEntertainment, CategorySocial, CategoryClothing,
	CategoryEducation, CategoryOther, CategoryUnknown,
}

// NewCategory は定義済みのカテゴリを生成する。
func NewCategory(value string) (Category, error) {
	category := Category(strings.TrimSpace(value))
	for _, candidate := range categories {
		if category == candidate {
			return category, nil
		}
	}
	return "", ErrInvalidCategory
}

// String は保存に使用するカテゴリ名を返す。
func (c Category) String() string { return string(c) }

// Categories は定義済みカテゴリのコピーを返す。
func Categories() []Category { return append([]Category(nil), categories[:]...) }

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
