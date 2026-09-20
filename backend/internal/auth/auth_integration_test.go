package auth_test

import (
	"context"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/app"
	"snap_kakeibo/backend/internal/auth"
	authdomain "snap_kakeibo/backend/internal/auth/domain"
	"snap_kakeibo/backend/internal/auth/infrastructure"
	"snap_kakeibo/backend/internal/auth/library/token"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"

	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// TestLogoutRevokesTokenInRefreshTokensTable は移行前の Logout が、新しい refresh-tokens テーブルに
// 保存されたトークンを失効できることを確認する。STAGE が local / ci のときだけ動く。
func TestLogoutRevokesTokenInRefreshTokensTable(t *testing.T) {
	env := dynamodbtest.Connect(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	table := env.CreateTable(t, libdynamodb.RefreshTokensSchema)
	repository := infrastructure.DynamoDBRefreshTokenRepository{Table: table}

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	const raw = "raw-refresh-token"
	if err := repository.Save(ctx, authdomain.RefreshToken{
		Digest: token.Digest(raw), UserID: "user-1", Email: "one@example.com",
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastUsedAt: now, Hint: "oken",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := &auth.Service{DDB: awssdk.NewFromConfig(env.Config), Cfg: app.Config{RefreshTokensTable: table.Name()}}
	if err := svc.Logout(ctx, raw); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	got, err := repository.FindByDigest(ctx, token.Digest(raw))
	if err != nil {
		t.Fatalf("FindByDigest: %v", err)
	}
	if !got.Revoked() {
		t.Errorf("token is not revoked after Logout: %#v", got)
	}

	// 未知のトークンや空の Cookie ではアイテムを作らず、エラーにもしない
	for _, unknown := range []string{"unknown-token", ""} {
		if err := svc.Logout(ctx, unknown); err != nil {
			t.Errorf("Logout(%q): %v", unknown, err)
		}
	}
	if _, err := repository.FindByDigest(ctx, token.Digest("unknown-token")); err == nil {
		t.Error("Logout(unknown) created an item")
	}
}
