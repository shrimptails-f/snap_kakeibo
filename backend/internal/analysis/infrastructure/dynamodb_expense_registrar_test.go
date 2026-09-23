package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	ledgerdomain "snap_kakeibo/backend/internal/ledger/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// transactAPI は TransactWriteItems だけを差し替える DynamoDB API。
type transactAPI struct {
	in  *awssdk.TransactWriteItemsInput
	err error
}

func (a *transactAPI) GetItem(context.Context, *awssdk.GetItemInput, ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error) {
	return &awssdk.GetItemOutput{}, nil
}

func (a *transactAPI) PutItem(context.Context, *awssdk.PutItemInput, ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error) {
	return &awssdk.PutItemOutput{}, nil
}

func (a *transactAPI) UpdateItem(context.Context, *awssdk.UpdateItemInput, ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error) {
	return &awssdk.UpdateItemOutput{}, nil
}

func (a *transactAPI) Query(context.Context, *awssdk.QueryInput, ...func(*awssdk.Options)) (*awssdk.QueryOutput, error) {
	return &awssdk.QueryOutput{}, nil
}

func (a *transactAPI) TransactWriteItems(_ context.Context, in *awssdk.TransactWriteItemsInput, _ ...func(*awssdk.Options)) (*awssdk.TransactWriteItemsOutput, error) {
	a.in = in
	return &awssdk.TransactWriteItemsOutput{}, a.err
}

func newRegistrar(api *transactAPI) DynamoDBExpenseRegistrar {
	client := libdynamodb.NewWithAPI(api, nil)
	return DynamoDBExpenseRegistrar{
		Client:           client,
		AnalysisRequests: client.Table("requests"),
		Expenses:         client.Table("expenses"),
		ExpenseDetails:   client.Table("details"),
		MonthlySummaries: client.Table("summaries"),
	}
}

// detailSpec は testExpense に渡す支出明細の指定。
type detailSpec struct {
	id       string
	name     string
	category common.Category
	amount   int64
	quantity int64
}

// testExpense は解析結果から作られた形(調整額 0、未編集、カテゴリ決定元 AI)の支出集約を作る。
func testExpense(t *testing.T, id, requestID string, readAmount int64, specs ...detailSpec) ledgerdomain.Expense {
	t.Helper()
	expenseID, _ := common.NewExpenseID(id)
	userID, _ := common.NewUserID("u1")
	sourceID, _ := common.NewAnalysisRequestID(requestID)
	date, _ := common.NewPurchaseDate("2026-09-18")
	read, err := common.NewReadAmount(readAmount)
	if err != nil {
		t.Fatalf("NewReadAmount() error = %v", err)
	}
	details := make([]ledgerdomain.ExpenseDetail, 0, len(specs))
	for _, spec := range specs {
		detailID, _ := ledgerdomain.NewExpenseDetailID(spec.id)
		amount, _ := common.NewDetailAmount(spec.amount)
		quantity, _ := common.NewQuantity(spec.quantity)
		detail, err := ledgerdomain.NewExpenseDetail(detailID, spec.name, amount, quantity, spec.category, ledgerdomain.CategorySourceAI)
		if err != nil {
			t.Fatalf("NewExpenseDetail(%s) error = %v", spec.id, err)
		}
		details = append(details, detail)
	}
	expense, err := ledgerdomain.NewExpense(expenseID, userID, sourceID, "スーパー", date, read, details)
	if err != nil {
		t.Fatalf("NewExpense() error = %v", err)
	}
	return expense
}

