package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/library/token"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

func TestLogoutRevokesRefreshToken(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.September, 21, 12, 34, 56, 0, time.UTC)
	revoker := &logoutRevoker{}
	usecase := application.NewLogoutUsecase(token.RefreshTokenGenerator{}, revoker, timewrapper.NewFixed(now))

	if err := usecase.Logout(context.Background(), application.LogoutInput{RefreshToken: " raw-token "}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if revoker.digest != token.Digest("raw-token") {
		t.Errorf("digest = %q, want %q", revoker.digest, token.Digest("raw-token"))
	}
	if !revoker.revokedAt.Equal(now) {
		t.Errorf("revokedAt = %v, want %v", revoker.revokedAt, now)
	}
}

func TestLogoutIsIdempotentForMissingToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		revokeErr error
	}{
		{name: "empty cookie"},
		{name: "unknown token", raw: "unknown", revokeErr: application.ErrRefreshTokenNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			revoker := &logoutRevoker{err: tt.revokeErr}
			usecase := application.NewLogoutUsecase(token.RefreshTokenGenerator{}, revoker, timewrapper.NewFixed(time.Now()))
			if err := usecase.Logout(context.Background(), application.LogoutInput{RefreshToken: tt.raw}); err != nil {
				t.Fatalf("Logout() error = %v", err)
			}
			if tt.raw == "" && revoker.calls != 0 {
				t.Errorf("Revoke() calls = %d, want 0", revoker.calls)
			}
		})
	}
}

func TestLogoutReturnsRepositoryError(t *testing.T) {
	t.Parallel()
	want := errors.New("dynamodb unavailable")
	usecase := application.NewLogoutUsecase(token.RefreshTokenGenerator{}, &logoutRevoker{err: want}, timewrapper.NewFixed(time.Now()))
	if err := usecase.Logout(context.Background(), application.LogoutInput{RefreshToken: "token"}); !errors.Is(err, want) {
		t.Fatalf("Logout() error = %v, want %v", err, want)
	}
}

type logoutRevoker struct {
	digest    string
	revokedAt time.Time
	calls     int
	err       error
}

func (r *logoutRevoker) Revoke(_ context.Context, digest string, revokedAt time.Time) error {
	r.calls++
	r.digest = digest
	r.revokedAt = revokedAt
	return r.err
}
