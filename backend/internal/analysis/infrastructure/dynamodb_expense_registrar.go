package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	ledgerdomain "snap_kakeibo/backend/internal/ledger/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// レコードの type 属性と、AI 解析由来であることを表す source 値。
const (
	recordTypeExpense        = "EXPENSE"
	recordTypeExpenseDetail  = "EXPENSE_DETAIL"
	recordTypeMonthlySummary = "MONTHLY_SUMMARY"
	sourceAI                 = "AI"
)

// expenseRecord は expenses テーブルのアイテム。
// recorded_amount は read_amount + adjustment_amount の導出値だが、月次再構築や一覧で再計算せずに読めるよう保存する。
type expenseRecord struct {
	PK                string `dynamodbav:"PK"`
	SK                string `dynamodbav:"SK"`
	Type              string `dynamodbav:"type"`
	ExpenseID         string `dynamodbav:"expense_id"`
	AnalysisRequestID string `dynamodbav:"analysis_request_id"`
	StoreName         string `dynamodbav:"store_name"`
	PurchaseDate      string `dynamodbav:"purchase_date"`
	YearMonth         string `dynamodbav:"year_month"`
	Source            string `dynamodbav:"source"`
	CreatedAt         string `dynamodbav:"created_at"`
	UpdatedAt         string `dynamodbav:"updated_at"`
	ReadAmount        int64  `dynamodbav:"read_amount"`
	AnalysisEvidence  string `dynamodbav:"analysis_evidence,omitempty"`
	AdjustmentAmount  int64  `dynamodbav:"adjustment_amount"`
	RecordedAmount    int64  `dynamodbav:"recorded_amount"`
	IsEdited          bool   `dynamodbav:"is_edited"`
}

// expenseDetailRecord は expense_details テーブルのアイテム。GSI1 で月ごとに金額順で引く。
type expenseDetailRecord struct {
	PK                string `dynamodbav:"PK"`
	SK                string `dynamodbav:"SK"`
	GSI1PK            string `dynamodbav:"GSI1PK"`
	GSI1SK            string `dynamodbav:"GSI1SK"`
	Type              string `dynamodbav:"type"`
	DetailID          string `dynamodbav:"detail_id"`
	ExpenseID         string `dynamodbav:"expense_id"`
	AnalysisRequestID string `dynamodbav:"analysis_request_id"`
	Name              string `dynamodbav:"name"`
	Category          string `dynamodbav:"category"`
	CategorySource    string `dynamodbav:"category_source"`
	Source            string `dynamodbav:"source"`
	StoreName         string `dynamodbav:"store_name"`
	PurchaseDate      string `dynamodbav:"purchase_date"`
	YearMonth         string `dynamodbav:"year_month"`
	CreatedAt         string `dynamodbav:"created_at"`
	UpdatedAt         string `dynamodbav:"updated_at"`
	Amount            int64  `dynamodbav:"amount"`
	TaxIncludedAmount *int64 `dynamodbav:"tax_included_amount,omitempty"`
	TaxRate           *int64 `dynamodbav:"tax_rate,omitempty"`
	TaxMode           string `dynamodbav:"tax_mode,omitempty"`
	TaxAllocation     string `dynamodbav:"tax_allocation,omitempty"`
	TaxStatus         string `dynamodbav:"tax_status,omitempty"`
	TaxReason         string `dynamodbav:"tax_reason,omitempty"`
	SuggestedTaxRate  *int64 `dynamodbav:"suggested_tax_rate,omitempty"`
	Quantity          int64  `dynamodbav:"quantity"`
	IsEdited          bool   `dynamodbav:"is_edited"`
}

// DynamoDBExpenseRegistrar は支出・支出明細・月次集計の登録と analysis_requests の SUCCEEDED への遷移を
// 1 つの TransactWriteItems で行う。途中で失敗しても半端な状態にならない。
type DynamoDBExpenseRegistrar struct {
	Client           *libdynamodb.Client
	AnalysisRequests *libdynamodb.Table
	Expenses         *libdynamodb.Table
	ExpenseDetails   *libdynamodb.Table
	MonthlySummaries *libdynamodb.Table
}

var _ application.ExpenseRegistrar = DynamoDBExpenseRegistrar{}

// Register は支出を登録する。
//   - analysis_requests: ANALYZING かつ attempt 一致のときだけ SUCCEEDED にし expense_id を書く
//   - expenses / expense_details: 新規のときだけ Put
//   - monthly_summaries: total_recorded_amount / expense_count / detail_count / category_total_* を加算
//
// analysis_requests の条件不一致は既に別の処理が終端状態にしたとみなし、成功として扱う。
// 登録内容が DynamoDB に拒否された(ValidationError)場合は ErrExpenseRejected を返す。
func (r DynamoDBExpenseRegistrar) Register(ctx context.Context, job domain.AnalysisJob, expense ledgerdomain.Expense, rawResultKey string, now time.Time) error {
	items, err := r.transactItems(job, expense, rawResultKey, now)
	if err != nil {
		return err
	}
	_, err = r.Client.TransactWriteItems(ctx, &awssdk.TransactWriteItemsInput{TransactItems: items})
	switch {
	case err == nil:
		return nil
	case libdynamodb.IsConditionalCheckFailed(err):
		return nil
	case libdynamodb.IsTransactionValidationFailed(err):
		return fmt.Errorf("%w: %w", application.ErrExpenseRejected, err)
	}
	return err
}

