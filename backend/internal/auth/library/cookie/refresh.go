// Package cookie は認証 Cookie の HTTP 表現を提供する。
package cookie

import "net/http"

const RefreshTokenName = "refresh_token"

// Refresh は refresh token を送る Set-Cookie ヘッダ値を返す。
func Refresh(value string, maxAge int) string {
	return (&http.Cookie{Name: RefreshTokenName, Value: value, Path: "/api/auth", MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}).String()
}
