package application

import (
	"context"
	"time"

	"snap_kakeibo/backend/internal/upload/domain"
)

// UploadHistoryRepository は upload_histories に新しい履歴を登録する。
type UploadHistoryRepository interface {
	// Save は履歴を新規登録する。同じ利用者・upload_id の履歴が既にあれば ErrUploadAlreadyExists を返す。
	Save(ctx context.Context, history domain.UploadHistory) error
}

// UploadURLPresigner はクライアントが元画像を直接 PUT するための署名付き URL を発行する。
// PUT する側は同じ Content-Type を付ける必要がある。
type UploadURLPresigner interface {
	PresignPut(ctx context.Context, key, contentType string, expires time.Duration) (string, error)
}

// IDGenerator は upload_id を採番する。
type IDGenerator interface {
	NewID() (string, error)
}

// UploadRetryMarker は再実行できる履歴を ANALYZING に戻して attempt を進める。
type UploadRetryMarker interface {
	// MarkRetrying は status が domain.RetryableStatuses のいずれかであるときだけ ANALYZING にし、attempt を 1 進めて
	// 前回の失敗情報(error_code / error_message / failed_at)を消す。進めた後の attempt を返す。
	// 履歴が無い、または再実行できない status なら ErrUploadNotRetryable を返す。
	MarkRetrying(ctx context.Context, userID, uploadID string, now time.Time) (attempt int, err error)
}

// AnalyzeJobEnqueuer は解析のやり直しを analyze キューへ送る。
type AnalyzeJobEnqueuer interface {
	EnqueueRetry(ctx context.Context, job domain.RetryJob) error
}
