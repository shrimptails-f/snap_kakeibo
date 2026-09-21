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
