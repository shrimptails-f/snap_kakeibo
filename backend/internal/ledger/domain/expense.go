// Package domain は家計簿の支出集約と月次読み取りモデルを提供する。
package domain

import (
	"errors"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
)

type (
	// ExpenseID は支出を識別する共有ID。
	ExpenseID = common.ExpenseID
	// AnalysisRequestID は支出の登録元を識別する共有ID。
	AnalysisRequestID = common.AnalysisRequestID
	// PurchaseDate は解析結果と支出で共有する購入日。
	PurchaseDate = common.PurchaseDate
	// ReadAmount は解析結果と支出で共有する読取金額。
	ReadAmount = common.ReadAmount
	// DetailAmount は支出明細の金額。
	DetailAmount = common.DetailAmount
	// Quantity は支出明細の数量。
	Quantity = common.Quantity
	// Category は支出明細の用途分類。
	Category = common.Category
	// YearMonth は支出と月次集計で共有する対象月。
	YearMonth = common.YearMonth
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
	id                ExpenseDetailID
	name              string
	amount            DetailAmount
	taxIncludedAmount *int64
	taxRate           *int64
	taxMode           string
	taxAllocation     string
	quantity          Quantity
	category          Category
	categorySource    CategorySource
	source            RecordSource
	edited            bool
}

// NewExpenseDetail は不変条件を満たす支出明細を生成する。
func NewExpenseDetail(id ExpenseDetailID, name string, amount DetailAmount, quantity Quantity, category Category, source CategorySource) (ExpenseDetail, error) {
	name = strings.TrimSpace(name)
	if id == "" || name == "" || !quantity.Valid() || !amount.Valid() {
		return ExpenseDetail{}, ErrInvalidExpenseDetail
	}
	if _, err := common.NewCategory(category.String()); err != nil {
		return ExpenseDetail{}, err
	}
	if _, err := NewCategorySource(source.String()); err != nil {
		return ExpenseDetail{}, err
	}
	return ExpenseDetail{id: id, name: name, amount: amount, quantity: quantity, category: category, categorySource: source, source: RecordSourceAI}, nil
}

// RestoreExpenseDetail は永続化された作成元と編集状態を含めて支出明細を復元する。
func RestoreExpenseDetail(id ExpenseDetailID, name string, amount DetailAmount, quantity Quantity, category Category, categorySource CategorySource, source RecordSource, edited bool) (ExpenseDetail, error) {
	detail, err := NewExpenseDetail(id, name, amount, quantity, category, categorySource)
	if err != nil {
		return ExpenseDetail{}, err
	}
	if _, err := NewRecordSource(source.String()); err != nil {
		return ExpenseDetail{}, err
	}
	detail.source, detail.edited = source, edited
	return detail, nil
}

// ID は支出明細IDを返す。
func (d ExpenseDetail) ID() ExpenseDetailID { return d.id }

// Name は明細名を返す。
func (d ExpenseDetail) Name() string { return d.name }

// Amount は数量反映後の明細金額を返す。
func (d ExpenseDetail) Amount() DetailAmount { return d.amount }

// TaxIncludedAmount は根拠を確認できた税込み明細額を返す。nil は未確定を表す。
func (d ExpenseDetail) TaxIncludedAmount() *int64 { return copyInt64(d.taxIncludedAmount) }

// TaxRate は確認済みの商品別税率を返す。
func (d ExpenseDetail) TaxRate() *int64 { return copyInt64(d.taxRate) }

// TaxMode は商品行の内外税区分を返す。
func (d ExpenseDetail) TaxMode() string { return d.taxMode }

// TaxAllocation は税込み額の算出根拠を返す。
func (d ExpenseDetail) TaxAllocation() string { return d.taxAllocation }

// ReportingAmount は内訳に使う金額。未確定なら印字額を返す。
func (d ExpenseDetail) ReportingAmount() int64 {
	if d.taxIncludedAmount != nil {
		return *d.taxIncludedAmount
	}
	return d.amount.Yen()
}

