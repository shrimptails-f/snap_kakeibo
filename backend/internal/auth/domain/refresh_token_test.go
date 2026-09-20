package domain

import (
	"testing"
	"time"
)

func TestRefreshTokenUsable(t *testing.T) {
	t.Parallel()
	expiresAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name  string
		token RefreshToken
		now   time.Time
		want  bool
	}{
		{name: "before expiry", token: RefreshToken{ExpiresAt: expiresAt}, now: expiresAt.Add(-time.Second), want: true},
		{name: "exactly at expiry", token: RefreshToken{ExpiresAt: expiresAt}, now: expiresAt, want: false},
		{name: "after expiry", token: RefreshToken{ExpiresAt: expiresAt}, now: expiresAt.Add(time.Second), want: false},
		{name: "revoked", token: RefreshToken{ExpiresAt: expiresAt, RevokedAt: expiresAt.Add(-time.Hour)}, now: expiresAt.Add(-time.Second), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.token.Usable(tt.now); got != tt.want {
				t.Errorf("Usable() = %v, want %v", got, tt.want)
			}
		})
	}
}
