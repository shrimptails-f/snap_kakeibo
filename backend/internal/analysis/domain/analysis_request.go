package domain

import (
	"errors"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
)

type (
	// AnalysisRequestID は解析依頼を識別する共有ID。
	AnalysisRequestID = common.AnalysisRequestID
	// ExpenseID は登録した支出を識別する共有ID。
	ExpenseID = common.ExpenseID
	// PurchaseDate は解析結果と支出で共有する購入日。
	PurchaseDate = common.PurchaseDate
	// ReadAmount は解析結果と支出で共有する読取金額。
	ReadAmount = common.ReadAmount
	// DetailAmount は解析結果と支出で共有する明細金額。
	DetailAmount = common.DetailAmount
	// Quantity は解析結果と支出で共有する数量。
	Quantity = common.Quantity
	// Category は解析結果と支出で共有するカテゴリ。
	Category = common.Category
)

var (
	// ErrInvalidAnalysisRequest は解析依頼の必須項目が不足していることを表す。
	ErrInvalidAnalysisRequest = errors.New("analysis request has invalid required fields")
	// ErrAttemptMismatch は指定した試行番号が解析依頼の現在値と異なることを表す。
	ErrAttemptMismatch = errors.New("analysis attempt does not match current attempt")
	// ErrInvalidAnalysisTransition は現在の状態から要求された状態へ遷移できないことを表す。
	ErrInvalidAnalysisTransition = errors.New("invalid analysis status transition")
	// ErrRetryPolicyUndecided は解析中の依頼に対する再解析条件が未決定であることを表す。
	ErrRetryPolicyUndecided = errors.New("retry policy for an analyzing request is undecided")
	// ErrInvalidAnalysisResult は検証済み解析結果の必須項目または明細数が不正なことを表す。
	ErrInvalidAnalysisResult = errors.New("analysis result is invalid")
	// ErrInvalidAnalyzedDetail は解析明細の名前が空であることを表す。
	ErrInvalidAnalyzedDetail = errors.New("analyzed detail name is required")
	// ErrInvalidReceiptImage はレシート画像の参照が空であることを表す。
	ErrInvalidReceiptImage = errors.New("receipt image reference is required")
	// ErrInvalidFailureReason は失敗コードが空であることを表す。
	ErrInvalidFailureReason = errors.New("failure code is required")
)

// Attempt は1から始まる解析試行番号。
type Attempt struct{ value int }

// NewAttempt は正の試行番号を生成する。
func NewAttempt(value int) (Attempt, bool) { return Attempt{value: value}, value >= 1 }

// Int は試行番号を返す。
func (a Attempt) Int() int { return a.value }

// Next は次の解析試行番号を返す。
func (a Attempt) Next() Attempt { return Attempt{value: a.value + 1} }

// Matches は同じ解析試行を表すかを返す。
func (a Attempt) Matches(other Attempt) bool { return a == other }

// AnalysisStatus は解析依頼の状態。
type AnalysisStatus string

const (
	AnalysisStatusUploading AnalysisStatus = "UPLOADING"
	AnalysisStatusAnalyzing AnalysisStatus = "ANALYZING"
	AnalysisStatusSucceeded AnalysisStatus = "SUCCEEDED"
	AnalysisStatusNoData    AnalysisStatus = "NO_DATA"
	AnalysisStatusFailed    AnalysisStatus = "FAILED"
)

// ReceiptImage は解析対象となる画像の参照と表示情報。
type ReceiptImage struct {
	reference   string
	fileName    string
	contentType string
}

// NewReceiptImage は保存先への参照を持つレシート画像を生成する。
func NewReceiptImage(reference, fileName, contentType string) (ReceiptImage, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ReceiptImage{}, ErrInvalidReceiptImage
	}
	return ReceiptImage{reference: reference, fileName: strings.TrimSpace(fileName), contentType: strings.TrimSpace(contentType)}, nil
}

// Reference は画像保存先への参照を返す。
func (i ReceiptImage) Reference() string { return i.reference }

