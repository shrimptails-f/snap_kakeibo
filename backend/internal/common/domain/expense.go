package domain

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidExpense は支出の必須項目または明細数が不正なことを表す。
	ErrInvalidExpense = errors.New("expense is invalid")
	// ErrInvalidExpenseDetail は支出明細の必須項目が不正なことを表す。
	ErrInvalidExpenseDetail = errors.New("expense detail is invalid")
	// ErrDuplicateExpenseDetail は同じ支出内で明細IDが重複していることを表す。
	ErrDuplicateExpenseDetail = errors.New("expense detail ID is duplicated")
	// ErrExpenseDetailNotFound は指定した支出明細が存在しないことを表す。
	ErrExpenseDetailNotFound = errors.New("expense detail was not found")
)

// ExpenseDetail は支出に含まれる商品またはサービスの1行。
type ExpenseDetail struct {
	id             ExpenseDetailID
	name           string
	amount         DetailAmount
	quantity       Quantity
	category       Category
	categorySource CategorySource
}

// NewExpenseDetail は不変条件を満たす支出明細を生成する。
func NewExpenseDetail(id ExpenseDetailID, name string, amount DetailAmount, quantity Quantity, category Category, source CategorySource) (ExpenseDetail, error) {
	name = strings.TrimSpace(name)
	if id == "" || name == "" || quantity.value < 1 || quantity.value > maxQuantity || amount.yen < 0 || amount.yen > maxAmount {
		return ExpenseDetail{}, ErrInvalidExpenseDetail
	}
	if _, err := NewCategory(category.String()); err != nil {
		return ExpenseDetail{}, err
	}
	if _, err := NewCategorySource(source.String()); err != nil {
		return ExpenseDetail{}, err
	}
	return ExpenseDetail{id: id, name: name, amount: amount, quantity: quantity, category: category, categorySource: source}, nil
}

// ID は支出明細IDを返す。
func (d ExpenseDetail) ID() ExpenseDetailID { return d.id }

// Name は明細名を返す。
func (d ExpenseDetail) Name() string { return d.name }

// Amount は数量反映後の明細金額を返す。
func (d ExpenseDetail) Amount() DetailAmount { return d.amount }

// Quantity は数量を返す。
func (d ExpenseDetail) Quantity() Quantity { return d.quantity }

// Category はカテゴリを返す。
func (d ExpenseDetail) Category() Category { return d.category }

// CategorySource はカテゴリ決定元を返す。
func (d ExpenseDetail) CategorySource() CategorySource { return d.categorySource }

// Expense は家計へ金額上の影響を与える1件の支出を管理する集約ルート。
type Expense struct {
	id              ExpenseID
	userID          UserID
	sourceRequestID AnalysisRequestID
	storeName       string
	purchasedAt     PurchaseDate
	readAmount      ReadAmount
	adjustment      AdjustmentAmount
	recordedAmount  RecordedAmount
	details         []ExpenseDetail
	edited          bool
}

// NewExpense は検証済み解析結果などから支出を生成する。
func NewExpense(id ExpenseID, userID UserID, sourceRequestID AnalysisRequestID, storeName string, purchasedAt PurchaseDate, readAmount ReadAmount, details []ExpenseDetail) (Expense, error) {
	if id == "" || userID == "" || purchasedAt.value.IsZero() || readAmount.yen == 0 || len(details) < 1 || len(details) > 50 {
		return Expense{}, ErrInvalidExpense
	}
	if hasDuplicateDetailID(details) {
		return Expense{}, ErrDuplicateExpenseDetail
	}
	adjustment := NewAdjustmentAmount(0)
	recorded, err := NewRecordedAmount(readAmount, adjustment)
	if err != nil {
		return Expense{}, err
	}
	return Expense{id: id, userID: userID, sourceRequestID: sourceRequestID, storeName: strings.TrimSpace(storeName), purchasedAt: purchasedAt, readAmount: readAmount, adjustment: adjustment, recordedAmount: recorded, details: append([]ExpenseDetail(nil), details...)}, nil
}

