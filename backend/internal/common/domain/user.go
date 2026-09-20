// Package domain は複数の業務で共有するドメインモデルを提供する。
package domain

import "strings"

// UserID は利用者を一意に識別する値オブジェクト。
type UserID string

// NewUserID は空白を除いた利用者 ID を生成する。空の値は false を返す。
func NewUserID(value string) (UserID, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return UserID(value), true
}

// String は永続化や外部境界で使う文字列表現を返す。
func (id UserID) String() string { return string(id) }

// User は認証済みの利用者を表す共通ドメインモデル。
// PasswordHash は認証のためだけに利用し、外部レスポンスには載せない。
type User struct {
	ID           UserID
	Email        string
	PasswordHash string
}
