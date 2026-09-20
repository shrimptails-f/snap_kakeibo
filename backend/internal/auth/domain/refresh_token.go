// Package domain は認証業務に閉じたドメインモデルを提供する。
package domain

import (
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
)

// RefreshToken は発行済み refresh token の永続化対象を表す。
// RawToken は保存せず、Digest だけを PK に使う。
type RefreshToken struct {
	Digest     string
	UserID     common.UserID
	Email      string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt time.Time
	Hint       string
}
