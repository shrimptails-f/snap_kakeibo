package cookie

import (
	"net/http"
	"testing"
)

func TestRefreshCookie(t *testing.T) {
	parsed, err := http.ParseSetCookie(Refresh("token", 3600))
	if err != nil {
		t.Fatalf("ParseSetCookie() error = %v", err)
	}
	if parsed.Name != RefreshTokenName || parsed.Value != "token" || parsed.Path != "/api/auth" || !parsed.HttpOnly || !parsed.Secure || parsed.SameSite != http.SameSiteLaxMode || parsed.MaxAge != 3600 {
		t.Errorf("cookie = %#v", parsed)
	}
}

func TestReadRefresh(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cookies []string
		want    string
	}{
		{name: "single cookie", cookies: []string{"refresh_token=abc"}, want: "abc"},
		{name: "multiple cookies in one header", cookies: []string{"session=1; refresh_token=abc; other=2"}, want: "abc"},
		{name: "multiple headers", cookies: []string{"session=1", "refresh_token=abc"}, want: "abc"},
		{name: "missing", cookies: []string{"session=1"}, want: ""},
		{name: "no cookies", cookies: nil, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadRefresh(tt.cookies); got != tt.want {
				t.Errorf("ReadRefresh() = %q, want %q", got, tt.want)
			}
		})
	}
}
