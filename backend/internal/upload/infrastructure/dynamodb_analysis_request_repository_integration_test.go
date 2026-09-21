package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
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

func newRequest(t *testing.T, userID, requestID, fileName, contentType string, now time.Time) domain.AnalysisRequest {
	t.Helper()
	request, err := domain.NewUploadRequest(userID, requestID, fileName, contentType, now, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewUploadRequest(%s) error = %v", requestID, err)
	}
	return request
}

// TestSaveAgainstDynamoDB は analysis_requests への登録が他の Lambda が読む形で書かれ、
// 同じ analysis_request_id の二重登録が拒否されることを Floci で確認する。STAGE が local / ci のときだけ動く。
func TestSaveAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.AnalysisRequestsSchema)
	repo := infrastructure.DynamoDBAnalysisRequestRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	request := newRequest(t, "user-1", "request-1", "a.png", "image/png", integrationNow)
	if err := repo.Save(ctx, request); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	item := getItem(ctx, t, table, "user-1", "request-1")
	want := map[string]any{
		"PK": "USER#user-1", "SK": "ANALYSIS_REQUEST#request-1",
		"GSI1PK": "USER#user-1#MONTH#2026-09", "GSI1SK": "ANALYSIS_REQUEST_CREATED_AT#2026-09-20T12:00:00Z#request-1",
		"type": "ANALYSIS_REQUEST", "analysis_request_id": "request-1", "status": "UPLOADING", "attempt": float64(1),
		"s3_key": "receipts/user-1/request-1/original.jpg", "file_name": "a.png", "content_type": "image/png",
		"year_month": "2026-09", "upload_expires_at": "2026-09-20T12:15:00Z", "created_at": "2026-09-20T12:00:00Z", "updated_at": "2026-09-20T12:00:00Z",
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
	if err := repo.Save(ctx, request); !errors.Is(err, application.ErrAnalysisRequestAlreadyExists) {
		t.Fatalf("Save(duplicate) error = %v, want ErrAnalysisRequestAlreadyExists", err)
	}
	// 別の analysis_request_id は同じ利用者でも登録できる
	if err := repo.Save(ctx, newRequest(t, "user-1", "request-2", "", "", integrationNow)); err != nil {
		t.Fatalf("Save(other) error = %v", err)
	}
}

// TestMarkRetryingAgainstDynamoDB は再解析の状態遷移が docs/backend.md「再解析」の通りに書かれ、
// 再解析できない status と存在しない解析依頼が条件式で拒否されることを Floci で確認する。
func TestMarkRetryingAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.AnalysisRequestsSchema)
	repo := infrastructure.DynamoDBAnalysisRequestRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	retryAt := integrationNow.Add(time.Hour)

	// analyze-receipt が FAILED にした状態を作る
	if err := repo.Save(ctx, newRequest(t, "user-1", "request-1", "", "", integrationNow)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	setStatus(ctx, t, table, "user-1", "request-1", "FAILED", true)

	attempt, err := repo.MarkRetrying(ctx, "user-1", "request-1", retryAt)
	if err != nil || attempt != 2 {
		t.Fatalf("MarkRetrying() = %d, %v, want 2, nil", attempt, err)
	}
	item := getItem(ctx, t, table, "user-1", "request-1")
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
	for _, key := range []string{"PK", "SK", "GSI1PK", "GSI1SK", "type", "analysis_request_id", "s3_key", "file_name", "content_type", "year_month", "upload_expires_at", "created_at"} {
		if _, ok := item[key]; !ok {
			t.Errorf("item[%q] should be kept: %+v", key, item)
		}
	}

	// 停滞した ANALYZING はもう一度やり直せ、attempt がさらに進む
	if attempt, err := repo.MarkRetrying(ctx, "user-1", "request-1", retryAt); err != nil || attempt != 3 {
		t.Errorf("MarkRetrying(analyzing) = %d, %v, want 3, nil", attempt, err)
	}
	// NO_DATA もやり直せる
	setStatus(ctx, t, table, "user-1", "request-1", "NO_DATA", false)
	if attempt, err := repo.MarkRetrying(ctx, "user-1", "request-1", retryAt); err != nil || attempt != 4 {
		t.Errorf("MarkRetrying(no_data) = %d, %v, want 4, nil", attempt, err)
	}

	// 再解析できない status は拒否され、attempt も進まない
	for _, status := range []string{"UPLOADING", "SUCCEEDED"} {
		setStatus(ctx, t, table, "user-1", "request-1", status, false)
		if _, err := repo.MarkRetrying(ctx, "user-1", "request-1", retryAt); !errors.Is(err, application.ErrAnalysisRequestNotRetryable) {
			t.Errorf("MarkRetrying(%s) error = %v, want ErrAnalysisRequestNotRetryable", status, err)
		}
		if item := getItem(ctx, t, table, "user-1", "request-1"); item["status"] != status || item["attempt"] != float64(4) {
			t.Errorf("MarkRetrying(%s) should not change the item: %+v", status, item)
		}
	}
	// 存在しない解析依頼(他人の analysis_request_id を含む)も同じエラー
	if _, err := repo.MarkRetrying(ctx, "user-2", "request-1", retryAt); !errors.Is(err, application.ErrAnalysisRequestNotRetryable) {
		t.Errorf("MarkRetrying(missing) error = %v, want ErrAnalysisRequestNotRetryable", err)
	}
}

