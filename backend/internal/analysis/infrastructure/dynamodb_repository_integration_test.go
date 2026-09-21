package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/analysis/infrastructure"
	common "snap_kakeibo/backend/internal/common/domain"
	ledgerdomain "snap_kakeibo/backend/internal/ledger/domain"
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
	requests  infrastructure.DynamoDBAnalysisRequestRepository
	registrar infrastructure.DynamoDBExpenseRegistrar
}

func newTables(t *testing.T) *tables {
	t.Helper()
	env := dynamodbtest.Connect(t)
	requests := env.CreateTable(t, libdynamodb.AnalysisRequestsSchema)
	return &tables{
		env:      env,
		requests: infrastructure.DynamoDBAnalysisRequestRepository{Table: requests},
		registrar: infrastructure.DynamoDBExpenseRegistrar{
			Client:           env.Client,
			AnalysisRequests: requests,
			Expenses:         env.CreateTable(t, libdynamodb.ExpensesSchema),
			ExpenseDetails:   env.CreateTable(t, libdynamodb.ExpenseDetailsSchema),
			MonthlySummaries: env.CreateTable(t, libdynamodb.MonthlySummariesSchema),
		},
	}
}

// seedRequest は upload Lambda が作る形の analysis_requests アイテムを入れる。
func (tb *tables) seedRequest(t *testing.T, ctx context.Context, job domain.AnalysisJob, status string) {
	t.Helper()
	_, err := tb.requests.Table.PutItem(ctx, &awssdk.PutItemInput{Item: map[string]ddbtypes.AttributeValue{
		"PK":      &ddbtypes.AttributeValueMemberS{Value: infrastructure.UserPK(job.UserID)},
		"SK":      &ddbtypes.AttributeValueMemberS{Value: infrastructure.AnalysisRequestSK(job.AnalysisRequestID)},
		"status":  &ddbtypes.AttributeValueMemberS{Value: status},
		"attempt": &ddbtypes.AttributeValueMemberN{Value: "1"},
	}})
	if err != nil {
		t.Fatalf("seed request: %v", err)
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

func (tb *tables) request(t *testing.T, ctx context.Context, job domain.AnalysisJob) map[string]any {
	t.Helper()
	return tb.getItem(t, ctx, tb.requests.Table, infrastructure.UserPK(job.UserID), infrastructure.AnalysisRequestSK(job.AnalysisRequestID))
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
	firstJob       = domain.AnalysisJob{UserID: "user-1", AnalysisRequestID: "request-1", Attempt: 1, Trigger: domain.TriggerS3}
)

// sampleExpense は読取金額 350 円、明細 2 件(food 200 / social 150)の支出集約を作る。
func sampleExpense(t *testing.T, id, requestID string) ledgerdomain.Expense {
	t.Helper()
	return buildExpense(t, id, requestID, 350, []detailSpec{{id + "-d1", "牛乳", common.CategoryFood, 200}, {id + "-d2", "会食", common.CategorySocial, 150}})
}

type detailSpec struct {
	id, name string
	category common.Category
	amount   int64
}

func buildExpense(t *testing.T, id, requestID string, readAmount int64, specs []detailSpec) ledgerdomain.Expense {
	t.Helper()
	expenseID, _ := common.NewExpenseID(id)
	userID, _ := common.NewUserID("user-1")
	sourceID, _ := common.NewAnalysisRequestID(requestID)
	date, _ := common.NewPurchaseDate("2026-09-18")
	read, _ := common.NewReadAmount(readAmount)
	details := make([]ledgerdomain.ExpenseDetail, 0, len(specs))
	for _, spec := range specs {
		detailID, _ := ledgerdomain.NewExpenseDetailID(spec.id)
		amount, _ := common.NewDetailAmount(spec.amount)
		quantity, _ := common.NewQuantity(1)
		detail, err := ledgerdomain.NewExpenseDetail(detailID, spec.name, amount, quantity, spec.category, ledgerdomain.CategorySourceAI)
		if err != nil {
			t.Fatalf("NewExpenseDetail(%s) error = %v", spec.id, err)
		}
		details = append(details, detail)
	}
	expense, err := ledgerdomain.NewExpense(expenseID, userID, sourceID, "スーパー", date, read, details)
	if err != nil {
		t.Fatalf("NewExpense(%s) error = %v", id, err)
	}
	return expense
}

// TestAnalysisRequestTransitionsAgainstDynamoDB は analysis_requests の状態遷移が条件付きで行われ、
// 重複配信や別 attempt のジョブが結果を上書きしないことを Floci で確認する。STAGE が local / ci のときだけ動く。
func TestAnalysisRequestTransitionsAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	tb := newTables(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tb.seedRequest(t, ctx, firstJob, "UPLOADING")

	// 存在しない解析依頼や別の attempt は処理しない
	if started, err := tb.requests.MarkAnalyzing(ctx, domain.AnalysisJob{UserID: "user-1", AnalysisRequestID: "missing", Attempt: 1}, integrationNow); err != nil || started {
		t.Fatalf("MarkAnalyzing(missing) = %v, %v", started, err)
	}
	if started, err := tb.requests.MarkAnalyzing(ctx, domain.AnalysisJob{UserID: "user-1", AnalysisRequestID: "request-1", Attempt: 2}, integrationNow); err != nil || started {
		t.Fatalf("MarkAnalyzing(attempt 2) = %v, %v", started, err)
	}
	if started, err := tb.requests.MarkAnalyzing(ctx, firstJob, integrationNow); err != nil || !started {
		t.Fatalf("MarkAnalyzing() = %v, %v", started, err)
	}
	if item := tb.request(t, ctx, firstJob); item["status"] != "ANALYZING" || item["updated_at"] != "2026-09-20T12:00:00Z" {
		t.Errorf("after MarkAnalyzing: %v", item)
	}
	// 同じジョブの再配信は ANALYZING のまま続行できる
	if started, err := tb.requests.MarkAnalyzing(ctx, firstJob, integrationNow); err != nil || !started {
		t.Fatalf("MarkAnalyzing(again) = %v, %v", started, err)
	}

	reason, _ := domain.NewFailureReason(domain.FailureNoDate, "購入日を取得できませんでした")
	if err := tb.requests.MarkFailed(ctx, firstJob, reason, "analysis-results/user-1/request-1/1/r.json", integrationNow.Add(time.Minute)); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	item := tb.request(t, ctx, firstJob)
	if item["status"] != "FAILED" || item["error_code"] != "NO_DATE" || item["error_message"] != reason.SafeMessage() || item["failed_at"] != "2026-09-20T12:01:00Z" || item["raw_result_s3_key"] != "analysis-results/user-1/request-1/1/r.json" {
		t.Errorf("after MarkFailed: %v", item)
	}
	// 終端状態になった後は MarkAnalyzing も別の終端遷移も効かない(error にはならない)
	if started, err := tb.requests.MarkAnalyzing(ctx, firstJob, integrationNow); err != nil || started {
		t.Fatalf("MarkAnalyzing(after FAILED) = %v, %v", started, err)
	}
	if err := tb.requests.MarkNoData(ctx, firstJob, "", integrationNow); err != nil {
		t.Fatalf("MarkNoData(after FAILED): %v", err)
	}
	if err := tb.registrar.Register(ctx, firstJob, sampleExpense(t, "expense-x", "request-1"), "", integrationNow); err != nil {
		t.Fatalf("Register(after FAILED): %v", err)
	}
	if item := tb.request(t, ctx, firstJob); item["status"] != "FAILED" {
		t.Errorf("terminal state was overwritten: %v", item)
	}
	if n := tb.countItems(t, ctx, tb.registrar.Expenses, infrastructure.UserPK("user-1")); n != 0 {
		t.Errorf("expenses written for a FAILED request: %d", n)
	}

	// NO_DATA は失敗情報を消す
	second := domain.AnalysisJob{UserID: "user-1", AnalysisRequestID: "request-2", Attempt: 1}
	tb.seedRequest(t, ctx, second, "ANALYZING")
	if err := tb.requests.MarkFailed(ctx, second, reason, "", integrationNow); err != nil {
		t.Fatal(err)
	}
	// FAILED から NO_DATA へは遷移しないので、ANALYZING に戻してから確認する
	tb.seedRequest(t, ctx, second, "ANALYZING")
	if _, err := tb.requests.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                       map[string]ddbtypes.AttributeValue{"PK": &ddbtypes.AttributeValueMemberS{Value: infrastructure.UserPK(second.UserID)}, "SK": &ddbtypes.AttributeValueMemberS{Value: infrastructure.AnalysisRequestSK(second.AnalysisRequestID)}},
		UpdateExpression:          aws.String("SET error_code=:c, error_message=:m, failed_at=:f"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":c": &ddbtypes.AttributeValueMemberS{Value: "NO_DATE"}, ":m": &ddbtypes.AttributeValueMemberS{Value: "x"}, ":f": &ddbtypes.AttributeValueMemberS{Value: "y"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tb.requests.MarkNoData(ctx, second, "analysis-results/user-1/request-2/1/r.json", integrationNow); err != nil {
		t.Fatalf("MarkNoData: %v", err)
	}
	item = tb.request(t, ctx, second)
	if item["status"] != "NO_DATA" || item["raw_result_s3_key"] != "analysis-results/user-1/request-2/1/r.json" {
		t.Errorf("after MarkNoData: %v", item)
	}
	for _, key := range []string{"error_code", "error_message", "failed_at"} {
		if _, ok := item[key]; ok {
			t.Errorf("%s must be removed by MarkNoData: %v", key, item)
		}
	}
}

// TestRegisterExpenseAgainstDynamoDB は支出・支出明細・月次集計の登録と analysis_requests の SUCCEEDED への遷移が
// 1 トランザクションで行われ、再実行しても二重登録されず、同じ月の登録が月次集計に積み上がることを Floci で確認する。
func TestRegisterExpenseAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	tb := newTables(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tb.seedRequest(t, ctx, firstJob, "ANALYZING")

	expense := sampleExpense(t, "expense-1", "request-1")
	rawKey := "analysis-results/user-1/request-1/1/resp.json"
	if err := tb.registrar.Register(ctx, firstJob, expense, rawKey, integrationNow.Add(time.Minute)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	request := tb.request(t, ctx, firstJob)
	if request["status"] != "SUCCEEDED" || request["expense_id"] != "expense-1" || request["raw_result_s3_key"] != rawKey || request["updated_at"] != "2026-09-20T12:01:00Z" {
		t.Errorf("request = %v", request)
	}
	got := tb.getItem(t, ctx, tb.registrar.Expenses, infrastructure.UserPK("user-1"), infrastructure.ExpenseSK("expense-1"))
	if got["type"] != "EXPENSE" || got["analysis_request_id"] != "request-1" || got["store_name"] != "スーパー" || got["purchase_date"] != "2026-09-18" || got["year_month"] != "2026-09" || got["source"] != "AI" {
		t.Errorf("expense = %v", got)
	}
	if got["read_amount"] != float64(350) || got["recorded_amount"] != float64(350) || got["adjustment_amount"] != float64(0) || got["is_edited"] != false || got["created_at"] != "2026-09-20T12:01:00Z" {
		t.Errorf("expense amounts = %v", got)
	}
	detail := tb.getItem(t, ctx, tb.registrar.ExpenseDetails, infrastructure.DetailPK("user-1", "expense-1"), infrastructure.DetailSK("expense-1-d1"))
	if detail["type"] != "EXPENSE_DETAIL" || detail["expense_id"] != "expense-1" || detail["analysis_request_id"] != "request-1" || detail["name"] != "牛乳" || detail["category"] != "food" || detail["category_source"] != "AI" || detail["amount"] != float64(200) || detail["quantity"] != float64(1) || detail["purchase_date"] != "2026-09-18" {
		t.Errorf("detail = %v", detail)
	}
	if detail["GSI1PK"] != infrastructure.UserMonthPK("user-1", "2026-09") || detail["GSI1SK"] != infrastructure.DetailMonthSK(200, "2026-09-18", "expense-1-d1") {
		t.Errorf("detail GSI keys = %v", detail)
	}
	if n := tb.countItems(t, ctx, tb.registrar.ExpenseDetails, infrastructure.DetailPK("user-1", "expense-1")); n != 2 {
		t.Errorf("detail count = %d", n)
	}
	summary := tb.getItem(t, ctx, tb.registrar.MonthlySummaries, infrastructure.UserPK("user-1"), infrastructure.MonthSK("2026-09"))
	if summary["type"] != "MONTHLY_SUMMARY" || summary["user_id"] != "user-1" || summary["year_month"] != "2026-09" || summary["updated_at"] != "2026-09-20T12:01:00Z" {
		t.Errorf("summary = %v", summary)
	}
	if summary["total_recorded_amount"] != float64(350) || summary["expense_count"] != float64(1) || summary["detail_count"] != float64(2) || summary["version"] != float64(1) || summary["category_total_food"] != float64(200) || summary["category_total_social"] != float64(150) {
		t.Errorf("summary totals = %v", summary)
	}

	// 同じジョブの再実行: analysis_requests は既に SUCCEEDED なので何も書かれない(error にもならない)
	if err := tb.registrar.Register(ctx, firstJob, sampleExpense(t, "expense-dup", "request-1"), rawKey, integrationNow); err != nil {
		t.Fatalf("Register(again): %v", err)
	}
	if n := tb.countItems(t, ctx, tb.registrar.Expenses, infrastructure.UserPK("user-1")); n != 1 {
		t.Errorf("expense count after duplicate = %d", n)
	}
	if summary := tb.getItem(t, ctx, tb.registrar.MonthlySummaries, infrastructure.UserPK("user-1"), infrastructure.MonthSK("2026-09")); summary["expense_count"] != float64(1) {
		t.Errorf("summary changed by duplicate: %v", summary)
	}

	// 同じ月の別の解析依頼は月次集計に積み上がる
	second := domain.AnalysisJob{UserID: "user-1", AnalysisRequestID: "request-2", Attempt: 1}
	tb.seedRequest(t, ctx, second, "ANALYZING")
	other := buildExpense(t, "expense-2", "request-2", 200, []detailSpec{{"expense-2-d1", "パン", common.CategoryFood, 200}})
	if err := tb.registrar.Register(ctx, second, other, "", integrationNow); err != nil {
		t.Fatalf("Register(second): %v", err)
	}
	summary = tb.getItem(t, ctx, tb.registrar.MonthlySummaries, infrastructure.UserPK("user-1"), infrastructure.MonthSK("2026-09"))
	if summary["total_recorded_amount"] != float64(550) || summary["expense_count"] != float64(2) || summary["detail_count"] != float64(3) || summary["version"] != float64(2) || summary["category_total_food"] != float64(400) || summary["category_total_social"] != float64(150) {
		t.Errorf("summary after second expense = %v", summary)
	}
	// 2 件目は rawKey を空で登録したので raw_result_s3_key を持たない
	secondRequest := tb.request(t, ctx, second)
	if _, ok := secondRequest["raw_result_s3_key"]; secondRequest["status"] != "SUCCEEDED" || secondRequest["expense_id"] != "expense-2" || ok {
		t.Errorf("second request = %v", secondRequest)
	}
}