// FileName は利用者が送信したファイル名を返す。
func (i ReceiptImage) FileName() string { return i.fileName }

// ContentType は画像のContent-Typeを返す。
func (i ReceiptImage) ContentType() string { return i.contentType }

// FailureReason は解析を終端失敗にする安全な理由。
type FailureReason struct {
	code        string
	safeMessage string
}

// NewFailureReason は空でない失敗コードから理由を生成する。
func NewFailureReason(code, safeMessage string) (FailureReason, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return FailureReason{}, ErrInvalidFailureReason
	}
	return FailureReason{code: code, safeMessage: strings.TrimSpace(safeMessage)}, nil
}

// Code は失敗コードを返す。
func (r FailureReason) Code() string { return r.code }

// SafeMessage は利用者へ表示可能な説明を返す。
func (r FailureReason) SafeMessage() string { return r.safeMessage }

// AnalysisRequest はレシート画像の受付から解析の終端までを管理する集約ルート。
type AnalysisRequest struct {
	id              AnalysisRequestID
	userID          common.UserID
	image           ReceiptImage
	status          AnalysisStatus
	currentAttempt  Attempt
	uploadExpiresAt time.Time
	expenseID       ExpenseID
	failureReason   *FailureReason
	failedAt        time.Time
	createdAt       time.Time
	updatedAt       time.Time
}

