package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/analysis/infrastructure"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// tables は本番と同じキー構成の一時テーブル一式と、それに束縛したリポジトリ。
type tables struct {
	env       *dynamodbtest.Env
	histories infrastructure.DynamoDBUploadHistoryRepository
	registrar infrastructure.DynamoDBBillingRegistrar
}

func newTables(t *testing.T) *tables {
	t.Helper()
	env := dynamodbtest.Connect(t)
	uploads := env.CreateTable(t, libdynamodb.UploadHistoriesSchema)
	return &tables{
		env:       env,
		histories: infrastructure.DynamoDBUploadHistoryRepository{Table: uploads},
		registrar: infrastructure.DynamoDBBillingRegistrar{
			Client:           env.Client,
			UploadHistories:  uploads,
			Billings:         env.CreateTable(t, libdynamodb.BillingsSchema),
			BillingDetails:   env.CreateTable(t, libdynamodb.BillingDetailsSchema),
			MonthlySummaries: env.CreateTable(t, libdynamodb.MonthlySummariesSchema),
		},
	}
}

// seedUpload は upload Lambda が作る形の upload_histories アイテムを入れる。
func (tb *tables) seedUpload(t *testing.T, ctx context.Context, job domain.Job, status string) {
	t.Helper()
	_, err := tb.histories.Table.PutItem(ctx, &awssdk.PutItemInput{Item: map[string]ddbtypes.AttributeValue{
		"PK":      &ddbtypes.AttributeValueMemberS{Value: infrastructure.UserPK(job.UserID)},
		"SK":      &ddbtypes.AttributeValueMemberS{Value: infrastructure.UploadSK(job.UploadID)},
		"status":  &ddbtypes.AttributeValueMemberS{Value: status},
		"attempt": &ddbtypes.AttributeValueMemberN{Value: "1"},
	}})
	if err != nil {
		t.Fatalf("seed upload: %v", err)
	}
}

func (tb *tables) getItem(t *testing.T, ctx context.Context, table *libdynamodb.Table, pk, sk string) map[string]any {
	t.Helper()
	out, err := table.GetItem(ctx, &awssdk.GetItemInput{Key: map[string]ddbtypes.AttributeValue{
		"PK": &ddbtypes.AttributeValueMemberS{Value: pk}, "SK": &ddbtypes.AttributeValueMemberS{Value: sk},
	}, ConsistentRead: aws.Bool(true)})
	if err != nil {
		t.Fatalf("GetItem(%s, %s): %v", pk, sk, err)
	}
	if len(out.Item) == 0 {
		return nil
	}
	var item map[string]any
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	return item
}

func (tb *tables) upload(t *testing.T, ctx context.Context, job domain.Job) map[string]any {
	t.Helper()
	return tb.getItem(t, ctx, tb.histories.Table, infrastructure.UserPK(job.UserID), infrastructure.UploadSK(job.UploadID))
}

func (tb *tables) countItems(t *testing.T, ctx context.Context, table *libdynamodb.Table, pk string) int {
	t.Helper()
	out, err := table.Query(ctx, &awssdk.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": &ddbtypes.AttributeValueMemberS{Value: pk}},
		ConsistentRead:            aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("Query(%s): %v", pk, err)
	}
	return len(out.Items)
}

var (
	integrationNow = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	firstJob       = domain.Job{UserID: "user-1", UploadID: "upload-1", Attempt: 1, Trigger: domain.TriggerS3}
)

func sampleBilling(id, uploadID string) domain.Billing {
	return domain.Billing{
		ID: id, UserID: "user-1", UploadID: uploadID, StoreName: "スーパー", PurchasedAt: "2026-09-18", TotalAmount: 350, CreatedAt: integrationNow,
		Details: []domain.BillingDetail{
			{ID: id + "-d1", Name: "牛乳", Category: "food", Amount: 200, Quantity: 1},
			{ID: id + "-d2", Name: "洗剤", Category: "daily_goods", Amount: 150, Quantity: 1},
		},
	}
}

