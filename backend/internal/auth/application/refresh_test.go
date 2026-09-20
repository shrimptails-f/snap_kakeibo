package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	authdomain "snap_kakeibo/backend/internal/auth/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

func TestRefreshRotatesTokens(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	user := common.User{ID: common.UserID("user-1"), Email: "member@example.com", PasswordHash: "hash"}
	stored := authdomain.RefreshToken{Digest: "digest:old", UserID: user.ID, Email: user.Email, ExpiresAt: now.Add(time.Hour)}
	repository := &users{user: user}
	store := &storedRefreshTokens{tokens: map[string]authdomain.RefreshToken{stored.Digest: stored}}
	saved := &refreshTokens{}
	usecase := application.RefreshUsecase{
		Users:                  repository,
		AccessTokens:           accessTokens{},
		Digester:               digester{},
		RefreshTokenFinder:     store,
		RefreshTokenRevoker:    store,
		RefreshTokens:          generatedRefreshToken{},
		RefreshTokenRepository: saved,
		Clock:                  timewrapper.NewFixed(now),
		RefreshTokenTTL:        30 * 24 * time.Hour,
	}

	out, err := usecase.Refresh(context.Background(), application.RefreshInput{RefreshToken: " old "})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if store.found != "digest:old" {
		t.Errorf("looked up digest = %q, want digest of trimmed raw token", store.found)
	}
	if repository.email != user.Email {
		t.Errorf("repository email = %q", repository.email)
	}
	if store.revoked != "digest:old" || !store.revokedAt.Equal(now) {
		t.Errorf("revoked = %q at %v, want digest:old at %v", store.revoked, store.revokedAt, now)
	}
	if out.Tokens.AccessToken != "access" || out.Tokens.RefreshToken != "refresh" || out.Tokens.ExpiresIn != 900 {
		t.Errorf("unexpected tokens: %#v", out.Tokens)
	}
	if out.Tokens.RefreshTokenExpiresIn != int64((30*24*time.Hour)/time.Second) {
		t.Errorf("refresh token expires in = %d", out.Tokens.RefreshTokenExpiresIn)
	}
	if saved.saved.UserID != user.ID || !saved.saved.ExpiresAt.Equal(now.Add(30*24*time.Hour)) {
		t.Errorf("saved refresh token = %#v", saved.saved)
	}
	if out.User != user {
		t.Errorf("user = %#v", out.User)
	}
}

func TestRefreshRejectsUnusableTokens(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	user := common.User{ID: common.UserID("user-1"), Email: "member@example.com", PasswordHash: "hash"}
	valid := authdomain.RefreshToken{Digest: "digest:token", UserID: user.ID, Email: user.Email, ExpiresAt: now.Add(time.Hour)}
	for _, tt := range []struct {
		name   string
		raw    string
		stored authdomain.RefreshToken
		user   common.User
		userOK bool
	}{
		{name: "blank token", raw: "   ", stored: valid, user: user, userOK: true},
		{name: "unknown digest", raw: "other", stored: valid, user: user, userOK: true},
		{name: "expired exactly now", raw: "token", stored: withExpiry(valid, now), user: user, userOK: true},
		{name: "expired", raw: "token", stored: withExpiry(valid, now.Add(-time.Second)), user: user, userOK: true},
		{name: "revoked", raw: "token", stored: withRevocation(valid, now.Add(-time.Minute)), user: user, userOK: true},
		{name: "user deleted", raw: "token", stored: valid, userOK: false},
		{name: "user ID mismatch", raw: "token", stored: valid, user: common.User{ID: "user-2", Email: user.Email}, userOK: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repository := &users{user: tt.user}
			if !tt.userOK {
				repository.err = application.ErrUserNotFound
			}
			store := &storedRefreshTokens{tokens: map[string]authdomain.RefreshToken{tt.stored.Digest: tt.stored}}
			saved := &refreshTokens{}
			usecase := application.RefreshUsecase{
				Users:                  repository,
				AccessTokens:           accessTokens{},
				Digester:               digester{},
				RefreshTokenFinder:     store,
				RefreshTokenRevoker:    store,
				RefreshTokens:          generatedRefreshToken{},
				RefreshTokenRepository: saved,
				Clock:                  timewrapper.NewFixed(now),
				RefreshTokenTTL:        time.Hour,
			}

			_, err := usecase.Refresh(context.Background(), application.RefreshInput{RefreshToken: tt.raw})
			if !errors.Is(err, application.ErrUnauthorized) {
				t.Fatalf("Refresh() error = %v, want unauthorized", err)
			}
			if store.revoked != "" {
				t.Errorf("revoked %q, want nothing revoked", store.revoked)
			}
			if saved.saved != (authdomain.RefreshToken{}) {
				t.Errorf("saved refresh token = %#v, want nothing saved", saved.saved)
			}
		})
	}
}