// SetTaxEvidence はレシートに基づく税情報を設定する。未確定の行では税込み額を渡さない。
func (d *ExpenseDetail) SetTaxEvidence(rate *int64, mode string, included *int64, allocation string) error {
	if included != nil && (*included < d.amount.Yen() || *included > 10_000_000 || (mode != "included" && mode != "external")) {
		return ErrInvalidExpenseDetail
	}
	if mode != "" && mode != "included" && mode != "external" && mode != "mixed" && mode != "unknown" {
		return ErrInvalidExpenseDetail
	}
	if rate != nil && (*rate <= 0 || *rate > 100) {
		return ErrInvalidExpenseDetail
	}
	d.taxRate, d.taxMode, d.taxIncludedAmount, d.taxAllocation = copyInt64(rate), mode, copyInt64(included), allocation
	return nil
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// Quantity は数量を返す。
func (d ExpenseDetail) Quantity() Quantity { return d.quantity }

// Category はカテゴリを返す。
func (d ExpenseDetail) Category() Category { return d.category }

// CategorySource はカテゴリ決定元を返す。
func (d ExpenseDetail) CategorySource() CategorySource { return d.categorySource }

// Source は明細の作成元を返す。
func (d ExpenseDetail) Source() RecordSource { return d.source }

// Edited は明細が利用者によって編集済みかを返す。
func (d ExpenseDetail) Edited() bool { return d.edited }

// Expense は家計へ金額上の影響を与える1件の支出を管理する集約ルート。
type Expense struct {
	id               ExpenseID
	userID           common.UserID
	sourceRequestID  AnalysisRequestID
	storeName        string
	purchaseDate     PurchaseDate
	readAmount       ReadAmount
	analysisEvidence string
	adjustment       AdjustmentAmount
	recordedAmount   RecordedAmount
	details          []ExpenseDetail
	edited           bool
	source           RecordSource
	updatedAt        time.Time
}

// ExpenseState は永続化された支出を復元するためのドメイン状態。
// RecordedAmountは読取金額と調整額から再導出し、永続値を正として受け取らない。
type ExpenseState struct {
	ID               ExpenseID
	UserID           common.UserID
	SourceRequestID  AnalysisRequestID
	StoreName        string
	PurchaseDate     PurchaseDate
	ReadAmount       ReadAmount
	AnalysisEvidence string
	Adjustment       AdjustmentAmount
	Details          []ExpenseDetail
	Edited           bool
	Source           RecordSource
	UpdatedAt        time.Time
}

// NewExpense は検証済み解析結果などから支出を生成する。
func NewExpense(id ExpenseID, userID common.UserID, sourceRequestID AnalysisRequestID, storeName string, purchaseDate PurchaseDate, readAmount ReadAmount, details []ExpenseDetail) (Expense, error) {
	return restoreExpense(ExpenseState{ID: id, UserID: userID, SourceRequestID: sourceRequestID, StoreName: storeName, PurchaseDate: purchaseDate, ReadAmount: readAmount, Adjustment: NewAdjustmentAmount(0), Details: details, Source: RecordSourceAI})
}

// RestoreExpense は永続化された状態を検証して支出を復元する。
func RestoreExpense(state ExpenseState) (Expense, error) { return restoreExpense(state) }

func restoreExpense(state ExpenseState) (Expense, error) {
	if state.ID == "" || state.UserID == "" || !state.PurchaseDate.Valid() || !state.ReadAmount.Valid() || len(state.Details) < 1 || len(state.Details) > 50 {
		return Expense{}, ErrInvalidExpense
	}
	if hasDuplicateDetailID(state.Details) {
		return Expense{}, ErrDuplicateExpenseDetail
	}
	if _, err := NewRecordSource(state.Source.String()); err != nil {
		return Expense{}, err
	}
	for _, detail := range state.Details {
		if detail.id == "" || strings.TrimSpace(detail.name) == "" || !detail.amount.Valid() || !detail.quantity.Valid() {
			return Expense{}, ErrInvalidExpenseDetail
		}
		if _, err := common.NewCategory(detail.category.String()); err != nil {
			return Expense{}, err
		}
		if _, err := NewCategorySource(detail.categorySource.String()); err != nil {
			return Expense{}, err
		}
	}
	recorded, err := NewRecordedAmount(state.ReadAmount, state.Adjustment)
	if err != nil {
		return Expense{}, err
	}
	return Expense{
		id: state.ID, userID: state.UserID, sourceRequestID: state.SourceRequestID,
		storeName: strings.TrimSpace(state.StoreName), purchaseDate: state.PurchaseDate,
		readAmount: state.ReadAmount, adjustment: state.Adjustment, recordedAmount: recorded,
		analysisEvidence: state.AnalysisEvidence,
		details:          append([]ExpenseDetail(nil), state.Details...), edited: state.Edited, source: state.Source, updatedAt: state.UpdatedAt,
	}, nil
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
	e.storeName, e.source, e.edited = strings.TrimSpace(name), RecordSourceUser, true
}

// ChangePurchaseDate は購入日を変更し、支出を編集済みにする。
func (e *Expense) ChangePurchaseDate(date PurchaseDate) error {
	if !date.Valid() {
		return common.ErrInvalidPurchaseDate
	}
	e.purchaseDate, e.source, e.edited = date, RecordSourceUser, true
	return nil
}

// AdjustAmount は符号付き調整額を変更し、計上額を再計算する。
func (e *Expense) AdjustAmount(adjustment AdjustmentAmount) error {
	recorded, err := NewRecordedAmount(e.readAmount, adjustment)
	if err != nil {
		return err
	}
	e.adjustment, e.recordedAmount, e.source, e.edited = adjustment, recorded, RecordSourceUser, true
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
	detail.name, detail.source, detail.edited, e.source, e.edited = name, RecordSourceUser, true, RecordSourceUser, true
	return nil
}

// ChangeDetailAmount は支出明細の金額と数量を変更する。
func (e *Expense) ChangeDetailAmount(id ExpenseDetailID, amount DetailAmount, quantity Quantity) error {
	if !amount.Valid() || !quantity.Valid() {
		return ErrInvalidExpenseDetail
	}
	detail, err := e.detail(id)
	if err != nil {
		return err
	}
	if detail.amount != amount {
		detail.taxIncludedAmount, detail.taxRate, detail.taxMode, detail.taxAllocation = nil, nil, "unknown", ""
	}
	detail.amount, detail.quantity, detail.source, detail.edited, e.source, e.edited = amount, quantity, RecordSourceUser, true, RecordSourceUser, true
	return nil
}

// ChangeDetailCategory は支出明細のカテゴリを利用者指定へ変更する。
func (e *Expense) ChangeDetailCategory(id ExpenseDetailID, category Category) error {
	if _, err := common.NewCategory(category.String()); err != nil {
		return err
	}
	detail, err := e.detail(id)
	if err != nil {
		return err
	}
	detail.category, detail.categorySource, detail.source, detail.edited, e.source, e.edited = category, CategorySourceUser, RecordSourceUser, true, RecordSourceUser, true
	return nil
}

// AddDetail は利用者が入力した明細を支出へ追加する。
func (e *Expense) AddDetail(detail ExpenseDetail) error {
	if len(e.details) >= 50 || detail.ID() == "" {
		return ErrInvalidExpenseDetail
	}
	if _, err := e.detail(detail.ID()); err == nil {
		return ErrDuplicateExpenseDetail
	}
	detail.source, detail.categorySource, detail.edited = RecordSourceUser, CategorySourceUser, true
	e.details = append(e.details, detail)
	e.source, e.edited = RecordSourceUser, true
	return nil
}

// RemoveDetail は指定した明細を削除する。操作後の件数は集約を保存する前に検証する。
func (e *Expense) RemoveDetail(id ExpenseDetailID) error {
	for index, detail := range e.details {
		if detail.ID() == id {
			e.details = append(e.details[:index:index], e.details[index+1:]...)
			e.source, e.edited = RecordSourceUser, true
			return nil
		}
	}
	return ErrExpenseDetailNotFound
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
func (e Expense) UserID() common.UserID { return e.userID }

// SourceRequestID は元となった解析依頼IDを返す。手入力では空を許容する。
func (e Expense) SourceRequestID() AnalysisRequestID { return e.sourceRequestID }

// StoreName は店名を返す。
func (e Expense) StoreName() string { return e.storeName }

// PurchaseDate は購入日を返す。
func (e Expense) PurchaseDate() PurchaseDate { return e.purchaseDate }

// ReadAmount は読取金額を返す。
func (e Expense) ReadAmount() ReadAmount { return e.readAmount }

// AnalysisEvidence は解析時に採用した金額の根拠を JSON で返す。手入力と旧データでは空。
func (e Expense) AnalysisEvidence() string { return e.analysisEvidence }

// SetAnalysisEvidence は解析済み支出の登録前に金額の根拠を保持する。
func (e *Expense) SetAnalysisEvidence(evidence string) { e.analysisEvidence = evidence }

// AdjustmentAmount は符号付き調整額を返す。
func (e Expense) AdjustmentAmount() AdjustmentAmount { return e.adjustment }

// RecordedAmount は月次集計へ反映する計上額を返す。
func (e Expense) RecordedAmount() RecordedAmount { return e.recordedAmount }

// Details は支出明細のコピーを返す。
func (e Expense) Details() []ExpenseDetail { return append([]ExpenseDetail(nil), e.details...) }

// Edited は利用者によって編集済みかを返す。
func (e Expense) Edited() bool { return e.edited }

// Source は支出の作成元を返す。
func (e Expense) Source() RecordSource { return e.source }

// UpdatedAt は支出の最終更新日時を返す。
func (e Expense) UpdatedAt() time.Time { return e.updatedAt }
