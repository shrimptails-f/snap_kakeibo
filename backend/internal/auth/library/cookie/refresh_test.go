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