func TestRegisterBuildsOneTransactionAcrossTables(t *testing.T) {
	t.Parallel()
	api := &transactAPI{}
	expense := testExpense(t, "e1", "req1", 300,
		detailSpec{"d1", "a", common.CategoryFood, 100, 1},
		detailSpec{"d2", "b", common.CategoryFood, 200, 1},
	)
	details := expense.Details()
	rate, inclusive := int64(10), int64(110)
	if err := details[0].SetTaxEvidence(&rate, "external", &inclusive, "receipt_tax_proportional_v1"); err != nil {
		t.Fatal(err)
	}
	var err error
	expense, err = ledgerdomain.NewExpense(expense.ID(), expense.UserID(), expense.SourceRequestID(), expense.StoreName(), expense.PurchaseDate(), expense.ReadAmount(), details)
	if err != nil {
		t.Fatal(err)
	}
	expense.SetAnalysisEvidence(`{"selected":{"amount":300},"status":"strong"}`)
	if err := newRegistrar(api).Register(context.Background(), domain.AnalysisJob{UserID: "u1", AnalysisRequestID: "req1", Attempt: 1}, expense, "raw.json", testTime); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	items := api.in.TransactItems
	if len(items) != 5 {
		t.Fatalf("transact items = %d, want request + expense + 2 details + summary", len(items))
	}
	request := items[0].Update
	if aws.ToString(request.TableName) != "requests" || aws.ToString(request.ConditionExpression) != terminalCondition || request.ExpressionAttributeValues[":expense_id"].(*ddbtypes.AttributeValueMemberS).Value != "e1" {
		t.Errorf("request update = %+v", request)
	}
	if aws.ToString(items[1].Put.TableName) != "expenses" || aws.ToString(items[2].Put.TableName) != "details" || aws.ToString(items[3].Put.TableName) != "details" {
		t.Errorf("put tables = %s %s %s", aws.ToString(items[1].Put.TableName), aws.ToString(items[2].Put.TableName), aws.ToString(items[3].Put.TableName))
	}
	var record expenseRecord
	if err := attributevalue.UnmarshalMap(items[1].Put.Item, &record); err != nil {
		t.Fatalf("unmarshal expense record: %v", err)
	}
	if record.Type != "EXPENSE" || record.ExpenseID != "e1" || record.AnalysisRequestID != "req1" || record.PurchaseDate != "2026-09-18" || record.YearMonth != "2026-09" || record.ReadAmount != 300 || record.AnalysisEvidence != expense.AnalysisEvidence() || record.AdjustmentAmount != 0 || record.RecordedAmount != 300 || record.IsEdited {
		t.Errorf("expense record = %+v", record)
	}
	var savedDetail expenseDetailRecord
	if err := attributevalue.UnmarshalMap(items[2].Put.Item, &savedDetail); err != nil {
		t.Fatal(err)
	}
	if savedDetail.TaxIncludedAmount == nil || *savedDetail.TaxIncludedAmount != 110 || savedDetail.Amount != 100 || savedDetail.TaxRate == nil || *savedDetail.TaxRate != 10 {
		t.Errorf("saved detail = %+v", savedDetail)
	}
	summary := items[4].Update
	if aws.ToString(summary.TableName) != "summaries" || summary.ExpressionAttributeNames["#c1"] != "category_total_food" {
		t.Errorf("summary update = %+v", summary)
	}
	if got := summary.ExpressionAttributeValues[":c1"].(*ddbtypes.AttributeValueMemberN).Value; got != "310" {
		t.Errorf("category total = %s, want 310", got)
	}
	if got := summary.ExpressionAttributeValues[":confirmed"].(*ddbtypes.AttributeValueMemberN).Value; got != "1" {
		t.Errorf("confirmed count = %s, want 1", got)
	}
	if got := summary.ExpressionAttributeValues[":total"].(*ddbtypes.AttributeValueMemberN).Value; got != "300" {
		t.Errorf("total recorded amount = %s, want 300", got)
	}
}

func TestRegisterMapsCancellationReasons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"conditional check failed is idempotent", &ddbtypes.TransactionCanceledException{CancellationReasons: []ddbtypes.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}, {Code: aws.String("None")}}}, nil},
		{"validation error is rejected", &ddbtypes.TransactionCanceledException{CancellationReasons: []ddbtypes.CancellationReason{{Code: aws.String("None")}, {Code: aws.String("ValidationError")}}}, application.ErrExpenseRejected},
		{"other errors pass through", errors.New("throttled"), errors.New("throttled")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &transactAPI{err: tt.err}
			expense := testExpense(t, "e1", "req1", 100, detailSpec{"d1", "a", common.CategoryFood, 100, 1})
			err := newRegistrar(api).Register(context.Background(), domain.AnalysisJob{UserID: "u1", AnalysisRequestID: "req1", Attempt: 1}, expense, "", testTime)
			switch {
			case tt.want == nil && err != nil:
				t.Fatalf("Register() error = %v, want nil", err)
			case tt.want != nil && errors.Is(tt.want, application.ErrExpenseRejected) && !errors.Is(err, application.ErrExpenseRejected):
				t.Fatalf("Register() error = %v, want ErrExpenseRejected", err)
			case tt.want != nil && !errors.Is(tt.want, application.ErrExpenseRejected) && (err == nil || err.Error() != tt.want.Error()):
				t.Fatalf("Register() error = %v, want %v", err, tt.want)
			}
		})
	}
}

var testTime = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
