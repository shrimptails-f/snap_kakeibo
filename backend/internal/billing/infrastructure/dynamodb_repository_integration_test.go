package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/billing/application"
	"snap_kakeibo/backend/internal/billing/domain"
	"snap_kakeibo/backend/internal/billing/infrastructure"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// putRecord は analyze-receipt が TransactWriteItems で書く項目を模して 1 件登録する。
func putRecord(ctx context.Context, t *testing.T, table *libdynamodb.Table, record map[string]any) {
	t.Helper()
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if _, err := table.PutItem(ctx, &awssdk.PutItemInput{Item: item}); err != nil {
		t.Fatalf("PutItem() error = %v", err)
	}
}

// billingRecord は analysis/infrastructure.billingRecord と同じ属性一式。
func billingRecord(userID, billingID, uploadID string, amount int64) map[string]any {
	return map[string]any{
		"PK": "USER#" + userID, "SK": "BILLING#" + billingID, "type": "BILLING",
		"billing_id": billingID, "upload_id": uploadID, "store_name": "スーパー", "purchased_at": "2026-09-18", "year_month": "2026-09",
		"source": "AI", "created_at": "2026-09-18T12:00:00Z", "updated_at": "2026-09-18T12:00:00Z",
		"original_amount": amount, "discount_amount": int64(0), "final_amount": amount, "is_edited": false,
	}
}

// detailRecord は analysis/infrastructure.billingDetailRecord と同じ属性一式。
func detailRecord(userID, billingID, detailID, name string, amount, quantity int64) map[string]any {
	return map[string]any{
		"PK": "USER#" + userID + "#BILLING#" + billingID, "SK": "DETAIL#" + detailID,
		"GSI1PK": "USER#" + userID + "#MONTH#2026-09", "GSI1SK": "DETAIL_AMOUNT#0000000000#2026-09-18#" + detailID,
		"type": "BILLING_DETAIL", "detail_id": detailID, "billing_id": billingID, "upload_id": "up1",
		"name": name, "category": "food", "category_source": "AI", "source": "AI",
		"store_name": "スーパー", "purchased_at": "2026-09-18", "year_month": "2026-09",
		"created_at": "2026-09-18T12:00:00Z", "updated_at": "2026-09-18T12:00:00Z",
		"amount": amount, "quantity": quantity, "is_edited": false,
	}
}

// TestFindByIDAgainstDynamoDB は analyze-receipt が書いた形の billings から請求が読め、
// 存在しない請求と他人の請求が ErrBillingNotFound になることを Floci で確認する。STAGE が local / ci のときだけ動く。
func TestFindByIDAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.BillingsSchema)
	repo := infrastructure.DynamoDBBillingRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	putRecord(ctx, t, table, billingRecord("user-1", "billing-1", "upload-1", 1200))

	got, err := repo.FindByID(ctx, "user-1", "billing-1")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	want := domain.Billing{
		ID: "billing-1", UserID: "user-1", UploadID: "upload-1", StoreName: "スーパー", PurchasedAt: "2026-09-18", YearMonth: "2026-09",
		OriginalAmount: 1200, DiscountAmount: 0, FinalAmount: 1200,
	}
	if got != want {
		t.Errorf("FindByID() = %+v, want %+v", got, want)
	}

	// 存在しない請求と、他人の請求(同じ billing_id を別の利用者から)は同じエラー
	for _, in := range [][2]string{{"user-1", "billing-2"}, {"user-2", "billing-1"}} {
		if _, err := repo.FindByID(ctx, in[0], in[1]); !errors.Is(err, application.ErrBillingNotFound) {
			t.Errorf("FindByID(%s, %s) error = %v, want ErrBillingNotFound", in[0], in[1], err)
		}
	}
}

// TestListByBillingAgainstDynamoDB は請求の明細だけが detail_id の昇順で返り、
// 他の請求・他人の明細が混ざらず、明細の無い請求は空のスライスになることを Floci で確認する。
func TestListByBillingAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	table := env.CreateTable(t, libdynamodb.BillingDetailsSchema)
	repo := infrastructure.DynamoDBBillingDetailRepository{Table: table}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// 採番順と逆に登録しても SK 順で返る
	putRecord(ctx, t, table, detailRecord("user-1", "billing-1", "detail-2", "bread", 1000, 2))
	putRecord(ctx, t, table, detailRecord("user-1", "billing-1", "detail-1", "milk", 200, 1))
	putRecord(ctx, t, table, detailRecord("user-1", "billing-2", "detail-3", "other billing", 300, 1))
	putRecord(ctx, t, table, detailRecord("user-2", "billing-1", "detail-4", "other user", 400, 1))

	got, err := repo.ListByBilling(ctx, "user-1", "billing-1")
	if err != nil {
		t.Fatalf("ListByBilling() error = %v", err)
	}
	want := []domain.BillingDetail{
		{ID: "detail-1", Name: "milk", Category: "food", CategorySource: "AI", Amount: 200, Quantity: 1},
		{ID: "detail-2", Name: "bread", Category: "food", CategorySource: "AI", Amount: 1000, Quantity: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("ListByBilling() returned %d details, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListByBilling()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// 明細の無い請求は空のスライス(nil ではない)
	if got, err := repo.ListByBilling(ctx, "user-1", "billing-9"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("ListByBilling(missing) = %+v, %v, want empty", got, err)
	}
}
