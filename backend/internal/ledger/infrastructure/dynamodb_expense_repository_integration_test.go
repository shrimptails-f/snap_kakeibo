package infrastructure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/ledger/infrastructure"
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

// expenseRecord は analysis/infrastructure.expenseRecord と同じ属性一式。
func expenseRecord(userID, expenseID, requestID string, readAmount, adjustment int64, edited bool) map[string]any {
	return map[string]any{
		"PK": "USER#" + userID, "SK": "EXPENSE#" + expenseID, "type": "EXPENSE",
		"expense_id": expenseID, "analysis_request_id": requestID, "store_name": "スーパー", "purchase_date": "2026-09-18", "year_month": "2026-09",
		"source": "AI", "created_at": "2026-09-18T12:00:00Z", "updated_at": "2026-09-18T12:00:00Z",
		"read_amount": readAmount, "adjustment_amount": adjustment, "recorded_amount": readAmount + adjustment, "is_edited": edited,
	}
}

// detailRecord は analysis/infrastructure.expenseDetailRecord と同じ属性一式。
func detailRecord(userID, expenseID, detailID, name, category, source string, amount, quantity int64) map[string]any {
	return map[string]any{
		"PK": "USER#" + userID + "#EXPENSE#" + expenseID, "SK": "DETAIL#" + detailID,
		"GSI1PK": "USER#" + userID + "#MONTH#2026-09", "GSI1SK": "DETAIL_AMOUNT#0000000000#2026-09-18#" + detailID,
		"type": "EXPENSE_DETAIL", "detail_id": detailID, "expense_id": expenseID, "analysis_request_id": "req-1",
		"name": name, "category": category, "category_source": source, "source": "AI",
		"store_name": "スーパー", "purchase_date": "2026-09-18", "year_month": "2026-09",
		"created_at": "2026-09-18T12:00:00Z", "updated_at": "2026-09-18T12:00:00Z",
		"amount": amount, "quantity": quantity, "is_edited": false,
	}
}

func newRepository(t *testing.T) infrastructure.DynamoDBExpenseRepository {
	t.Helper()
	env := dynamodbtest.Connect(t)
	return infrastructure.DynamoDBExpenseRepository{
		Expenses:       env.CreateTable(t, libdynamodb.ExpensesSchema),
		ExpenseDetails: env.CreateTable(t, libdynamodb.ExpenseDetailsSchema),
	}
}

// find は文字列の識別子を値オブジェクトへ変換して FindByID を呼ぶ。
func find(ctx context.Context, repo infrastructure.DynamoDBExpenseRepository, user, expense string) (domain.Expense, error) {
	userID, _ := common.NewUserID(user)
	expenseID, _ := common.NewExpenseID(expense)
	return repo.FindByID(ctx, userID, expenseID)
}

// TestFindByIDAgainstDynamoDB は analyze-receipt が書いた形の expenses / expense_details から支出集約が復元され、
// 明細が detail_id の昇順で並び、他の支出・他人の明細が混ざらないことを Floci で確認する。STAGE が local / ci のときだけ動く。
func TestFindByIDAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	repo := newRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "expense-1", "request-1", 1200, -200, true))
	// 採番順と逆に登録しても SK 順で返る
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-1", "expense-1", "detail-2", "会食", "social", "USER", 1000, 2))
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-1", "expense-1", "detail-1", "牛乳", "food", "AI", 200, 1))
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-1", "expense-2", "detail-3", "other expense", "food", "AI", 300, 1))
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-2", "expense-1", "detail-4", "other user", "food", "AI", 400, 1))

	got, err := find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.ID() != "expense-1" || got.UserID() != "user-1" || got.SourceRequestID() != "request-1" || got.StoreName() != "スーパー" || got.PurchaseDate().String() != "2026-09-18" {
		t.Errorf("FindByID() = %+v", got)
	}
	if got.ReadAmount().Yen() != 1200 || got.AdjustmentAmount().Yen() != -200 || got.RecordedAmount().Yen() != 1000 || !got.Edited() {
		t.Errorf("amounts = read %d adjustment %d recorded %d edited %t", got.ReadAmount().Yen(), got.AdjustmentAmount().Yen(), got.RecordedAmount().Yen(), got.Edited())
	}
	if got.Source() != domain.RecordSourceAI || got.UpdatedAt().Format(time.RFC3339) != "2026-09-18T12:00:00Z" {
		t.Errorf("metadata = source %q updated_at %v", got.Source(), got.UpdatedAt())
	}
	details := got.Details()
	if len(details) != 2 {
		t.Fatalf("Details() returned %d, want 2: %+v", len(details), details)
	}
	if details[0].ID() != "detail-1" || details[0].Name() != "牛乳" || details[0].Category() != common.CategoryFood || details[0].CategorySource() != domain.CategorySourceAI || details[0].Amount().Yen() != 200 || details[0].Quantity().Int64() != 1 {
		t.Errorf("Details()[0] = %+v", details[0])
	}
	if details[0].Source() != domain.RecordSourceAI || details[0].Edited() {
		t.Errorf("Details()[0] metadata = source %q edited %t", details[0].Source(), details[0].Edited())
	}
	if details[1].ID() != "detail-2" || details[1].Category() != common.CategorySocial || details[1].CategorySource() != domain.CategorySourceUser || details[1].Amount().Yen() != 1000 || details[1].Quantity().Int64() != 2 {
		t.Errorf("Details()[1] = %+v", details[1])
	}
}

// TestFindByIDReportsNotFoundAgainstDynamoDB は存在しない支出と他人の支出が同じ ErrExpenseNotFound になることを Floci で確認する。
func TestFindByIDReportsNotFoundAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	repo := newRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "expense-1", "request-1", 1200, 0, false))

	for _, in := range [][2]string{{"user-1", "expense-2"}, {"user-2", "expense-1"}} {
		if _, err := find(ctx, repo, in[0], in[1]); !errors.Is(err, application.ErrExpenseNotFound) {
			t.Errorf("FindByID(%s, %s) error = %v, want ErrExpenseNotFound", in[0], in[1], err)
		}
	}
}

// TestFindByIDRejectsBrokenRecordsAgainstDynamoDB は集約の不変条件を満たさない項目(明細なし・不正なカテゴリ)を
// そのまま返さず error にすることを Floci で確認する。
func TestFindByIDRejectsBrokenRecordsAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	repo := newRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "no-details", "request-1", 1200, 0, false))
	if _, err := find(ctx, repo, "user-1", "no-details"); !errors.Is(err, domain.ErrInvalidExpense) {
		t.Errorf("FindByID(no details) error = %v, want ErrInvalidExpense", err)
	}

	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "bad-category", "request-1", 1200, 0, false))
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-1", "bad-category", "detail-1", "x", "business_dinner", "AI", 200, 1))
	if _, err := find(ctx, repo, "user-1", "bad-category"); !errors.Is(err, common.ErrInvalidCategory) {
		t.Errorf("FindByID(bad category) error = %v, want ErrInvalidCategory", err)
	}
}
