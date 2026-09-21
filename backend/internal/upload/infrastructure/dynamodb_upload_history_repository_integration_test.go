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

// TestMarkRetryingAgainstDynamoDB は再実行の状態遷移が docs/backend.md「再実行」の通りに書かれ、
// 再実行できない status と存在しない履歴が条件式で拒否されることを Floci で確認する。
func TestMarkRetryingAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.UploadHistoriesSchema)
	repo := infrastructure.DynamoDBUploadHistoryRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	retryAt := integrationNow.Add(time.Hour)

	// analyze-receipt が FAILED にした状態を作る
	history, _ := domain.NewUploadHistory("user-1", "upload-1", "", "", integrationNow, 15*time.Minute)
	if err := repo.Save(ctx, history); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	setStatus(ctx, t, table, "user-1", "upload-1", "FAILED", true)

	attempt, err := repo.MarkRetrying(ctx, "user-1", "upload-1", retryAt)
	if err != nil || attempt != 2 {
		t.Fatalf("MarkRetrying() = %d, %v, want 2, nil", attempt, err)
	}
	item := getItem(ctx, t, table, "user-1", "upload-1")
	for key, value := range map[string]any{"status": "ANALYZING", "attempt": float64(2), "updated_at": "2026-09-20T13:00:00Z"} {
		if item[key] != value {
			t.Errorf("item[%q] = %v, want %v", key, item[key], value)
		}
	}
	for _, key := range []string{"error_code", "error_message", "failed_at"} {
		if _, ok := item[key]; ok {
			t.Errorf("item[%q] should be removed, got %v", key, item[key])
		}
	}
	// 登録時の属性は残る
	for _, key := range []string{"PK", "SK", "GSI1PK", "GSI1SK", "type", "upload_id", "s3_key", "file_name", "content_type", "year_month", "expires_at", "created_at"} {
		if _, ok := item[key]; !ok {
			t.Errorf("item[%q] should be kept: %+v", key, item)
		}
	}

	// 停滞した ANALYZING はもう一度やり直せ、attempt がさらに進む
	if attempt, err := repo.MarkRetrying(ctx, "user-1", "upload-1", retryAt); err != nil || attempt != 3 {
		t.Errorf("MarkRetrying(analyzing) = %d, %v, want 3, nil", attempt, err)
	}
	// NO_DATA もやり直せる
	setStatus(ctx, t, table, "user-1", "upload-1", "NO_DATA", false)
	if attempt, err := repo.MarkRetrying(ctx, "user-1", "upload-1", retryAt); err != nil || attempt != 4 {
		t.Errorf("MarkRetrying(no_data) = %d, %v, want 4, nil", attempt, err)
	}

	// 再実行できない status は拒否され、attempt も進まない
	for _, status := range []string{"UPLOADING", "SUCCEEDED"} {
		setStatus(ctx, t, table, "user-1", "upload-1", status, false)
		if _, err := repo.MarkRetrying(ctx, "user-1", "upload-1", retryAt); !errors.Is(err, application.ErrUploadNotRetryable) {
			t.Errorf("MarkRetrying(%s) error = %v, want ErrUploadNotRetryable", status, err)
		}
		if item := getItem(ctx, t, table, "user-1", "upload-1"); item["status"] != status || item["attempt"] != float64(4) {
			t.Errorf("MarkRetrying(%s) should not change the item: %+v", status, item)
		}
	}
	// 存在しない履歴(他人の upload_id を含む)も同じエラー
	if _, err := repo.MarkRetrying(ctx, "user-2", "upload-1", retryAt); !errors.Is(err, application.ErrUploadNotRetryable) {
		t.Errorf("MarkRetrying(missing) error = %v, want ErrUploadNotRetryable", err)
	}
}

