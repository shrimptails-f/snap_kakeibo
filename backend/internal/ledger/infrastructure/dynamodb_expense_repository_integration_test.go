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
		Client:         env.Client,
		Expenses:       env.CreateTable(t, libdynamodb.ExpensesSchema),
		ExpenseDetails: env.CreateTable(t, libdynamodb.ExpenseDetailsSchema),
	}
}

func TestSaveAndFindByMonthAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	repo := newRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "expense-1", "request-1", 1200, 0, false))
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-1", "expense-1", "detail-1", "牛乳", "food", "AI", 1200, 1))
	expense, err := find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	previousDetails := expense.Details()
	date, _ := common.NewPurchaseDate("2026-10-01")
	amount, _ := common.NewDetailAmount(900)
	quantity, _ := common.NewQuantity(2)
	category, _ := common.NewCategory("daily_goods")
	if err := expense.ChangePurchaseDate(date); err != nil {
		t.Fatal(err)
	}
	expense.ChangeStoreName("別の店")
	if err := expense.AdjustAmount(domain.NewAdjustmentAmount(-300)); err != nil {
		t.Fatal(err)
	}
	if err := expense.RenameDetail("detail-1", "洗剤"); err != nil {
		t.Fatal(err)
	}
	if err := expense.ChangeDetailAmount("detail-1", amount, quantity); err != nil {
		t.Fatal(err)
	}
	if err := expense.ChangeDetailCategory("detail-1", category); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, expense, previousDetails, time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	userID, _ := common.NewUserID("user-1")
	oldMonth, _ := common.NewYearMonth("2026-09")
	newMonth, _ := common.NewYearMonth("2026-10")
	oldExpenses, err := repo.FindByMonth(ctx, userID, oldMonth)
	if err != nil || len(oldExpenses) != 0 {
		t.Fatalf("FindByMonth(old) = %+v, %v", oldExpenses, err)
	}
	newExpenses, err := repo.FindByMonth(ctx, userID, newMonth)
	if err != nil || len(newExpenses) != 1 {
		t.Fatalf("FindByMonth(new) = %+v, %v", newExpenses, err)
	}
	got := newExpenses[0]
	if got.StoreName() != "別の店" || got.RecordedAmount().Yen() != 900 || got.Details()[0].CategorySource() != domain.CategorySourceUser {
		t.Errorf("updated expense = %+v / %+v", got, got.Details())
	}
}

func TestSaveAddsAndDeletesDetailsAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	repo := newRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "expense-1", "request-1", 1200, 0, false))
	putRecord(ctx, t, repo.ExpenseDetails, detailRecord("user-1", "expense-1", "detail-1", "旧商品", "food", "AI", 1200, 1))
	expense, err := find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatal(err)
	}
	previous := expense.Details()
	if err := expense.RemoveDetail("detail-1"); err != nil {
		t.Fatal(err)
	}
	amount, _ := common.NewDetailAmount(500)
	quantity, _ := common.NewQuantity(1)
	category, _ := common.NewCategory("daily_goods")
	newDetail, err := domain.NewExpenseDetail("detail-2", "新商品", amount, quantity, category, domain.CategorySourceUser)
	if err != nil {
		t.Fatal(err)
	}
	if err := expense.AddDetail(newDetail); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, expense, previous, time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	got, err := find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Details()) != 1 || got.Details()[0].ID() != "detail-2" || got.Details()[0].Source() != domain.RecordSourceUser {
		t.Errorf("saved details = %+v", got.Details())
	}
	userID, _ := common.NewUserID("user-1")
	month, _ := common.NewYearMonth("2026-09")
	rows, err := repo.ListByMonth(ctx, userID, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DetailID != "detail-2" || rows[0].Amount != 500 {
		t.Errorf("monthly details = %+v", rows)
	}
}

func TestMonthlySummaryVersionConditionAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	repository := infrastructure.DynamoDBMonthlySummaryRepository{Table: env.CreateTable(t, libdynamodb.MonthlySummariesSchema)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	userID, _ := common.NewUserID("user-1")
	month, _ := common.NewYearMonth("2026-09")
	summary := domain.MonthlySummary{UserID: userID, YearMonth: month, TotalRecordedAmount: 100, ExpenseCount: 1, DetailCount: 1, CategoryTotals: domain.CategoryTotals{common.CategoryFood: 100}, Version: 0}
	if err := repository.Save(ctx, summary, time.Now()); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	version, err := repository.Version(ctx, userID, month)
	if err != nil || version != 1 {
		t.Fatalf("Version() = %d, %v", version, err)
	}
	if err := repository.Save(ctx, summary, time.Now()); !errors.Is(err, application.ErrConcurrentUpdate) {
		t.Fatalf("stale Save() error = %v, want ErrConcurrentUpdate", err)
	}
}

func TestMonthlyReadModelsAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	detailsTable := env.CreateTable(t, libdynamodb.ExpenseDetailsSchema)
	summariesTable := env.CreateTable(t, libdynamodb.MonthlySummariesSchema)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	first := detailRecord("user-1", "expense-1", "detail-1", "高い品", "food", "AI", 5000, 1)
	first["GSI1SK"] = infrastructure.DetailMonthSK(5000, "2026-09-18", "detail-1")
	second := detailRecord("user-1", "expense-2", "detail-2", "安い品", "daily_goods", "USER", 1000, 2)
	second["GSI1SK"] = infrastructure.DetailMonthSK(1000, "2026-09-12", "detail-2")
	second["source"], second["is_edited"] = "USER", true
	putRecord(ctx, t, detailsTable, second)
	putRecord(ctx, t, detailsTable, first)
	putRecord(ctx, t, summariesTable, map[string]any{
		"PK": "USER#user-1", "SK": "MONTH#2026-09", "user_id": "user-1", "year_month": "2026-09",
		"total_recorded_amount": int64(5500), "expense_count": int64(2), "detail_count": int64(2), "version": int64(1),
		"category_total_food": int64(5000), "category_total_daily_goods": int64(1000), "updated_at": "2026-09-22T01:00:00Z",
	})
	userID, _ := common.NewUserID("user-1")
	month, _ := common.NewYearMonth("2026-09")
	details, err := (infrastructure.DynamoDBExpenseRepository{ExpenseDetails: detailsTable}).ListByMonth(ctx, userID, month)
	if err != nil || len(details) != 2 || details[0].DetailID != "detail-1" || details[1].Source != domain.RecordSourceUser || !details[1].IsEdited {
		t.Fatalf("ListByMonth() = %+v, %v", details, err)
	}
	summaries, err := (infrastructure.DynamoDBMonthlySummaryRepository{Table: summariesTable}).List(ctx, userID)
	if err != nil || len(summaries) != 1 || summaries[0].Summary.TotalRecordedAmount != 5500 || summaries[0].Summary.CategoryTotals[common.CategoryFood] != 5000 {
		t.Fatalf("List() = %+v, %v", summaries, err)
	}
	if len(summaries[0].Summary.CategoryTotals) != len(common.Categories()) {
		t.Errorf("category totals count = %d", len(summaries[0].Summary.CategoryTotals))
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

func TestTaxSuggestionSurvivesSaveAndCanBeConfirmed(t *testing.T) {
	t.Parallel()
	repo := newRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	putRecord(ctx, t, repo.Expenses, expenseRecord("user-1", "expense-1", "request-1", 108, 0, false))
	record := detailRecord("user-1", "expense-1", "detail-1", "パン", "food", "AI", 100, 1)
	record["tax_status"], record["tax_reason"], record["suggested_tax_rate"] = "estimated", "product_preference", 8
	putRecord(ctx, t, repo.ExpenseDetails, record)
	expense, err := find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatal(err)
	}
	previous := expense.Details()
	expense.ChangeStoreName("変更後")
	if err := repo.Save(ctx, expense, previous, time.Now()); err != nil {
		t.Fatal(err)
	}
	expense, err = find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatal(err)
	}
	d := expense.Details()[0]
	if d.TaxStatus() != "estimated" || d.SuggestedTaxRate() == nil || *d.SuggestedTaxRate() != 8 || d.TaxIncludedAmount() != nil {
		t.Fatalf("detail=%+v", d)
	}
	previous = expense.Details()
	if err := expense.ConfirmDetailTax(d.ID(), 8, "external", 108); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, expense, previous, time.Now()); err != nil {
		t.Fatal(err)
	}
	expense, err = find(ctx, repo, "user-1", "expense-1")
	if err != nil {
		t.Fatal(err)
	}
	d = expense.Details()[0]
	if d.TaxStatus() != "user_confirmed" || d.SuggestedTaxRate() != nil || d.TaxIncludedAmount() == nil || *d.TaxIncludedAmount() != 108 {
		t.Fatalf("confirmed=%+v", d)
	}
}
