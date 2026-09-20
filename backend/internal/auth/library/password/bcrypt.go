// Package password はパスワードハッシュの比較を提供する。
package password

import "golang.org/x/crypto/bcrypt"

// Bcrypt は bcrypt を使った PasswordComparator 実装。
type Bcrypt struct{}

// Compare は password と bcrypt hash を照合する。
func (Bcrypt) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
