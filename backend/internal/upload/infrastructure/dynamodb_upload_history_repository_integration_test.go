package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
	"snap_kakeibo/backend/internal/upload/infrastructure"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var integrationNow = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

// TestSaveAgainstDynamoDB は upload_histories への登録が他の Lambda が読む形で書かれ、
// 同じ upload_id の二重登録が拒否されることを Floci で確認する。STAGE が local / ci のときだけ動く。
func TestSaveAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.UploadHistoriesSchema)
	repo := infrastructure.DynamoDBUploadHistoryRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	history, err := domain.NewUploadHistory("user-1", "upload-1", "a.png", "image/png", integrationNow, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewUploadHistory() error = %v", err)
	}
	if err := repo.Save(ctx, history); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	out, err := table.GetItem(ctx, &awssdk.GetItemInput{Key: map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: "USER#user-1"}, "SK": &ddbtypes.AttributeValueMemberS{Value: "UPLOAD#upload-1"},
	}, ConsistentRead: aws.Bool(true)})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	var item map[string]any
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	want := map[string]any{
		"PK": "USER#user-1", "SK": "UPLOAD#upload-1",
		"GSI1PK": "USER#user-1#MONTH#2026-09", "GSI1SK": "UPLOAD_CREATED_AT#2026-09-20T12:00:00Z#upload-1",
		"type": "UPLOAD_HISTORY", "upload_id": "upload-1", "status": "UPLOADING", "attempt": float64(1),
		"s3_key": "receipts/user-1/upload-1/original.jpg", "file_name": "a.png", "content_type": "image/png",
		"year_month": "2026-09", "expires_at": "2026-09-20T12:15:00Z", "created_at": "2026-09-20T12:00:00Z", "updated_at": "2026-09-20T12:00:00Z",
	}
	if len(item) != len(want) {
		t.Errorf("item has %d attributes, want %d: %+v", len(item), len(want), item)
	}
	for key, value := range want {
		if item[key] != value {
			t.Errorf("item[%q] = %v, want %v", key, item[key], value)
		}
	}

	// 同じキーの再登録は既存の項目を上書きしない
	if err := repo.Save(ctx, history); !errors.Is(err, application.ErrUploadAlreadyExists) {
		t.Fatalf("Save(duplicate) error = %v, want ErrUploadAlreadyExists", err)
	}
	// 別の upload_id は同じ利用者でも登録できる
	other, _ := domain.NewUploadHistory("user-1", "upload-2", "", "", integrationNow, time.Minute)
	if err := repo.Save(ctx, other); err != nil {
		t.Fatalf("Save(other) error = %v", err)
	}
}
