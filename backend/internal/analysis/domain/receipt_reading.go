// Package domain は画像受付・解析コンテキストの集約、値オブジェクト、不変条件を提供する。
package domain

import (
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
)

// ReceiptReading は画像から読み取った店名・購入日・印字金額候補・税区分・商品行。
// ReadAmount は旧形式の読み取り用で、候補がある場合は候補から選択する。
// OpenAI のレスポンス形式(JSON Schema)は infrastructure が解釈し、ここには載せない。
type ReceiptReading struct {
	StoreName        *string
	PurchaseDate     *string
	ReadAmount       *int64
	AmountCandidates []AmountCandidate
	TaxBreakdown     []TaxBreakdown
	Details          []ReadDetail
}

// ReadDetail は読み取った商品行。
type ReadDetail struct {
	Name     string
	Amount   int64
	Quantity int64
	Category string
	TaxRate  *int64
	TaxMode  string
}

// analysis_requests.error_code に載せる失敗コード。
const (
	FailureNoDate         = "NO_DATE"
	FailureInvalidDate    = "INVALID_DATE"
	FailureNoTotalAmount  = "NO_TOTAL_AMOUNT"
	FailureInvalidAmount  = "INVALID_AMOUNT"
	FailureTooManyDetails = "TOO_MANY_DETAILS"
	FailureAnalysisFailed = "ANALYSIS_FAILED"
	FailureInternal       = "INTERNAL"
)

// ValidateReading の上限値。金額・数量・明細件数の範囲は common/domain の値オブジェクトが持つ。
const (
	maxDetails              = 50
	maxStoreNameRunes       = 100
	maxDetailNameRunes      = 100
	purchaseDateLayout      = "2006-01-02"
	purchaseDateMaxAgeYears = 5
)

// AnalysisFailed は OpenAI の応答を解析結果として採用できなかったときの失敗理由を返す。
func AnalysisFailed(message string) FailureReason { return failure(FailureAnalysisFailed, message) }

// InternalFailure は画像の読み込みや登録など、再解析しても解決しないシステム側の失敗理由を返す。
func InternalFailure(message string) FailureReason { return failure(FailureInternal, message) }

func failure(code, message string) FailureReason {
	return FailureReason{code: code, safeMessage: strings.TrimSpace(message)}
}

// ValidateReading は読み取り内容が支出として自動登録してよいかを検証し、検証済みの解析結果へ変換する。
// 購入日は now の翌日まで・5 年前までを許容する。空の明細名は捨て、語彙にないカテゴリは unknown にする。
// 明細 0 件は失敗ではなく、呼び出し側が NO_DATA と判定できるよう HasData が false の解析結果を返す。
// 失敗は再解析しても解決しない業務上のものだけで、一時的な失敗(ネットワークやレート制限)はここへ来ない。
func ValidateReading(r ReceiptReading, now time.Time) (AnalysisResult, *FailureReason) {
	if r.PurchaseDate == nil {
		return AnalysisResult{}, ptr(failure(FailureNoDate, "購入日を取得できませんでした"))
	}
	d, err := time.ParseInLocation(purchaseDateLayout, *r.PurchaseDate, now.Location())
	if err != nil || d.After(dateOnly(now).AddDate(0, 0, 1)) || d.Before(dateOnly(now).AddDate(-purchaseDateMaxAgeYears, 0, 0)) {
		return AnalysisResult{}, ptr(failure(FailureInvalidDate, "購入日が有効な範囲ではありません"))
	}
	purchaseDate, err := common.NewPurchaseDate(*r.PurchaseDate)
	if err != nil {
		return AnalysisResult{}, ptr(failure(FailureInvalidDate, "購入日が有効な範囲ではありません"))
	}
	evidence := ReconcileAmounts(r)
	var selectedAmount *int64
	if evidence.Selected != nil {
		selectedAmount = &evidence.Selected.Amount
	}
	if selectedAmount == nil {
		return AnalysisResult{}, ptr(failure(FailureNoTotalAmount, "合計金額を取得できませんでした"))
	}
	readAmount, err := common.NewReadAmount(*selectedAmount)
	if err != nil {
		return AnalysisResult{}, ptr(failure(FailureInvalidAmount, "合計金額が有効な範囲ではありません"))
	}
	if len(r.Details) > maxDetails {
		return AnalysisResult{}, ptr(failure(FailureTooManyDetails, "明細件数が50件を超えています"))
	}
	storeName := ""
	if r.StoreName != nil {
		storeName = truncate(*r.StoreName, maxStoreNameRunes)
	}
	details := make([]AnalyzedDetail, 0, len(r.Details))
	for _, d := range r.Details {
		amount, amountErr := common.NewDetailAmount(d.Amount)
		quantity, quantityErr := common.NewQuantity(d.Quantity)
		if amountErr != nil || quantityErr != nil {
			return AnalysisResult{}, ptr(failure(FailureInvalidAmount, "明細の金額または数量が有効な範囲ではありません"))
		}
		name := truncate(strings.TrimSpace(d.Name), maxDetailNameRunes)
		if name == "" {
			continue
		}
		category, err := common.NewCategory(d.Category)
		if err != nil {
			category = common.CategoryUnknown
		}
		detail, err := NewAnalyzedDetail(name, amount, quantity, category)
		if err != nil {
			return AnalysisResult{}, ptr(failure(FailureInvalidAmount, "明細の金額または数量が有効な範囲ではありません"))
		}
		details = append(details, detail)
	}
	result, err := NewAnalysisResult(storeName, purchaseDate, readAmount, details)
	if err != nil {
		return AnalysisResult{}, ptr(failure(FailureInternal, "解析結果を組み立てられませんでした"))
	}
	result.evidence = evidence
	return result, nil
}

func ptr(r FailureReason) *FailureReason { return &r }

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}
