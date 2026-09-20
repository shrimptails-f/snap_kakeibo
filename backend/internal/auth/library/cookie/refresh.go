// Package cookie は認証 Cookie の HTTP 表現を提供する。
package cookie

import "net/http"

const RefreshTokenName = "refresh_token"

// Refresh は refresh token を送る Set-Cookie ヘッダ値を返す。
func Refresh(value string, maxAge int) string {
	return (&http.Cookie{Name: RefreshTokenName, Value: value, Path: "/api/auth", MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}).String()
}

// ReadRefresh は API Gateway が分解して渡す Cookie ヘッダ群から refresh token を取り出す。
// 見つからなければ空文字を返す。
func ReadRefresh(cookies []string) string {
	for _, raw := range cookies {
		request := http.Request{Header: http.Header{"Cookie": []string{raw}}}
		for _, c := range request.Cookies() {
			if c.Name == RefreshTokenName {
				return c.Value
			}
		}
	}
	return ""
}
