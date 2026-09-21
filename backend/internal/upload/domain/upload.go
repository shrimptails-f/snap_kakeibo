// Package domain はレシート画像アップロードのドメインモデルを提供する。
// 解析依頼の集約(AnalysisRequest)は画像受付・解析コンテキストで共有するため analysis/domain に置き、
// ここには受付時の既定値、元画像の置き場所、再解析の条件だけを持つ。
package domain

import (
	"errors"
	"strings"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	common "snap_kakeibo/backend/internal/common/domain"
)

type (
	// AnalysisRequest は upload が登録し、retry-analysis / list-analysis-requests が扱う解析依頼の集約。
	AnalysisRequest = analysisdomain.AnalysisRequest
	// AnalysisRequestState は永続化した解析依頼を復元するための状態。
	AnalysisRequestState = analysisdomain.AnalysisRequestState
	// AnalysisStatus は解析依頼の状態。
	AnalysisStatus = analysisdomain.AnalysisStatus
	// ReceiptImage は解析対象となるレシート画像の参照。
	ReceiptImage = analysisdomain.ReceiptImage
)

// 未指定のときに使う既定値。フロントエンドは file_name / content_type を省略できる。
const (
	DefaultFileName    = "receipt.jpg"
	DefaultContentType = "image/jpeg"
)

// FirstAttempt は最初の解析試行番号。retry-analysis が 1 ずつ増やす。
const FirstAttempt = 1

// ErrInvalidUpload は利用者または解析依頼の識別子が欠けていて解析依頼を作れないときに返す。
var ErrInvalidUpload = errors.New("upload requires user_id and analysis_request_id")

// RetryableStatuses は retry-analysis が再解析できる状態。
// ANALYZING を含むのは、前回の処理が停滞したまま終端に至らない場合にも利用者がやり直せるようにするため。
// UPLOADING は PUT が終わっていないので対象外。
var RetryableStatuses = []AnalysisStatus{analysisdomain.AnalysisStatusFailed, analysisdomain.AnalysisStatusNoData, analysisdomain.AnalysisStatusAnalyzing}

// Retryable は status が再解析できる状態なら true。
func Retryable(status AnalysisStatus) bool {
	for _, candidate := range RetryableStatuses {
		if status == candidate {
			return true
		}
	}
	return false
}

// NewUploadRequest はアップロード待ちの解析依頼を作る。
// fileName / contentType が空なら既定値を使い、S3 キーは ObjectKey で決める。アップロード期限は now + urlTTL。
func NewUploadRequest(userID, requestID, fileName, contentType string, now time.Time, urlTTL time.Duration) (AnalysisRequest, error) {
	user, ok := common.NewUserID(userID)
	if !ok {
		return AnalysisRequest{}, ErrInvalidUpload
	}
	id, ok := common.NewAnalysisRequestID(requestID)
	if !ok {
		return AnalysisRequest{}, ErrInvalidUpload
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = DefaultFileName
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = DefaultContentType
	}
	image, err := analysisdomain.NewReceiptImage(ObjectKey(user.String(), id.String()), fileName, contentType)
	if err != nil {
		return AnalysisRequest{}, err
	}
	now = now.UTC()
	return analysisdomain.NewAnalysisRequest(id, user, image, now.Add(urlTTL), now)
}

// ObjectKey は元画像の S3 キー。infra の PutObject 権限(receipts/*)と analyze-receipt の S3 通知の解釈(IDsFromKey)に合わせる。
func ObjectKey(userID, requestID string) string {
	return "receipts/" + userID + "/" + requestID + "/original.jpg"
}

// yearMonthLayout は YearMonth の形式(YYYY-MM)。
const yearMonthLayout = "2006-01"

// YearMonth は t を UTC の YYYY-MM にする。一覧が月ごとに引くためのキーで、解析依頼の作成日時から決める。
func YearMonth(t time.Time) string { return t.UTC().Format(yearMonthLayout) }
