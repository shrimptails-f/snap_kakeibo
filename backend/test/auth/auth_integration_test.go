// Package auth_test は認証機能を実際の Floci と本番同等の DI で通すシナリオテストを提供する。
package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/infrastructure"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/di"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/ssm/ssmtest"

	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"golang.org/x/crypto/bcrypt"
)

const (
	testEmail    = "member@example.com"
	testPassword = "correct horse battery staple"
	testUserID   = "integration-user"
)

// TestAuthenticationFlow は login → check → refresh → check → logout を、
// mock を使わず Floci の一時 DynamoDB テーブルに対して実行する。
func TestAuthenticationFlow(t *testing.T) {
	t.Parallel()

	env := dynamodbtest.Connect(t)
	users := env.CreateTableWithPrefix(t, "auth-flow", libdynamodb.UsersSchema)
	refreshTokens := env.CreateTableWithPrefix(t, "auth-flow", libdynamodb.RefreshTokensSchema)
	seedUser(t, users)
	// 本番と同じく署名鍵は SSM から取る。Floci の SSM に一時パラメータを置く
	jwtSecretParameter := ssmtest.PutSecureString(t, env.Config, "auth-flow-jwt-secret", "auth-integration-test-secret-at-least-32-bytes")

	cfg := settings.Config{
		UsersTable:         users.Name(),
		RefreshTokensTable: refreshTokens.Name(),
		JWTSecretParameter: jwtSecretParameter,
		Stage:              "local",
	}
	log := logger.NewNop()
	osw := oswrapper.New()

	loginContainer, err := di.NewAuthLoginContainer(cfg, env.Config, osw, log)
	if err != nil {
		t.Fatalf("create auth-login container: %v", err)
	}
	login, err := di.ResolveAuthLoginUsecase(loginContainer)
	if err != nil {
		t.Fatalf("resolve auth-login usecase: %v", err)
	}
	limiter, err := di.ResolveLoginAttemptLimiter(loginContainer)
	if err != nil {
		t.Fatalf("resolve login attempt limiter: %v", err)
	}

	checkContainer, err := di.NewAuthCheckContainer(cfg, env.Config, osw, log)
	if err != nil {
		t.Fatalf("create auth-check container: %v", err)
	}
	check, err := di.ResolveAuthCheckUsecase(checkContainer)
	if err != nil {
		t.Fatalf("resolve auth-check usecase: %v", err)
	}

	refreshContainer, err := di.NewAuthRefreshContainer(cfg, env.Config, osw, log)
	if err != nil {
		t.Fatalf("create auth-refresh container: %v", err)
	}
	refresh, err := di.ResolveAuthRefreshUsecase(refreshContainer)
	if err != nil {
		t.Fatalf("resolve auth-refresh usecase: %v", err)
	}

	logoutContainer, err := di.NewAuthLogoutContainer(cfg, env.Config, osw, log)
	if err != nil {
		t.Fatalf("create auth-logout container: %v", err)
	}
	logout, err := di.ResolveAuthLogoutUsecase(logoutContainer)
	if err != nil {
		t.Fatalf("resolve auth-logout usecase: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, subject := range []string{"ip:127.0.0.1", "email:" + testEmail} {
		allowed, err := limiter.Allow(ctx, subject)
		if err != nil {
			t.Fatalf("allow login for %q: %v", subject, err)
		}
		if !allowed {
			t.Fatalf("login for %q was unexpectedly rate limited", subject)
		}
	}

	loggedIn, err := login.Login(ctx, application.LoginInput{Email: strings.ToUpper(testEmail), Password: testPassword})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	assertIdentity(t, loggedIn.User.ID.String(), loggedIn.User.Email)
	assertCheck(t, ctx, check, loggedIn.Tokens.AccessToken)

	refreshed, err := refresh.Refresh(ctx, application.RefreshInput{RefreshToken: loggedIn.Tokens.RefreshToken})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.Tokens.RefreshToken == loggedIn.Tokens.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	assertCheck(t, ctx, check, refreshed.Tokens.AccessToken)

	if _, err := refresh.Refresh(ctx, application.RefreshInput{RefreshToken: loggedIn.Tokens.RefreshToken}); !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("reuse rotated refresh token: error = %v, want ErrUnauthorized", err)
	}

	if err := logout.Logout(ctx, application.LogoutInput{RefreshToken: refreshed.Tokens.RefreshToken}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := refresh.Refresh(ctx, application.RefreshInput{RefreshToken: refreshed.Tokens.RefreshToken}); !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("refresh after logout: error = %v, want ErrUnauthorized", err)
	}
}

func seedUser(t *testing.T, users *libdynamodb.Table) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = users.PutItem(ctx, &awssdk.PutItemInput{Item: map[string]ddbtypes.AttributeValue{
		"PK":            &ddbtypes.AttributeValueMemberS{Value: infrastructure.UserPKByEmail(testEmail)},
		"user_id":       &ddbtypes.AttributeValueMemberS{Value: testUserID},
		"email":         &ddbtypes.AttributeValueMemberS{Value: testEmail},
		"password_hash": &ddbtypes.AttributeValueMemberS{Value: string(hash)},
	}})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func assertCheck(t *testing.T, ctx context.Context, check application.CheckUsecaseInterface, accessToken string) {
	t.Helper()
	out, err := check.Check(ctx, application.CheckInput{Authorization: "Bearer " + accessToken})
	if err != nil {
		t.Fatalf("check access token: %v", err)
	}
	assertIdentity(t, out.UserID, out.Email)
}

func assertIdentity(t *testing.T, userID, email string) {
	t.Helper()
	if userID != testUserID || email != testEmail {
		t.Fatalf("identity = (%q, %q), want (%q, %q)", userID, email, testUserID, testEmail)
	}
}
