// Package domain は認証業務に閉じたドメインモデルを提供する。
package domain

import (
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
)

// RefreshToken は発行済み refresh token の永続化対象を表す。
// RawToken は保存せず、Digest だけを PK に使う。
// RevokedAt はゼロ値なら未失効を意味する。
type RefreshToken struct {
	Digest     string
	UserID     common.UserID
	Email      string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt time.Time
	RevokedAt  time.Time
	Hint       string
}

// Revoked は失効済みかを返す。
func (t RefreshToken) Revoked() bool { return !t.RevokedAt.IsZero() }

// Usable は now 時点で refresh に使えるかを返す。
// 期限ちょうどの時刻は期限切れとして扱う。
func (t RefreshToken) Usable(now time.Time) bool {
	return !t.Revoked() && now.Before(t.ExpiresAt)
}