// AnalysisRequestState は永続化された解析依頼を復元するためのドメイン状態。
// DynamoDBなど特定の永続化方式には依存しない。
type AnalysisRequestState struct {
	ID              AnalysisRequestID
	UserID          common.UserID
	Image           ReceiptImage
	Status          AnalysisStatus
	CurrentAttempt  Attempt
	UploadExpiresAt time.Time
	ExpenseID       ExpenseID
	FailureReason   *FailureReason
	FailedAt        time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewAnalysisRequest はアップロード待ちの解析依頼を生成する。
func NewAnalysisRequest(id AnalysisRequestID, userID common.UserID, image ReceiptImage, uploadExpiresAt, now time.Time) (AnalysisRequest, error) {
	if id == "" || userID == "" || image.reference == "" || uploadExpiresAt.IsZero() || now.IsZero() {
		return AnalysisRequest{}, ErrInvalidAnalysisRequest
	}
	return AnalysisRequest{id: id, userID: userID, image: image, status: AnalysisStatusUploading, currentAttempt: Attempt{value: 1}, uploadExpiresAt: uploadExpiresAt, createdAt: now, updatedAt: now}, nil
}

// RestoreAnalysisRequest は永続化された状態を検証して解析依頼を復元する。
func RestoreAnalysisRequest(state AnalysisRequestState) (AnalysisRequest, error) {
	if state.ID == "" || state.UserID == "" || state.Image.reference == "" || state.CurrentAttempt.value < 1 ||
		state.UploadExpiresAt.IsZero() || state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() || !validAnalysisStatus(state.Status) {
		return AnalysisRequest{}, ErrInvalidAnalysisRequest
	}
	if state.Status == AnalysisStatusSucceeded && state.ExpenseID == "" {
		return AnalysisRequest{}, ErrInvalidAnalysisRequest
	}
	if state.Status == AnalysisStatusFailed && (state.FailureReason == nil || state.FailureReason.code == "" || state.FailedAt.IsZero()) {
		return AnalysisRequest{}, ErrInvalidAnalysisRequest
	}
	if state.Status != AnalysisStatusSucceeded && state.ExpenseID != "" ||
		state.Status != AnalysisStatusFailed && (state.FailureReason != nil || !state.FailedAt.IsZero()) {
		return AnalysisRequest{}, ErrInvalidAnalysisRequest
	}
	var reason *FailureReason
	if state.FailureReason != nil {
		copy := *state.FailureReason
		reason = &copy
	}
	return AnalysisRequest{
		id: state.ID, userID: state.UserID, image: state.Image, status: state.Status,
		currentAttempt: state.CurrentAttempt, uploadExpiresAt: state.UploadExpiresAt,
		expenseID: state.ExpenseID, failureReason: reason, failedAt: state.FailedAt,
		createdAt: state.CreatedAt, updatedAt: state.UpdatedAt,
	}, nil
}

func validAnalysisStatus(status AnalysisStatus) bool {
	switch status {
	case AnalysisStatusUploading, AnalysisStatusAnalyzing, AnalysisStatusSucceeded, AnalysisStatusNoData, AnalysisStatusFailed:
		return true
	default:
		return false
	}
}

// StartAnalysis は一致する試行を解析中にする。同一試行の再配信は解析中から再開できる。
func (r *AnalysisRequest) StartAnalysis(attempt Attempt, now time.Time) error {
	if !r.currentAttempt.Matches(attempt) {
		return ErrAttemptMismatch
	}
	if r.status != AnalysisStatusUploading && r.status != AnalysisStatusAnalyzing {
		return ErrInvalidAnalysisTransition
	}
	r.status, r.updatedAt = AnalysisStatusAnalyzing, now
	return nil
}

// Complete は一致する解析中の試行を登録完了にする。
func (r *AnalysisRequest) Complete(attempt Attempt, expenseID ExpenseID, now time.Time) error {
	if expenseID == "" {
		return ErrInvalidAnalysisRequest
	}
	if err := r.ensureAnalyzing(attempt); err != nil {
		return err
	}
	r.status, r.expenseID, r.failureReason, r.failedAt, r.updatedAt = AnalysisStatusSucceeded, expenseID, nil, time.Time{}, now
	return nil
}

// MarkNoData は一致する解析中の試行を登録対象なしにする。
func (r *AnalysisRequest) MarkNoData(attempt Attempt, now time.Time) error {
	if err := r.ensureAnalyzing(attempt); err != nil {
		return err
	}
	r.status, r.expenseID, r.failureReason, r.failedAt, r.updatedAt = AnalysisStatusNoData, "", nil, time.Time{}, now
	return nil
}

// Fail は一致する解析中の試行を理由付きの解析失敗にする。
func (r *AnalysisRequest) Fail(attempt Attempt, reason FailureReason, now time.Time) error {
	if reason.code == "" {
		return ErrInvalidFailureReason
	}
	if err := r.ensureAnalyzing(attempt); err != nil {
		return err
	}
	r.status, r.expenseID, r.failureReason, r.failedAt, r.updatedAt = AnalysisStatusFailed, "", &reason, now, now
	return nil
}

// Retry は終端した失敗または登録対象なしの依頼に対して次の解析試行を開始する。
// 解析中の依頼を再解析できる条件は未決定のため、現時点では受け付けない。
func (r *AnalysisRequest) Retry(now time.Time) error {
	if r.status == AnalysisStatusAnalyzing {
		return ErrRetryPolicyUndecided
	}
	if r.status != AnalysisStatusFailed && r.status != AnalysisStatusNoData {
		return ErrInvalidAnalysisTransition
	}
	r.currentAttempt = r.currentAttempt.Next()
	r.status, r.expenseID, r.failureReason, r.failedAt, r.updatedAt = AnalysisStatusAnalyzing, "", nil, time.Time{}, now
	return nil
}

func (r AnalysisRequest) ensureAnalyzing(attempt Attempt) error {
	if !r.currentAttempt.Matches(attempt) {
		return ErrAttemptMismatch
	}
	if r.status != AnalysisStatusAnalyzing {
		return ErrInvalidAnalysisTransition
	}
	return nil
}

// ID は解析依頼IDを返す。
func (r AnalysisRequest) ID() AnalysisRequestID { return r.id }

// UserID は所有者の利用者IDを返す。
func (r AnalysisRequest) UserID() common.UserID { return r.userID }

// Image は解析対象画像を返す。
func (r AnalysisRequest) Image() ReceiptImage { return r.image }

// Status は現在の解析状態を返す。
func (r AnalysisRequest) Status() AnalysisStatus { return r.status }

// CurrentAttempt は現在の試行番号を返す。
func (r AnalysisRequest) CurrentAttempt() Attempt { return r.currentAttempt }

// UploadExpiresAt は画像アップロード期限を返す。
func (r AnalysisRequest) UploadExpiresAt() time.Time { return r.uploadExpiresAt }

// ExpenseID は登録完了時に作成された支出IDを返す。
func (r AnalysisRequest) ExpenseID() ExpenseID { return r.expenseID }

// FailureReason は解析失敗理由と、その有無を返す。
func (r AnalysisRequest) FailureReason() (FailureReason, bool) {
	if r.failureReason == nil {
		return FailureReason{}, false
	}
	return *r.failureReason, true
}

// FailedAt は解析が失敗した日時と、その有無を返す。
func (r AnalysisRequest) FailedAt() (time.Time, bool) { return r.failedAt, !r.failedAt.IsZero() }

// CreatedAt は作成日時を返す。
func (r AnalysisRequest) CreatedAt() time.Time { return r.createdAt }

// UpdatedAt は最終更新日時を返す。
func (r AnalysisRequest) UpdatedAt() time.Time { return r.updatedAt }

// AnalyzedDetail は検証済み解析結果の商品行。
type AnalyzedDetail struct {
	name     string
	amount   DetailAmount
	quantity Quantity
	category Category
}

// NewAnalyzedDetail は名前が空でない解析明細を生成する。
func NewAnalyzedDetail(name string, amount DetailAmount, quantity Quantity, category Category) (AnalyzedDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" || !quantity.Valid() || !amount.Valid() {
		return AnalyzedDetail{}, ErrInvalidAnalyzedDetail
	}
	if _, err := common.NewCategory(category.String()); err != nil {
		return AnalyzedDetail{}, err
	}
	return AnalyzedDetail{name: name, amount: amount, quantity: quantity, category: category}, nil
}

