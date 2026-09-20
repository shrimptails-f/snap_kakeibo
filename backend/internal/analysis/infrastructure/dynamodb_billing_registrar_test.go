package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
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

func newRegistrar(api *transactAPI) DynamoDBBillingRegistrar {
	client := libdynamodb.NewWithAPI(api, nil)
	return DynamoDBBillingRegistrar{
		Client:           client,
		UploadHistories:  client.Table("uploads"),
		Billings:         client.Table("billings"),
		BillingDetails:   client.Table("details"),
		MonthlySummaries: client.Table("summaries"),
	}
}

func TestRegisterBuildsOneTransactionAcrossTables(t *testing.T) {
	t.Parallel()
	api := &transactAPI{}
	billing := domain.Billing{ID: "b1", UserID: "u1", UploadID: "up1", PurchasedAt: "2026-09-18", TotalAmount: 300, Details: []domain.BillingDetail{
		{ID: "d1", Name: "a", Category: "food", Amount: 100, Quantity: 1},
		{ID: "d2", Name: "b", Category: "food", Amount: 200, Quantity: 1},
	}}
	if err := newRegistrar(api).Register(context.Background(), domain.Job{UserID: "u1", UploadID: "up1", Attempt: 1}, billing, "raw.json", testTime); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	items := api.in.TransactItems
	if len(items) != 5 {
		t.Fatalf("transact items = %d, want upload + billing + 2 details + summary", len(items))
	}
	if aws.ToString(items[0].Update.TableName) != "uploads" || aws.ToString(items[0].Update.ConditionExpression) != terminalCondition {
		t.Errorf("upload update = %+v", items[0].Update)
	}
	if aws.ToString(items[1].Put.TableName) != "billings" || aws.ToString(items[2].Put.TableName) != "details" || aws.ToString(items[3].Put.TableName) != "details" {
		t.Errorf("put tables = %s %s %s", aws.ToString(items[1].Put.TableName), aws.ToString(items[2].Put.TableName), aws.ToString(items[3].Put.TableName))
	}
	summary := items[4].Update
	if aws.ToString(summary.TableName) != "summaries" || summary.ExpressionAttributeNames["#c1"] != "category_total_food" {
		t.Errorf("summary update = %+v", summary)
	}
	if got := summary.ExpressionAttributeValues[":c1"].(*ddbtypes.AttributeValueMemberN).Value; got != "300" {
		t.Errorf("category total = %s, want 300", got)
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
		{"validation error is rejected", &ddbtypes.TransactionCanceledException{CancellationReasons: []ddbtypes.CancellationReason{{Code: aws.String("None")}, {Code: aws.String("ValidationError")}}}, application.ErrBillingRejected},
		{"other errors pass through", errors.New("throttled"), errors.New("throttled")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &transactAPI{err: tt.err}
			err := newRegistrar(api).Register(context.Background(), domain.Job{UserID: "u1", UploadID: "up1", Attempt: 1}, domain.Billing{ID: "b1", UserID: "u1", PurchasedAt: "2026-09-18"}, "", testTime)
			switch {
			case tt.want == nil && err != nil:
				t.Fatalf("Register() error = %v, want nil", err)
			case tt.want != nil && errors.Is(tt.want, application.ErrBillingRejected) && !errors.Is(err, application.ErrBillingRejected):
				t.Fatalf("Register() error = %v, want ErrBillingRejected", err)
			case tt.want != nil && !errors.Is(tt.want, application.ErrBillingRejected) && (err == nil || err.Error() != tt.want.Error()):
				t.Fatalf("Register() error = %v, want %v", err, tt.want)
			}
		})
	}
}

var testTime = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