// TestListByMonthAgainstDynamoDB は月ごとの一覧が upload_month_index から作成日時の降順で返り、
// 別の月・別の利用者の履歴が混ざらず、analyze-receipt が書く billing_id / error_code / error_message が読めることを Floci で確認する。
func TestListByMonthAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.UploadHistoriesSchema)
	repo := infrastructure.DynamoDBUploadHistoryRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// 同じ月に 3 件(作成順 upload-1 → upload-2 → upload-3)、前の月に 1 件、別の利用者に 1 件
	for _, h := range []struct {
		userID, uploadID string
		createdAt        time.Time
	}{
		{"user-1", "upload-1", integrationNow},
		{"user-1", "upload-2", integrationNow.Add(time.Hour)},
		{"user-1", "upload-3", integrationNow.Add(2 * time.Hour)},
		{"user-1", "upload-0", integrationNow.AddDate(0, -1, 0)},
		{"user-2", "upload-1", integrationNow},
	} {
		history, err := domain.NewUploadHistory(h.userID, h.uploadID, "a.png", "image/png", h.createdAt, 15*time.Minute)
		if err != nil {
			t.Fatalf("NewUploadHistory(%s) error = %v", h.uploadID, err)
		}
		if err := repo.Save(ctx, history); err != nil {
			t.Fatalf("Save(%s) error = %v", h.uploadID, err)
		}
	}
	// analyze-receipt の遷移を模す: upload-1 は FAILED、upload-2 は SUCCEEDED
	setStatus(ctx, t, table, "user-1", "upload-1", "FAILED", true)
	setSucceeded(ctx, t, table, "user-1", "upload-2", "billing-1")

	got, err := repo.ListByMonth(ctx, "user-1", "2026-09")
	if err != nil {
		t.Fatalf("ListByMonth() error = %v", err)
	}
	want := []domain.UploadHistory{
		{
			UserID: "user-1", UploadID: "upload-3", Status: domain.StatusUploading, Attempt: 1,
			S3Key: "receipts/user-1/upload-3/original.jpg", FileName: "a.png", ContentType: "image/png", YearMonth: "2026-09",
			ExpiresAt: integrationNow.Add(2*time.Hour + 15*time.Minute), CreatedAt: integrationNow.Add(2 * time.Hour), UpdatedAt: integrationNow.Add(2 * time.Hour),
		},
		{
			UserID: "user-1", UploadID: "upload-2", Status: domain.StatusSucceeded, Attempt: 1,
			S3Key: "receipts/user-1/upload-2/original.jpg", FileName: "a.png", ContentType: "image/png", YearMonth: "2026-09",
			ExpiresAt: integrationNow.Add(time.Hour + 15*time.Minute), CreatedAt: integrationNow.Add(time.Hour), UpdatedAt: integrationNow.Add(time.Hour),
			BillingID: "billing-1",
		},
		{
			UserID: "user-1", UploadID: "upload-1", Status: domain.StatusFailed, Attempt: 1,
			S3Key: "receipts/user-1/upload-1/original.jpg", FileName: "a.png", ContentType: "image/png", YearMonth: "2026-09",
			ExpiresAt: integrationNow.Add(15 * time.Minute), CreatedAt: integrationNow, UpdatedAt: integrationNow,
			ErrorCode: "INTERNAL", ErrorMessage: "boom",
		},
	}
	if len(got) != len(want) {
		t.Fatalf("ListByMonth() returned %d histories, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !sameHistory(got[i], want[i]) {
			t.Errorf("ListByMonth()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// 前の月には upload-0 だけ
	if got, err := repo.ListByMonth(ctx, "user-1", "2026-08"); err != nil || len(got) != 1 || got[0].UploadID != "upload-0" {
		t.Errorf("ListByMonth(2026-08) = %+v, %v, want only upload-0", got, err)
	}
	// 履歴の無い月と利用者は空のスライス(nil ではない)
	for _, in := range [][2]string{{"user-1", "2026-07"}, {"user-3", "2026-09"}} {
		if got, err := repo.ListByMonth(ctx, in[0], in[1]); err != nil || got == nil || len(got) != 0 {
			t.Errorf("ListByMonth(%s, %s) = %+v, %v, want empty", in[0], in[1], got, err)
		}
	}
}

// sameHistory は time.Time を Equal で比べる(Floci から読み戻した時刻は UTC だが Location の内部表現が違うことがある)。
func sameHistory(a, b domain.UploadHistory) bool {
	if !a.ExpiresAt.Equal(b.ExpiresAt) || !a.CreatedAt.Equal(b.CreatedAt) || !a.UpdatedAt.Equal(b.UpdatedAt) {
		return false
	}
	a.ExpiresAt, a.CreatedAt, a.UpdatedAt = b.ExpiresAt, b.CreatedAt, b.UpdatedAt
	return a == b
}

// setSucceeded は analyze-receipt の登録を模して SUCCEEDED と billing_id を書く。
func setSucceeded(ctx context.Context, t *testing.T, table *libdynamodb.Table, userID, uploadID, billingID string) {
	t.Helper()
	if _, err := table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                      uploadKey(userID, uploadID),
		UpdateExpression:         aws.String("SET #status=:status, billing_id=:billing_id"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":status": &ddbtypes.AttributeValueMemberS{Value: "SUCCEEDED"}, ":billing_id": &ddbtypes.AttributeValueMemberS{Value: billingID},
		},
	}); err != nil {
		t.Fatalf("set succeeded: %v", err)
	}
}

// setStatus は analyze-receipt の遷移を模して status を書き換える。withFailure なら失敗情報も付ける。
func setStatus(ctx context.Context, t *testing.T, table *libdynamodb.Table, userID, uploadID, status string, withFailure bool) {
	t.Helper()
	expr := "SET #status=:status"
	values := map[string]ddbtypes.AttributeValue{":status": &ddbtypes.AttributeValueMemberS{Value: status}}
	if withFailure {
		expr += ", error_code=:code, error_message=:message, failed_at=:failed_at"
		values[":code"] = &ddbtypes.AttributeValueMemberS{Value: "INTERNAL"}
		values[":message"] = &ddbtypes.AttributeValueMemberS{Value: "boom"}
		values[":failed_at"] = &ddbtypes.AttributeValueMemberS{Value: "2026-09-20T12:30:00Z"}
	}
	if _, err := table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                       uploadKey(userID, uploadID),
		UpdateExpression:          aws.String(expr),
		ExpressionAttributeNames:  map[string]string{"#status": "status"},
		ExpressionAttributeValues: values,
	}); err != nil {
		t.Fatalf("set status %s: %v", status, err)
	}
}

func getItem(ctx context.Context, t *testing.T, table *libdynamodb.Table, userID, uploadID string) map[string]any {
	t.Helper()
	out, err := table.GetItem(ctx, &awssdk.GetItemInput{Key: uploadKey(userID, uploadID), ConsistentRead: aws.Bool(true)})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	var item map[string]any
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	return item
}

func uploadKey(userID, uploadID string) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: "USER#" + userID}, "SK": &ddbtypes.AttributeValueMemberS{Value: "UPLOAD#" + uploadID},
	}
}
