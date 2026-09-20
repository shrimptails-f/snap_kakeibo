package token

import (
	"context"
	"strings"
	"testing"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTIssuerSetsExpiryFromClock(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	clock := timewrapper.NewFixed(now)
	issuer := JWTIssuer{
		Secrets: StaticSecretProvider{Value: "test-secret"}, Issuer: "snap-kakeibo-dev",
		Clock: clock, TTL: 15 * time.Minute,
	}
	raw, expiresIn, err := issuer.Issue(context.Background(), common.User{ID: common.UserID("user-1"), Email: "member@example.com"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if expiresIn != 900 {
		t.Fatalf("expiresIn = %d, want 900", expiresIn)
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return []byte("test-secret"), nil }, jwt.WithTimeFunc(clock.Now))
	if err != nil || !parsed.Valid {
		t.Fatalf("parse token: %v", err)
	}
	if claims["exp"] != float64(now.Add(15*time.Minute).Unix()) || claims["sub"] != "user-1" {
		t.Errorf("claims = %#v", claims)
	}
}

func TestRefreshTokenGeneratorStoresDigestOnly(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	raw, stored, err := (RefreshTokenGenerator{}).Generate(common.User{ID: common.UserID("user-1"), Email: "member@example.com"}, now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if stored.Digest != Digest(raw) || stored.Digest == raw {
		t.Errorf("digest = %q for raw token %q", stored.Digest, raw)
	}
	if stored.Hint != raw[len(raw)-8:] || !strings.HasSuffix(raw, stored.Hint) {
		t.Errorf("hint = %q", stored.Hint)
	}
}
