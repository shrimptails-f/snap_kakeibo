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

func TestLoginIssuesAndPersistsTokens(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	user := common.User{ID: common.UserID("user-1"), Email: "member@example.com", PasswordHash: "hash"}
	repository := &users{user: user}
	refreshes := &refreshTokens{}
	usecase := application.LoginUsecase{
		Users:                  repository,
		Passwords:              passwords{},
		AccessTokens:           accessTokens{},
		RefreshTokens:          generatedRefreshToken{},
		RefreshTokenRepository: refreshes,
		Clock:                  timewrapper.NewFixed(now),
		RefreshTokenTTL:        30 * 24 * time.Hour,
	}

	out, err := usecase.Login(context.Background(), application.LoginInput{Email: " MEMBER@example.com ", Password: "correct"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if repository.email != "member@example.com" {
		t.Errorf("repository email = %q", repository.email)
	}
	if out.Tokens.AccessToken != "access" || out.Tokens.RefreshToken != "refresh" || out.Tokens.ExpiresIn != 900 {
		t.Errorf("unexpected tokens: %#v", out.Tokens)
	}
	if out.Tokens.RefreshTokenExpiresIn != int64((30*24*time.Hour)/time.Second) {
		t.Errorf("refresh token expires in = %d", out.Tokens.RefreshTokenExpiresIn)
	}
	if refreshes.saved.UserID != user.ID || !refreshes.saved.ExpiresAt.Equal(now.Add(30*24*time.Hour)) {
		t.Errorf("saved refresh token = %#v", refreshes.saved)
	}
}

func TestLoginDoesNotRevealUnknownUser(t *testing.T) {
	t.Parallel()
	usecase := application.LoginUsecase{
		Users:           &users{err: application.ErrUserNotFound},
		Passwords:       passwords{},
		AccessTokens:    accessTokens{},
		RefreshTokens:   generatedRefreshToken{},
		Clock:           timewrapper.NewFixed(time.Time{}),
		RefreshTokenTTL: time.Hour,
	}
	_, err := usecase.Login(context.Background(), application.LoginInput{Email: "missing@example.com", Password: "password"})
	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want invalid credentials", err)
	}
}

type users struct {
	user  common.User
	err   error
	email string
}

func (r *users) FindByEmail(_ context.Context, email string) (common.User, error) {
	r.email = email
	return r.user, r.err
}

type passwords struct{}

func (passwords) Compare(_, password string) error {
	if password == "correct" {
		return nil
	}
	return errors.New("wrong password")
}

type accessTokens struct{}

func (accessTokens) Issue(context.Context, common.User) (string, int64, error) {
	return "access", 900, nil
}

type generatedRefreshToken struct{}

func (generatedRefreshToken) Generate(user common.User, now, expiresAt time.Time) (string, authdomain.RefreshToken, error) {
	return "refresh", authdomain.RefreshToken{Digest: "digest", UserID: user.ID, ExpiresAt: expiresAt, CreatedAt: now}, nil
}

type refreshTokens struct{ saved authdomain.RefreshToken }

func (r *refreshTokens) Save(_ context.Context, token authdomain.RefreshToken) error {
	r.saved = token
	return nil
}
