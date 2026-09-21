// Package domain はレシート画像アップロードのドメインモデルを提供する。
package domain

import (
	"errors"
	"strings"
	"time"
)

// Status は upload_histories.status の値。upload が書くのは UPLOADING だけで、以降は analyze-receipt / retry-upload が遷移させる。
type Status string

// StatusUploading はクライアントが署名付き URL へ PUT する前の初期状態。
const StatusUploading Status = "UPLOADING"

// 未指定のときに使う既定値。フロントエンドは file_name / content_type を省略できる。
const (
	DefaultFileName    = "receipt.jpg"
	DefaultContentType = "image/jpeg"
)

// FirstAttempt は最初の解析の attempt。retry-upload が 1 ずつ増やす。
const FirstAttempt = 1

// ErrInvalidUploadHistory は識別子が欠けた履歴を作ろうとしたときに返す。
var ErrInvalidUploadHistory = errors.New("upload history requires user_id and upload_id")

// UploadHistory は upload_histories に登録するアップロード 1 件。
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

// YearMonth は t を UTC の YYYY-MM にする。
func YearMonth(t time.Time) string { return t.UTC().Format("2006-01") }
