package application_test

import (
	"context"
	"errors"
	"testing"

	"snap_kakeibo/backend/internal/auth/application"
	common "snap_kakeibo/backend/internal/common/domain"
)

type accessTokenVerifierStub struct {
	got  string
	user common.User
	err  error
}

func (s *accessTokenVerifierStub) Verify(_ context.Context, raw string) (common.User, error) {
	s.got = raw
	return s.user, s.err
}

func TestCheckReturnsAuthenticatedUser(t *testing.T) {
	verifier := &accessTokenVerifierStub{user: common.User{ID: common.UserID("user-1"), Email: "member@example.com", PasswordHash: "must-not-leak"}}
	usecase := application.NewCheckUsecase(verifier)

	got, err := usecase.Check(context.Background(), application.CheckInput{Authorization: "Bearer access-token"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if verifier.got != "access-token" {
		t.Errorf("Verify() token = %q, want access-token", verifier.got)
	}
	if got.UserID != "user-1" || got.Email != "member@example.com" {
		t.Errorf("Check() = %#v", got)
	}
}

func TestCheckRejectsInvalidAuthorization(t *testing.T) {
	for _, authorization := range []string{"", "Basic token", "Bearer", "Bearer   "} {
		t.Run(authorization, func(t *testing.T) {
			usecase := application.NewCheckUsecase(&accessTokenVerifierStub{})
			_, err := usecase.Check(context.Background(), application.CheckInput{Authorization: authorization})
			if !errors.Is(err, application.ErrUnauthorized) {
				t.Fatalf("Check() error = %v, want ErrUnauthorized", err)
			}
		})
	}
}

func TestCheckMapsInvalidTokenToUnauthorized(t *testing.T) {
	usecase := application.NewCheckUsecase(&accessTokenVerifierStub{err: application.ErrUnauthorized})
	_, err := usecase.Check(context.Background(), application.CheckInput{Authorization: "Bearer invalid"})
	if !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("Check() error = %v, want ErrUnauthorized", err)
	}
}

func TestCheckReturnsVerifierFailure(t *testing.T) {
	want := errors.New("secret unavailable")
	usecase := application.NewCheckUsecase(&accessTokenVerifierStub{err: want})
	_, err := usecase.Check(context.Background(), application.CheckInput{Authorization: "Bearer token"})
	if !errors.Is(err, want) {
		t.Fatalf("Check() error = %v, want %v", err, want)
	}
}
