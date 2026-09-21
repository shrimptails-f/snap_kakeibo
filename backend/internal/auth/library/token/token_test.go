package token

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"github.com/golang-jwt/jwt/v5"
)

// StaticSecretProvider は固定の署名鍵を返すテスト用の SecretProvider。本番は SSMSecretProvider だけを使う。
type StaticSecretProvider struct{ Value string }

func (p StaticSecretProvider) Secret(context.Context) (string, error) { return p.Value, nil }

func TestJWTIssuerSetsExpiryFromClock(t *testing.T) {
	t.Parallel()
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

func TestJWTVerifierVerifiesIssuedToken(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	clock := timewrapper.NewFixed(now)
	secrets := StaticSecretProvider{Value: "test-secret"}
	issuer := JWTIssuer{Secrets: secrets, Issuer: "snap-kakeibo-test", Clock: clock, TTL: 15 * time.Minute}
	raw, _, err := issuer.Issue(context.Background(), common.User{ID: common.UserID("user-1"), Email: "member@example.com"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	user, err := (JWTVerifier{Secrets: secrets, Issuer: "snap-kakeibo-test", Clock: clock}).Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if user.ID != common.UserID("user-1") || user.Email != "member@example.com" {
		t.Errorf("Verify() = %#v", user)
	}
}

func TestJWTVerifierRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	issuedAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	secrets := StaticSecretProvider{Value: "test-secret"}
	issuer := JWTIssuer{Secrets: secrets, Issuer: "snap-kakeibo-test", Clock: timewrapper.NewFixed(issuedAt), TTL: 15 * time.Minute}
	raw, _, err := issuer.Issue(context.Background(), common.User{ID: common.UserID("user-1")})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	_, err = (JWTVerifier{Secrets: secrets, Issuer: "snap-kakeibo-test", Clock: timewrapper.NewFixed(issuedAt.Add(16 * time.Minute))}).Verify(context.Background(), raw)
	if !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("Verify() error = %v, want ErrUnauthorized", err)
	}
}

func TestRefreshTokenGeneratorStoresDigestOnly(t *testing.T) {
	t.Parallel()
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

func TestRefreshTokenGeneratorDigestMatchesStoredDigest(t *testing.T) {
	t.Parallel()
	generator := RefreshTokenGenerator{}
	raw, stored, err := generator.Generate(common.User{ID: common.UserID("user-1")}, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got := generator.Digest(raw); got != stored.Digest {
		t.Errorf("Digest(raw) = %q, want stored digest %q", got, stored.Digest)
	}
	if generator.Digest(raw+"x") == stored.Digest {
		t.Error("Digest() of a different token matches the stored digest")
	}
}