func (r DynamoDBExpenseRegistrar) transactItems(job domain.AnalysisJob, expense ledgerdomain.Expense, rawResultKey string, now time.Time) ([]ddbtypes.TransactWriteItem, error) {
	timestamp := formatTime(now)
	userID, expenseID := expense.UserID().String(), expense.ID().String()
	purchaseDate := expense.PurchaseDate().String()
	month := expense.PurchaseDate().YearMonth().String()

	expenseItem, err := attributevalue.MarshalMap(expenseRecord{
		PK: UserPK(userID), SK: ExpenseSK(expenseID), Type: recordTypeExpense,
		ExpenseID: expenseID, AnalysisRequestID: expense.SourceRequestID().String(), StoreName: expense.StoreName(), PurchaseDate: purchaseDate, YearMonth: month,
		ReadAmount: expense.ReadAmount().Yen(), AnalysisEvidence: expense.AnalysisEvidence(), AdjustmentAmount: expense.AdjustmentAmount().Yen(), RecordedAmount: expense.RecordedAmount().Yen(),
		IsEdited: expense.Edited(), Source: sourceAI, CreatedAt: timestamp, UpdatedAt: timestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal expense: %w", err)
	}

	values := terminalValues(job, string(domain.AnalysisStatusSucceeded), rawResultKey, now)
	values[":expense_id"] = stringValue(expenseID)
	update := "SET #status=:status,expense_id=:expense_id,updated_at=:now"
	if rawResultKey != "" {
		update += ",raw_result_s3_key=:raw_key"
	}
	items := []ddbtypes.TransactWriteItem{
		{Update: &ddbtypes.Update{
			TableName:                 aws.String(r.AnalysisRequests.Name()),
			Key:                       requestKey(job),
			UpdateExpression:          aws.String(update),
			ConditionExpression:       aws.String(terminalCondition),
			ExpressionAttributeNames:  map[string]string{"#status": "status"},
			ExpressionAttributeValues: values,
		}},
		{Put: &ddbtypes.Put{TableName: aws.String(r.Expenses.Name()), Item: expenseItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
	}
	details := expense.Details()
	for _, d := range details {
		detailID := d.ID().String()
		detailItem, err := attributevalue.MarshalMap(expenseDetailRecord{
			PK: DetailPK(userID, expenseID), SK: DetailSK(detailID),
			GSI1PK: UserMonthPK(userID, month), GSI1SK: DetailMonthSK(d.ReportingAmount(), purchaseDate, detailID),
			Type: recordTypeExpenseDetail, DetailID: detailID, ExpenseID: expenseID, AnalysisRequestID: expense.SourceRequestID().String(),
			Name: d.Name(), Category: d.Category().String(), CategorySource: d.CategorySource().String(), Amount: d.Amount().Yen(), TaxIncludedAmount: d.TaxIncludedAmount(), TaxRate: d.TaxRate(), TaxMode: d.TaxMode(), TaxAllocation: d.TaxAllocation(), TaxStatus: d.TaxStatus(), TaxReason: d.TaxReason(), SuggestedTaxRate: d.SuggestedTaxRate(), Quantity: d.Quantity().Int64(), Source: sourceAI,
			StoreName: expense.StoreName(), PurchaseDate: purchaseDate, YearMonth: month, CreatedAt: timestamp, UpdatedAt: timestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal expense detail: %w", err)
		}
		items = append(items, ddbtypes.TransactWriteItem{Put: &ddbtypes.Put{TableName: aws.String(r.ExpenseDetails.Name()), Item: detailItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}})
	}
	items = append(items, r.monthlySummaryItem(expense, month, timestamp))
	return items, nil
}

// monthlySummaryItem は月次集計への加算。初回は type / user_id / year_month も埋める。
func (r DynamoDBExpenseRegistrar) monthlySummaryItem(expense ledgerdomain.Expense, month, timestamp string) ddbtypes.TransactWriteItem {
	details := expense.Details()
	names := map[string]string{"#type": "type"}
	values := map[string]ddbtypes.AttributeValue{
		":type":      stringValue(recordTypeMonthlySummary),
		":user":      stringValue(expense.UserID().String()),
		":month":     stringValue(month),
		":now":       stringValue(timestamp),
		":total":     numberValue(expense.RecordedAmount().Yen()),
		":one":       numberValue(1),
		":count":     numberValue(int64(len(details))),
		":confirmed": numberValue(confirmedDetailCount(details)),
	}
	adds := []string{"total_recorded_amount :total", "expense_count :one", "detail_count :count", "confirmed_detail_count :confirmed", "version :one"}
	i := 0
	for category, total := range categoryTotals(details) {
		i++
		name, value := fmt.Sprintf("#c%d", i), fmt.Sprintf(":c%d", i)
		names[name] = "category_total_" + category
		values[value] = numberValue(total)
		adds = append(adds, name+" "+value)
	}
	expr := "SET #type=if_not_exists(#type,:type),user_id=if_not_exists(user_id,:user),year_month=if_not_exists(year_month,:month),updated_at=:now ADD " + strings.Join(adds, ", ")
	return ddbtypes.TransactWriteItem{Update: &ddbtypes.Update{
		TableName:                 aws.String(r.MonthlySummaries.Name()),
		Key:                       map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(expense.UserID().String())), "SK": stringValue(MonthSK(month))},
		UpdateExpression:          aws.String(expr),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}}
}

// categoryTotals はカテゴリごとの明細金額の合計。登場したカテゴリだけを返し、月次集計の category_total_<category> に加算する。
func categoryTotals(details []ledgerdomain.ExpenseDetail) map[string]int64 {
	totals := map[string]int64{}
	for _, d := range details {
		totals[d.Category().String()] += d.ReportingAmount()
	}
	return totals
}

func confirmedDetailCount(details []ledgerdomain.ExpenseDetail) int64 {
	var count int64
	for _, detail := range details {
		if detail.TaxIncludedAmount() != nil {
			count++
		}
	}
	return count
}