func hasDuplicateDetailID(details []ExpenseDetail) bool {
	seen := make(map[ExpenseDetailID]struct{}, len(details))
	for _, detail := range details {
		if detail.id == "" {
			return true
		}
		if _, ok := seen[detail.id]; ok {
			return true
		}
		seen[detail.id] = struct{}{}
	}
	return false
}

// ChangeStoreName は店名を変更し、支出を編集済みにする。
func (e *Expense) ChangeStoreName(name string) {
	e.storeName, e.edited = strings.TrimSpace(name), true
}

// ChangePurchaseDate は購入日を変更し、支出を編集済みにする。
func (e *Expense) ChangePurchaseDate(date PurchaseDate) error {
	if date.value.IsZero() {
		return ErrInvalidPurchaseDate
	}
	e.purchasedAt, e.edited = date, true
	return nil
}

// AdjustAmount は符号付き調整額を変更し、計上額を再計算する。
func (e *Expense) AdjustAmount(adjustment AdjustmentAmount) error {
	recorded, err := NewRecordedAmount(e.readAmount, adjustment)
	if err != nil {
		return err
	}
	e.adjustment, e.recordedAmount, e.edited = adjustment, recorded, true
	return nil
}

// RenameDetail は支出明細の名前を変更する。
func (e *Expense) RenameDetail(id ExpenseDetailID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidExpenseDetail
	}
	detail, err := e.detail(id)
	if err != nil {
		return err
	}
	detail.name, e.edited = name, true
	return nil
}

// ChangeDetailAmount は支出明細の金額と数量を変更する。
func (e *Expense) ChangeDetailAmount(id ExpenseDetailID, amount DetailAmount, quantity Quantity) error {
	detail, err := e.detail(id)
	if err != nil {
		return err
	}
	detail.amount, detail.quantity, e.edited = amount, quantity, true
	return nil
}

// ChangeDetailCategory は支出明細のカテゴリを利用者指定へ変更する。
func (e *Expense) ChangeDetailCategory(id ExpenseDetailID, category Category) error {
	if _, err := NewCategory(category.String()); err != nil {
		return err
	}
	detail, err := e.detail(id)
	if err != nil {
		return err
	}
	detail.category, detail.categorySource, e.edited = category, CategorySourceUser, true
	return nil
}

func (e *Expense) detail(id ExpenseDetailID) (*ExpenseDetail, error) {
	for index := range e.details {
		if e.details[index].id == id {
			return &e.details[index], nil
		}
	}
	return nil, ErrExpenseDetailNotFound
}

// ID は支出IDを返す。
func (e Expense) ID() ExpenseID { return e.id }

// UserID は所有者の利用者IDを返す。
func (e Expense) UserID() UserID { return e.userID }

// SourceRequestID は元となった解析依頼IDを返す。手入力では空を許容する。
func (e Expense) SourceRequestID() AnalysisRequestID { return e.sourceRequestID }

// StoreName は店名を返す。
func (e Expense) StoreName() string { return e.storeName }

// PurchasedAt は購入日を返す。
func (e Expense) PurchasedAt() PurchaseDate { return e.purchasedAt }

// ReadAmount は読取金額を返す。
func (e Expense) ReadAmount() ReadAmount { return e.readAmount }

// AdjustmentAmount は符号付き調整額を返す。
func (e Expense) AdjustmentAmount() AdjustmentAmount { return e.adjustment }

// RecordedAmount は月次集計へ反映する計上額を返す。
func (e Expense) RecordedAmount() RecordedAmount { return e.recordedAmount }

// Details は支出明細のコピーを返す。
func (e Expense) Details() []ExpenseDetail { return append([]ExpenseDetail(nil), e.details...) }

// Edited は利用者によって編集済みかを返す。
func (e Expense) Edited() bool { return e.edited }