// Name は明細名を返す。
func (d AnalyzedDetail) Name() string { return d.name }

// Amount は明細金額を返す。
func (d AnalyzedDetail) Amount() DetailAmount { return d.amount }

// Quantity は数量を返す。
func (d AnalyzedDetail) Quantity() Quantity { return d.quantity }

// Category はカテゴリを返す。
func (d AnalyzedDetail) Category() Category { return d.category }

// AnalysisResult は外部レスポンスから変換された検証済み解析結果。
type AnalysisResult struct {
	storeName    string
	purchaseDate PurchaseDate
	readAmount   ReadAmount
	details      []AnalyzedDetail
	evidence     AmountEvidence
}

// NewAnalysisResult は支出へ変換可能な解析結果を生成する。明細0件はNO_DATA判定のため許容する。
func NewAnalysisResult(storeName string, purchaseDate PurchaseDate, readAmount ReadAmount, details []AnalyzedDetail) (AnalysisResult, error) {
	if !purchaseDate.Valid() || !readAmount.Valid() || len(details) > 50 {
		return AnalysisResult{}, ErrInvalidAnalysisResult
	}
	return AnalysisResult{storeName: strings.TrimSpace(storeName), purchaseDate: purchaseDate, readAmount: readAmount, details: append([]AnalyzedDetail(nil), details...)}, nil
}

// StoreName は店名を返す。
func (r AnalysisResult) StoreName() string { return r.storeName }

// PurchaseDate は購入日を返す。
func (r AnalysisResult) PurchaseDate() PurchaseDate { return r.purchaseDate }

// ReadAmount は読取金額を返す。
func (r AnalysisResult) ReadAmount() ReadAmount { return r.readAmount }

// Evidence は支払合計の選択根拠を返す。
func (r AnalysisResult) Evidence() AmountEvidence { return r.evidence }

// Details は解析明細のコピーを返す。
func (r AnalysisResult) Details() []AnalyzedDetail {
	return append([]AnalyzedDetail(nil), r.details...)
}

// HasData は支出へ登録する明細が1件以上あるかを返す。
func (r AnalysisResult) HasData() bool { return len(r.details) > 0 }
