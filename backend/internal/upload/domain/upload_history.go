// Package domain はレシート画像アップロードのドメインモデルを提供する。
package domain

import (
	"errors"
	"strings"
	"time"
)

// Status は upload_histories.status の値。upload が書くのは UPLOADING だけで、以降は analyze-receipt / retry-upload が遷移させる。
type Status string

const (
	// StatusUploading はクライアントが署名付き URL へ PUT する前の初期状態。
	StatusUploading Status = "UPLOADING"
	// StatusAnalyzing は analyze-receipt が処理中(または retry-upload が再投入した直後)の状態。
	StatusAnalyzing Status = "ANALYZING"
	// StatusSucceeded は解析が完了し billings を登録した終端状態。再実行できない。
	StatusSucceeded Status = "SUCCEEDED"
	// StatusFailed は解析が失敗した終端状態。error_code / error_message / failed_at を持つ。
	StatusFailed Status = "FAILED"
	// StatusNoData はレシートとして読めず billings を作らなかった終端状態。
	StatusNoData Status = "NO_DATA"
)

// RetryableStatuses は retry-upload が解析をやり直せる status。
// ANALYZING を含むのは、前回の処理が停滞したまま終端に至らない場合にも利用者がやり直せるようにするため。
// UPLOADING は PUT が終わっていないので対象外。
var RetryableStatuses = []Status{StatusFailed, StatusNoData, StatusAnalyzing}

// Retryable は s が再実行できる status なら true。
func (s Status) Retryable() bool {
	for _, status := range RetryableStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// 未指定のときに使う既定値。フロントエンドは file_name / content_type を省略できる。
const (
	DefaultFileName    = "receipt.jpg"
	DefaultContentType = "image/jpeg"
)

// FirstAttempt は最初の解析の attempt。retry-upload が 1 ずつ増やす。
const FirstAttempt = 1

// ErrInvalidUploadHistory は識別子が欠けた履歴を作ろうとしたときに返す。
var ErrInvalidUploadHistory = errors.New("upload history requires user_id and upload_id")

// UploadHistory は upload_histories のアップロード 1 件。
// upload が登録するのは UPLOADING の初期状態で、BillingID / ErrorCode / ErrorMessage は analyze-receipt が
// SUCCEEDED / FAILED に遷移させるときに書く(list-uploads はそれを読んで返す)。
// ExpiresAt は署名付き URL の期限で、クライアントはそれまでに PUT を終える必要がある。
type UploadHistory struct {
	UserID      string
	UploadID    string
	Status      Status
	Attempt     int
	S3Key       string
	FileName    string
	ContentType string
	// YearMonth は一覧が月ごとに引くための YYYY-MM(UTC)。
	YearMonth string
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	// BillingID は SUCCEEDED のときだけ入る。
	BillingID string
	// ErrorCode / ErrorMessage は FAILED のときだけ入る。retry-upload が ANALYZING に戻すときに消える。
	ErrorCode    string
	ErrorMessage string
}

// NewUploadHistory は UPLOADING の初期状態の履歴を作る。
// fileName / contentType が空なら既定値を使い、S3 キーは ObjectKey で決める。
func NewUploadHistory(userID, uploadID, fileName, contentType string, now time.Time, urlTTL time.Duration) (UploadHistory, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(uploadID) == "" {
		return UploadHistory{}, ErrInvalidUploadHistory
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = DefaultFileName
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = DefaultContentType
	}
	now = now.UTC()
	return UploadHistory{
		UserID:      userID,
		UploadID:    uploadID,
		Status:      StatusUploading,
		Attempt:     FirstAttempt,
		S3Key:       ObjectKey(userID, uploadID),
		FileName:    fileName,
		ContentType: contentType,
		YearMonth:   YearMonth(now),
		ExpiresAt:   now.Add(urlTTL),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// ObjectKey は元画像の S3 キー。infra の PutObject 権限(receipts/*)と analyze-receipt の S3 通知の解釈(idsFromKey)に合わせる。
func ObjectKey(userID, uploadID string) string {
	return "receipts/" + userID + "/" + uploadID + "/original.jpg"
}

// yearMonthLayout は YearMonth の形式(YYYY-MM)。
const yearMonthLayout = "2006-01"

// ErrInvalidYearMonth は YYYY-MM でない月を指定したときに返す。
var ErrInvalidYearMonth = errors.New("year month must be YYYY-MM")

// YearMonth は t を UTC の YYYY-MM にする。
func YearMonth(t time.Time) string { return t.UTC().Format(yearMonthLayout) }

// ValidateYearMonth は s が YearMonth と同じ YYYY-MM(月は 2 桁、01〜12)であることを確認する。
// 一覧は s をそのまま GSI のキーに使うので、形式が違えば空の結果になる前に入力の誤りとして弾く。
func ValidateYearMonth(s string) error {
	if _, err := time.Parse(yearMonthLayout, s); err != nil {
		return ErrInvalidYearMonth
	}
	return nil
}