// TestListByMonthAgainstDynamoDB は月ごとの一覧が analysis_request_month_index から作成日時の降順で返り、
// 別の月・別の利用者の解析依頼が混ざらず、analyze-receipt が書く expense_id / error_code / error_message から集約が復元できることを Floci で確認する。
func TestListByMonthAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.AnalysisRequestsSchema)
	repo := infrastructure.DynamoDBAnalysisRequestRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// 同じ月に 3 件(作成順 request-1 → request-2 → request-3)、前の月に 1 件、別の利用者に 1 件
	for _, h := range []struct {
		userID, requestID string
		createdAt         time.Time
	}{
		{"user-1", "request-1", integrationNow},
		{"user-1", "request-2", integrationNow.Add(time.Hour)},
		{"user-1", "request-3", integrationNow.Add(2 * time.Hour)},
		{"user-1", "request-0", integrationNow.AddDate(0, -1, 0)},
		{"user-2", "request-1", integrationNow},
	} {
		if err := repo.Save(ctx, newRequest(t, h.userID, h.requestID, "a.png", "image/png", h.createdAt)); err != nil {
			t.Fatalf("Save(%s) error = %v", h.requestID, err)
		}
	}
	// analyze-receipt の遷移を模す: request-1 は FAILED、request-2 は SUCCEEDED
	setStatus(ctx, t, table, "user-1", "request-1", "FAILED", true)
	setSucceeded(ctx, t, table, "user-1", "request-2", "expense-1")

	got, err := repo.ListByMonth(ctx, "user-1", "2026-09")
	if err != nil {
		t.Fatalf("ListByMonth() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListByMonth() returned %d requests, want 3: %+v", len(got), got)
	}
	for i, want := range []struct {
		id        string
		status    domain.AnalysisStatus
		createdAt time.Time
		expenseID string
		errorCode string
	}{
		{"request-3", analysisdomain.AnalysisStatusUploading, integrationNow.Add(2 * time.Hour), "", ""},
		{"request-2", analysisdomain.AnalysisStatusSucceeded, integrationNow.Add(time.Hour), "expense-1", ""},
		{"request-1", analysisdomain.AnalysisStatusFailed, integrationNow, "", "INTERNAL"},
	} {
		r := got[i]
		if r.ID().String() != want.id || r.UserID() != "user-1" || r.Status() != want.status || r.CurrentAttempt().Int() != 1 || r.ExpenseID().String() != want.expenseID {
			t.Errorf("ListByMonth()[%d] = id %q status %q attempt %d expense %q, want %+v", i, r.ID(), r.Status(), r.CurrentAttempt().Int(), r.ExpenseID(), want)
		}
		if r.Image().Reference() != "receipts/user-1/"+want.id+"/original.jpg" || r.Image().FileName() != "a.png" || r.Image().ContentType() != "image/png" {
			t.Errorf("ListByMonth()[%d].Image() = %+v", i, r.Image())
		}
		if !r.CreatedAt().Equal(want.createdAt) || !r.UpdatedAt().Equal(want.createdAt) || !r.UploadExpiresAt().Equal(want.createdAt.Add(15*time.Minute)) {
			t.Errorf("ListByMonth()[%d] times = created %v updated %v expires %v", i, r.CreatedAt(), r.UpdatedAt(), r.UploadExpiresAt())
		}
		reason, ok := r.FailureReason()
		if (want.errorCode != "") != ok || reason.Code() != want.errorCode || ok && reason.SafeMessage() != "boom" {
			t.Errorf("ListByMonth()[%d].FailureReason() = %+v, %v, want %q", i, reason, ok, want.errorCode)
		}
	}

	// 前の月には request-0 だけ
	if got, err := repo.ListByMonth(ctx, "user-1", "2026-08"); err != nil || len(got) != 1 || got[0].ID() != "request-0" {
		t.Errorf("ListByMonth(2026-08) = %+v, %v, want only request-0", got, err)
	}
	// 解析依頼の無い月と利用者は空のスライス(nil ではない)
	for _, in := range [][2]string{{"user-1", "2026-07"}, {"user-3", "2026-09"}} {
		if got, err := repo.ListByMonth(ctx, in[0], in[1]); err != nil || got == nil || len(got) != 0 {
			t.Errorf("ListByMonth(%s, %s) = %+v, %v, want empty", in[0], in[1], got, err)
		}
	}
}