func TestRefreshDoesNotIssueTokensWhenRevokeFails(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	user := common.User{ID: common.UserID("user-1"), Email: "member@example.com", PasswordHash: "hash"}
	stored := authdomain.RefreshToken{Digest: "digest:token", UserID: user.ID, Email: user.Email, ExpiresAt: now.Add(time.Hour)}
	revokeErr := errors.New("DynamoDB unavailable")
	store := &storedRefreshTokens{tokens: map[string]authdomain.RefreshToken{stored.Digest: stored}, revokeErr: revokeErr}
	saved := &refreshTokens{}
	usecase := application.RefreshUsecase{
		Users:                  &users{user: user},
		AccessTokens:           accessTokens{},
		Digester:               digester{},
		RefreshTokenFinder:     store,
		RefreshTokenRevoker:    store,
		RefreshTokens:          generatedRefreshToken{},
		RefreshTokenRepository: saved,
		Clock:                  timewrapper.NewFixed(now),
		RefreshTokenTTL:        time.Hour,
	}

	_, err := usecase.Refresh(context.Background(), application.RefreshInput{RefreshToken: "token"})
	if !errors.Is(err, revokeErr) {
		t.Fatalf("Refresh() error = %v, want %v", err, revokeErr)
	}
	if saved.saved != (authdomain.RefreshToken{}) {
		t.Errorf("saved refresh token = %#v, want nothing saved", saved.saved)
	}
}

func TestRefreshPropagatesLookupErrors(t *testing.T) {
	t.Parallel()
	lookupErr := errors.New("DynamoDB unavailable")
	usecase := application.RefreshUsecase{
		Users:              &users{},
		Digester:           digester{},
		RefreshTokenFinder: &storedRefreshTokens{findErr: lookupErr},
		Clock:              timewrapper.NewFixed(time.Time{}),
	}

	_, err := usecase.Refresh(context.Background(), application.RefreshInput{RefreshToken: "token"})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("Refresh() error = %v, want %v", err, lookupErr)
	}
}

func withExpiry(token authdomain.RefreshToken, expiresAt time.Time) authdomain.RefreshToken {
	token.ExpiresAt = expiresAt
	return token
}

func withRevocation(token authdomain.RefreshToken, revokedAt time.Time) authdomain.RefreshToken {
	token.RevokedAt = revokedAt
	return token
}

// digester は raw token をそのまま識別できる形で digest にするテスト用の実装。
type digester struct{}

func (digester) Digest(raw string) string { return "digest:" + raw }

type storedRefreshTokens struct {
	tokens    map[string]authdomain.RefreshToken
	findErr   error
	revokeErr error
	found     string
	revoked   string
	revokedAt time.Time
}

func (s *storedRefreshTokens) FindByDigest(_ context.Context, digest string) (authdomain.RefreshToken, error) {
	s.found = digest
	if s.findErr != nil {
		return authdomain.RefreshToken{}, s.findErr
	}
	token, ok := s.tokens[digest]
	if !ok {
		return authdomain.RefreshToken{}, application.ErrRefreshTokenNotFound
	}
	return token, nil
}

func (s *storedRefreshTokens) Revoke(_ context.Context, digest string, revokedAt time.Time) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.revoked = digest
	s.revokedAt = revokedAt
	return nil
}
