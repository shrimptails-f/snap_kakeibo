package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	authdomain "snap_kakeibo/backend/internal/auth/domain"
	"snap_kakeibo/backend/internal/auth/infrastructure"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// TestRefreshTokenRepositoryAgainstDynamoDB は本番と同じキー構成の refresh-tokens テーブルに対して
// 保存 → digest 検索 → 失効 → ユーザー単位の一括失効 を通す。STAGE が local / ci のときだけ動く。
func TestRefreshTokenRepositoryAgainstDynamoDB(t *testing.T) {
	env := dynamodbtest.Connect(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	table := env.CreateTable(t, libdynamodb.RefreshTokensSchema)
	repository := infrastructure.DynamoDBRefreshTokenRepository{Table: table}

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	tokens := []authdomain.RefreshToken{
		{Digest: "user1-a", UserID: "user-1", Email: "one@example.com", ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastUsedAt: now, Hint: "aaaa"},
		{Digest: "user1-b", UserID: "user-1", Email: "one@example.com", ExpiresAt: now.Add(time.Hour), CreatedAt: now.Add(time.Minute), LastUsedAt: now, Hint: "bbbb"},
		{Digest: "user2-a", UserID: "user-2", Email: "two@example.com", ExpiresAt: now.Add(time.Hour), CreatedAt: now, LastUsedAt: now, Hint: "cccc"},
	}
	for _, token := range tokens {
		if err := repository.Save(ctx, token); err != nil {
			t.Fatalf("Save(%s): %v", token.Digest, err)
		}
	}
	// 同じ digest は二重に保存できない
	if err := repository.Save(ctx, tokens[0]); err == nil {
		t.Error("Save() accepted a duplicate digest")
	}

	got, err := repository.FindByDigest(ctx, "user1-a")
	if err != nil {
		t.Fatalf("FindByDigest: %v", err)
	}
	if got.UserID != "user-1" || got.Email != "one@example.com" || got.Hint != "aaaa" || got.Revoked() {
		t.Errorf("FindByDigest() = %#v", got)
	}
	if !got.ExpiresAt.Equal(now.Add(time.Hour)) || !got.CreatedAt.Equal(now) {
		t.Errorf("timestamps = expires %v created %v", got.ExpiresAt, got.CreatedAt)
	}
	if _, err := repository.FindByDigest(ctx, "missing"); !errors.Is(err, application.ErrRefreshTokenNotFound) {
		t.Errorf("FindByDigest(missing) error = %v", err)
	}

	// TTL 属性は Unix 秒の数値で保存されている
	raw := awssdk.NewFromConfig(env.Config)
	item, err := raw.GetItem(ctx, &awssdk.GetItemInput{TableName: aws.String(table.Name()), Key: map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: infrastructure.RefreshPK("user1-a")},
	}})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if _, ok := item.Item["expires_at"].(*ddbtypes.AttributeValueMemberN); !ok {
		t.Errorf("expires_at = %#v, want a number", item.Item["expires_at"])
	}

	revokedAt := now.Add(10 * time.Minute)
	if err := repository.Revoke(ctx, "user1-a", revokedAt); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := repository.Revoke(ctx, "missing", revokedAt); !errors.Is(err, application.ErrRefreshTokenNotFound) {
		t.Errorf("Revoke(missing) error = %v", err)
	}
	got, err = repository.FindByDigest(ctx, "user1-a")
	if err != nil {
		t.Fatalf("FindByDigest after revoke: %v", err)
	}
	if !got.Revoked() || !got.RevokedAt.Equal(revokedAt) || got.Usable(revokedAt.Add(time.Second)) {
		t.Errorf("token after revoke = %#v", got)
	}

	// user-1 の残りだけ失効し、user-2 は影響を受けない
	if err := repository.RevokeAllByUser(ctx, "user-1", revokedAt.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeAllByUser: %v", err)
	}
	for digest, wantRevoked := range map[string]bool{"user1-a": true, "user1-b": true, "user2-a": false} {
		got, err := repository.FindByDigest(ctx, digest)
		if err != nil {
			t.Fatalf("FindByDigest(%s): %v", digest, err)
		}
		if got.Revoked() != wantRevoked {
			t.Errorf("%s revoked = %v, want %v", digest, got.Revoked(), wantRevoked)
		}
	}
	// 先に単独で失効した user1-a の失効時刻は上書きされない(未失効のものだけを対象にする)
	got, _ = repository.FindByDigest(ctx, "user1-a")
	if !got.RevokedAt.Equal(revokedAt) {
		t.Errorf("user1-a revoked_at = %v, want %v (not overwritten)", got.RevokedAt, revokedAt)
	}
}