// TestListByMonthRejectsInconsistentItemAgainstDynamoDB は状態と付随する値が食い違う項目(FAILED なのに error_code が無い)を
// 集約として復元せず error にすることを Floci で確認する。
func TestListByMonthRejectsInconsistentItemAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.AnalysisRequestsSchema)
	repo := infrastructure.DynamoDBAnalysisRequestRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := repo.Save(ctx, newRequest(t, "user-1", "request-1", "", "", integrationNow)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	setStatus(ctx, t, table, "user-1", "request-1", "FAILED", false)
	if _, err := repo.ListByMonth(ctx, "user-1", "2026-09"); !errors.Is(err, analysisdomain.ErrInvalidAnalysisRequest) {
		t.Errorf("ListByMonth() error = %v, want ErrInvalidAnalysisRequest", err)
	}
}

// setSucceeded は analyze-receipt の登録を模して SUCCEEDED と expense_id を書く。
func setSucceeded(ctx context.Context, t *testing.T, table *libdynamodb.Table, userID, requestID, expenseID string) {
	t.Helper()
	if _, err := table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                      requestKey(userID, requestID),
		UpdateExpression:         aws.String("SET #status=:status, expense_id=:expense_id"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":status": &ddbtypes.AttributeValueMemberS{Value: "SUCCEEDED"}, ":expense_id": &ddbtypes.AttributeValueMemberS{Value: expenseID},
		},
	}); err != nil {
		t.Fatalf("set succeeded: %v", err)
	}
}

// setStatus は analyze-receipt の遷移を模して status を書き換える。withFailure なら失敗情報も付ける。
func setStatus(ctx context.Context, t *testing.T, table *libdynamodb.Table, userID, requestID, status string, withFailure bool) {
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
		Key:                       requestKey(userID, requestID),
		UpdateExpression:          aws.String(expr),
		ExpressionAttributeNames:  map[string]string{"#status": "status"},
		ExpressionAttributeValues: values,
	}); err != nil {
		t.Fatalf("set status %s: %v", status, err)
	}
}

func getItem(ctx context.Context, t *testing.T, table *libdynamodb.Table, userID, requestID string) map[string]any {
	t.Helper()
	out, err := table.GetItem(ctx, &awssdk.GetItemInput{Key: requestKey(userID, requestID), ConsistentRead: aws.Bool(true)})
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	var item map[string]any
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	return item
}

func requestKey(userID, requestID string) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: "USER#" + userID}, "SK": &ddbtypes.AttributeValueMemberS{Value: "ANALYSIS_REQUEST#" + requestID},
	}
}
