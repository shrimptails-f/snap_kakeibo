package dynamodbtest_test

import (
	"context"
	"strings"
	"testing"
	"time"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// TestFlociTemporaryTables は実際の DynamoDB API(ローカルの Floci)に対して
// 作成 → 読み書き → GSI 検索 → テスト終了時の削除 を通す。STAGE が local / ci のときだけ動く。
func TestFlociTemporaryTables(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var names []string
	t.Run("create, use and clean up", func(t *testing.T) {
		tables := env.CreateAllTables(t, "scenario")
		for _, table := range []*libdynamodb.Table{tables.Users, tables.RefreshTokens, tables.MonthlySummaries, tables.UploadHistories, tables.Billings, tables.BillingDetails} {
			names = append(names, table.Name())
			exists, err := env.Manager.TableExists(ctx, table.Name())
			if err != nil || !exists {
				t.Fatalf("%s: exists=%v err=%v", table.Name(), exists, err)
			}
		}
		if !strings.HasPrefix(tables.Users.Name(), "scenario-users-") {
			t.Errorf("Users table name = %q", tables.Users.Name())
		}
		// 同じスキーマでもう 1 つ作ると別名になる(テスト同士が同じテーブルを見ない)
		if again := env.CreateTable(t, libdynamodb.UsersSchema); again.Name() == tables.Users.Name() {
			t.Errorf("second Users table got the same name %q", again.Name())
		}

		// 業務コードと同じ経路(Table に束縛した PutItem / GetItem)で読み書きできる
		key := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "EMAIL#a@example.com"}}
		if _, err := tables.Users.PutItem(ctx, &awssdk.PutItemInput{Item: map[string]types.AttributeValue{
			"PK":      key["PK"],
			"user_id": &types.AttributeValueMemberS{Value: "user-1"},
		}}); err != nil {
			t.Fatalf("PutItem: %v", err)
		}
		got, err := tables.Users.GetItem(ctx, &awssdk.GetItemInput{Key: key})
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if v, ok := got.Item["user_id"].(*types.AttributeValueMemberS); !ok || v.Value != "user-1" {
			t.Errorf("user_id = %+v", got.Item["user_id"])
		}

		// GSI が本番と同じ名前・キーで作られている(Query が通る)
		raw := awssdk.NewFromConfig(env.Config)
		if _, err := raw.PutItem(ctx, &awssdk.PutItemInput{TableName: aws.String(tables.UploadHistories.Name()), Item: map[string]types.AttributeValue{
			"PK":     &types.AttributeValueMemberS{Value: "USER#user-1"},
			"SK":     &types.AttributeValueMemberS{Value: "UPLOAD#1"},
			"GSI1PK": &types.AttributeValueMemberS{Value: "USER#user-1#2026-09"},
			"GSI1SK": &types.AttributeValueMemberS{Value: "2026-09-20T00:00:00Z"},
		}}); err != nil {
			t.Fatalf("PutItem(upload-histories): %v", err)
		}
		q, err := raw.Query(ctx, &awssdk.QueryInput{
			TableName:                 aws.String(tables.UploadHistories.Name()),
			IndexName:                 aws.String(libdynamodb.UploadMonthIndex),
			KeyConditionExpression:    aws.String("GSI1PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: "USER#user-1#2026-09"}},
		})
		if err != nil {
			t.Fatalf("Query(%s): %v", libdynamodb.UploadMonthIndex, err)
		}
		if q.Count != 1 {
			t.Errorf("Query count = %d, want 1", q.Count)
		}
	})

	// サブテストの t.Cleanup が走った後なので、テーブルは残っていない
	if len(names) != 6 {
		t.Fatalf("created %d tables, want 6", len(names))
	}
	for _, name := range names {
		exists, err := env.Manager.TableExists(ctx, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if exists {
			t.Errorf("%s still exists after cleanup", name)
		}
	}
}