// TestUploadHistoryTransitionsAgainstDynamoDB は upload_histories の状態遷移が条件付きで行われ、
// 重複配信や別 attempt のジョブが結果を上書きしないことを Floci で確認する。STAGE が local / ci のときだけ動く。
func TestUploadHistoryTransitionsAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	tb := newTables(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tb.seedUpload(t, ctx, firstJob, "UPLOADING")

	// 存在しないアップロードや別の attempt は処理しない
	if started, err := tb.histories.MarkAnalyzing(ctx, domain.Job{UserID: "user-1", UploadID: "missing", Attempt: 1}, integrationNow); err != nil || started {
		t.Fatalf("MarkAnalyzing(missing) = %v, %v", started, err)
	}
	if started, err := tb.histories.MarkAnalyzing(ctx, domain.Job{UserID: "user-1", UploadID: "upload-1", Attempt: 2}, integrationNow); err != nil || started {
		t.Fatalf("MarkAnalyzing(attempt 2) = %v, %v", started, err)
	}
	if started, err := tb.histories.MarkAnalyzing(ctx, firstJob, integrationNow); err != nil || !started {
		t.Fatalf("MarkAnalyzing() = %v, %v", started, err)
	}
	if item := tb.upload(t, ctx, firstJob); item["status"] != "ANALYZING" || item["updated_at"] != "2026-09-20T12:00:00Z" {
		t.Errorf("after MarkAnalyzing: %v", item)
	}
	// 同じジョブの再配信は ANALYZING のまま続行できる
	if started, err := tb.histories.MarkAnalyzing(ctx, firstJob, integrationNow); err != nil || !started {
		t.Fatalf("MarkAnalyzing(again) = %v, %v", started, err)
	}

	failure := domain.Failure{Code: domain.FailureNoDate, Message: "購入日を取得できませんでした"}
	if err := tb.histories.MarkFailed(ctx, firstJob, failure, "analysis-results/user-1/upload-1/1/r.json", integrationNow.Add(time.Minute)); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	item := tb.upload(t, ctx, firstJob)
	if item["status"] != "FAILED" || item["error_code"] != "NO_DATE" || item["error_message"] != failure.Message || item["failed_at"] != "2026-09-20T12:01:00Z" || item["raw_result_s3_key"] != "analysis-results/user-1/upload-1/1/r.json" {
		t.Errorf("after MarkFailed: %v", item)
	}
	// 終端状態になった後は MarkAnalyzing も別の終端遷移も効かない(error にはならない)
	if started, err := tb.histories.MarkAnalyzing(ctx, firstJob, integrationNow); err != nil || started {
		t.Fatalf("MarkAnalyzing(after FAILED) = %v, %v", started, err)
	}
	if err := tb.histories.MarkNoData(ctx, firstJob, "", integrationNow); err != nil {
		t.Fatalf("MarkNoData(after FAILED): %v", err)
	}
	if err := tb.registrar.Register(ctx, firstJob, sampleBilling("billing-x", "upload-1"), "", integrationNow); err != nil {
		t.Fatalf("Register(after FAILED): %v", err)
	}
	if item := tb.upload(t, ctx, firstJob); item["status"] != "FAILED" {
		t.Errorf("terminal state was overwritten: %v", item)
	}
	if n := tb.countItems(t, ctx, tb.registrar.Billings, infrastructure.UserPK("user-1")); n != 0 {
		t.Errorf("billings written for a FAILED upload: %d", n)
	}

	// NO_DATA は失敗情報を消す
	second := domain.Job{UserID: "user-1", UploadID: "upload-2", Attempt: 1}
	tb.seedUpload(t, ctx, second, "ANALYZING")
	if err := tb.histories.MarkFailed(ctx, second, failure, "", integrationNow); err != nil {
		t.Fatal(err)
	}
	// FAILED から NO_DATA へは遷移しないので、ANALYZING に戻してから確認する
	tb.seedUpload(t, ctx, second, "ANALYZING")
	if _, err := tb.histories.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                       map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: infrastructure.UserPK(second.UserID)}, "SK": &ddbtypes.AttributeValueMemberS{Value: infrastructure.UploadSK(second.UploadID)}},
		UpdateExpression:          aws.String("SET error_code=:c, error_message=:m, failed_at=:f"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":c": &ddbtypes.AttributeValueMemberS{Value: "NO_DATE"}, ":m": &ddbtypes.AttributeValueMemberS{Value: "x"}, ":f": &ddbtypes.AttributeValueMemberS{Value: "y"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tb.histories.MarkNoData(ctx, second, "analysis-results/user-1/upload-2/1/r.json", integrationNow); err != nil {
		t.Fatalf("MarkNoData: %v", err)
	}
	item = tb.upload(t, ctx, second)
	if item["status"] != "NO_DATA" || item["raw_result_s3_key"] != "analysis-results/user-1/upload-2/1/r.json" {
		t.Errorf("after MarkNoData: %v", item)
	}
	for _, key := range []string{"error_code", "error_message", "failed_at"} {
		if _, ok := item[key]; ok {
			t.Errorf("%s must be removed by MarkNoData: %v", key, item)
		}
	}
}

// TestRegisterBillingAgainstDynamoDB は請求・明細・月次集計の登録と upload_histories の SUCCEEDED への遷移が
// 1 トランザクションで行われ、再実行しても二重登録されず、同じ月の登録が月次集計に積み上がることを Floci で確認する。
func TestRegisterBillingAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	tb := newTables(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tb.seedUpload(t, ctx, firstJob, "ANALYZING")

	billing := sampleBilling("billing-1", "upload-1")
	rawKey := "analysis-results/user-1/upload-1/1/resp.json"
	if err := tb.registrar.Register(ctx, firstJob, billing, rawKey, integrationNow.Add(time.Minute)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	upload := tb.upload(t, ctx, firstJob)
	if upload["status"] != "SUCCEEDED" || upload["billing_id"] != "billing-1" || upload["raw_result_s3_key"] != rawKey || upload["updated_at"] != "2026-09-20T12:01:00Z" {
		t.Errorf("upload = %v", upload)
	}
	got := tb.getItem(t, ctx, tb.registrar.Billings, infrastructure.UserPK("user-1"), infrastructure.BillingSK("billing-1"))
	if got["type"] != "BILLING" || got["upload_id"] != "upload-1" || got["store_name"] != "スーパー" || got["purchased_at"] != "2026-09-18" || got["year_month"] != "2026-09" || got["source"] != "AI" {
		t.Errorf("billing = %v", got)
	}
	if got["original_amount"] != float64(350) || got["final_amount"] != float64(350) || got["discount_amount"] != float64(0) || got["is_edited"] != false || got["created_at"] != "2026-09-20T12:00:00Z" {
		t.Errorf("billing amounts = %v", got)
	}
	detail := tb.getItem(t, ctx, tb.registrar.BillingDetails, infrastructure.DetailPK("user-1", "billing-1"), infrastructure.DetailSK("billing-1-d1"))
	if detail["type"] != "BILLING_DETAIL" || detail["name"] != "牛乳" || detail["category"] != "food" || detail["category_source"] != "AI" || detail["amount"] != float64(200) || detail["quantity"] != float64(1) {
		t.Errorf("detail = %v", detail)
	}
	if detail["GSI1PK"] != infrastructure.UploadMonthPK("user-1", "2026-09") || detail["GSI1SK"] != infrastructure.DetailMonthSK(200, "2026-09-18", "billing-1-d1") {
		t.Errorf("detail GSI keys = %v", detail)
	}
	if n := tb.countItems(t, ctx, tb.registrar.BillingDetails, infrastructure.DetailPK("user-1", "billing-1")); n != 2 {
		t.Errorf("detail count = %d", n)
	}
	summary := tb.getItem(t, ctx, tb.registrar.MonthlySummaries, infrastructure.UserPK("user-1"), infrastructure.MonthSK("2026-09"))
	if summary["type"] != "MONTHLY_SUMMARY" || summary["user_id"] != "user-1" || summary["year_month"] != "2026-09" || summary["updated_at"] != "2026-09-20T12:00:00Z" {
		t.Errorf("summary = %v", summary)
	}
	if summary["total_amount"] != float64(350) || summary["billing_count"] != float64(1) || summary["detail_count"] != float64(2) || summary["version"] != float64(1) || summary["category_total_food"] != float64(200) || summary["category_total_daily_goods"] != float64(150) {
		t.Errorf("summary totals = %v", summary)
	}

	// 同じジョブの再実行: upload_histories は既に SUCCEEDED なので何も書かれない(error にもならない)
	if err := tb.registrar.Register(ctx, firstJob, sampleBilling("billing-dup", "upload-1"), rawKey, integrationNow); err != nil {
		t.Fatalf("Register(again): %v", err)
	}
	if n := tb.countItems(t, ctx, tb.registrar.Billings, infrastructure.UserPK("user-1")); n != 1 {
		t.Errorf("billing count after duplicate = %d", n)
	}
	if summary := tb.getItem(t, ctx, tb.registrar.MonthlySummaries, infrastructure.UserPK("user-1"), infrastructure.MonthSK("2026-09")); summary["billing_count"] != float64(1) {
		t.Errorf("summary changed by duplicate: %v", summary)
	}

	// 同じ月の別アップロードは月次集計に積み上がる
	second := domain.Job{UserID: "user-1", UploadID: "upload-2", Attempt: 1}
	tb.seedUpload(t, ctx, second, "ANALYZING")
	other := sampleBilling("billing-2", "upload-2")
	other.Details = other.Details[:1]
	other.TotalAmount = 200
	if err := tb.registrar.Register(ctx, second, other, "", integrationNow); err != nil {
		t.Fatalf("Register(second): %v", err)
	}
	summary = tb.getItem(t, ctx, tb.registrar.MonthlySummaries, infrastructure.UserPK("user-1"), infrastructure.MonthSK("2026-09"))
	if summary["total_amount"] != float64(550) || summary["billing_count"] != float64(2) || summary["detail_count"] != float64(3) || summary["version"] != float64(2) || summary["category_total_food"] != float64(400) || summary["category_total_daily_goods"] != float64(150) {
		t.Errorf("summary after second billing = %v", summary)
	}
	// 2 件目は rawKey を空で登録したので raw_result_s3_key を持たない
	secondUpload := tb.upload(t, ctx, second)
	if _, ok := secondUpload["raw_result_s3_key"]; secondUpload["status"] != "SUCCEEDED" || secondUpload["billing_id"] != "billing-2" || ok {
		t.Errorf("second upload = %v", secondUpload)
	}
}
