package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/app"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// レコードの type 属性と、AI 解析由来であることを表す source 値。
const (
	recordTypeBilling        = "BILLING"
	recordTypeBillingDetail  = "BILLING_DETAIL"
	recordTypeMonthlySummary = "MONTHLY_SUMMARY"
	sourceAI                 = "AI"
)

// billingRecord は billings テーブルのアイテム。
type billingRecord struct {
	PK             string `dynamodbav:"PK"`
	SK             string `dynamodbav:"SK"`
	Type           string `dynamodbav:"type"`
	BillingID      string `dynamodbav:"billing_id"`
	UploadID       string `dynamodbav:"upload_id"`
	StoreName      string `dynamodbav:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month"`
	Source         string `dynamodbav:"source"`
	CreatedAt      string `dynamodbav:"created_at"`
	UpdatedAt      string `dynamodbav:"updated_at"`
	OriginalAmount int64  `dynamodbav:"original_amount"`
	DiscountAmount int64  `dynamodbav:"discount_amount"`
	FinalAmount    int64  `dynamodbav:"final_amount"`
	IsEdited       bool   `dynamodbav:"is_edited"`
}

// billingDetailRecord は billing_details テーブルのアイテム。GSI1 で月ごとに金額順で引く。
type billingDetailRecord struct {
	PK             string `dynamodbav:"PK"`
	SK             string `dynamodbav:"SK"`
	GSI1PK         string `dynamodbav:"GSI1PK"`
	GSI1SK         string `dynamodbav:"GSI1SK"`
	Type           string `dynamodbav:"type"`
	DetailID       string `dynamodbav:"detail_id"`
	BillingID      string `dynamodbav:"billing_id"`
	UploadID       string `dynamodbav:"upload_id"`
	Name           string `dynamodbav:"name"`
	Category       string `dynamodbav:"category"`
	CategorySource string `dynamodbav:"category_source"`
	Source         string `dynamodbav:"source"`
	StoreName      string `dynamodbav:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month"`
	CreatedAt      string `dynamodbav:"created_at"`
	UpdatedAt      string `dynamodbav:"updated_at"`
	Amount         int64  `dynamodbav:"amount"`
	Quantity       int64  `dynamodbav:"quantity"`
	IsEdited       bool   `dynamodbav:"is_edited"`
}

// DynamoDBBillingRegistrar は請求・明細・月次集計の登録と upload_histories の SUCCEEDED への遷移を
// 1 つの TransactWriteItems で行う。途中で失敗しても半端な状態にならない。
type DynamoDBBillingRegistrar struct {
	Client           *libdynamodb.Client
	UploadHistories  *libdynamodb.Table
	Billings         *libdynamodb.Table
	BillingDetails   *libdynamodb.Table
	MonthlySummaries *libdynamodb.Table
}

var _ application.BillingRegistrar = DynamoDBBillingRegistrar{}

// Register は請求を登録する。
//   - upload_histories: ANALYZING かつ attempt 一致のときだけ SUCCEEDED にし billing_id を書く
//   - billings / billing_details: 新規のときだけ Put
//   - monthly_summaries: total_amount / billing_count / detail_count / category_total_* を加算
//
// upload_histories の条件不一致は既に別の処理が終端状態にしたとみなし、成功として扱う。
// 登録内容が DynamoDB に拒否された(ValidationError)場合は ErrBillingRejected を返す。
func (r DynamoDBBillingRegistrar) Register(ctx context.Context, job domain.Job, billing domain.Billing, rawResultKey string, now time.Time) error {
	items, err := r.transactItems(job, billing, rawResultKey, now)
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
		return fmt.Errorf("%w: %w", application.ErrBillingRejected, err)
	}
	return err
}

func (r DynamoDBBillingRegistrar) transactItems(job domain.Job, billing domain.Billing, rawResultKey string, now time.Time) ([]ddbtypes.TransactWriteItem, error) {
	timestamp := formatTime(billing.CreatedAt)
	month := billing.YearMonth()

	billingItem, err := attributevalue.MarshalMap(billingRecord{
		PK: app.UserPK(billing.UserID), SK: app.BillingSK(billing.ID), Type: recordTypeBilling,
		BillingID: billing.ID, UploadID: billing.UploadID, StoreName: billing.StoreName, PurchasedAt: billing.PurchasedAt, YearMonth: month,
		OriginalAmount: billing.TotalAmount, FinalAmount: billing.TotalAmount, Source: sourceAI, CreatedAt: timestamp, UpdatedAt: timestamp,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal billing: %w", err)
	}

	values := terminalValues(job, string(domain.StatusSucceeded), rawResultKey, now)
	values[":billing_id"] = stringValue(billing.ID)
	update := "SET #status=:status,billing_id=:billing_id,updated_at=:now"
	if rawResultKey != "" {
		update += ",raw_result_s3_key=:raw_key"
	}
	items := []ddbtypes.TransactWriteItem{
		{Update: &ddbtypes.Update{
			TableName:                 aws.String(r.UploadHistories.Name()),
			Key:                       uploadKey(job),
			UpdateExpression:          aws.String(update),
			ConditionExpression:       aws.String(terminalCondition),
			ExpressionAttributeNames:  map[string]string{"#status": "status"},
			ExpressionAttributeValues: values,
		}},
		{Put: &ddbtypes.Put{TableName: aws.String(r.Billings.Name()), Item: billingItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}},
	}
	for _, d := range billing.Details {
		detailItem, err := attributevalue.MarshalMap(billingDetailRecord{
			PK: app.DetailPK(billing.UserID, billing.ID), SK: app.DetailSK(d.ID),
			GSI1PK: app.UploadMonthPK(billing.UserID, month), GSI1SK: app.DetailMonthSK(d.Amount, billing.PurchasedAt, d.ID),
			Type: recordTypeBillingDetail, DetailID: d.ID, BillingID: billing.ID, UploadID: billing.UploadID,
			Name: d.Name, Category: d.Category, CategorySource: sourceAI, Amount: d.Amount, Quantity: d.Quantity, Source: sourceAI,
			StoreName: billing.StoreName, PurchasedAt: billing.PurchasedAt, YearMonth: month, CreatedAt: timestamp, UpdatedAt: timestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal billing detail: %w", err)
		}
		items = append(items, ddbtypes.TransactWriteItem{Put: &ddbtypes.Put{TableName: aws.String(r.BillingDetails.Name()), Item: detailItem, ConditionExpression: aws.String("attribute_not_exists(PK)")}})
	}
	items = append(items, r.monthlySummaryItem(billing, month, timestamp))
	return items, nil
}

// monthlySummaryItem は月次集計への加算。初回は type / user_id / year_month も埋める。
func (r DynamoDBBillingRegistrar) monthlySummaryItem(billing domain.Billing, month, timestamp string) ddbtypes.TransactWriteItem {
	names := map[string]string{"#type": "type"}
	values := map[string]ddbtypes.AttributeValue{
		":type":  stringValue(recordTypeMonthlySummary),
		":user":  stringValue(billing.UserID),
		":month": stringValue(month),
		":now":   stringValue(timestamp),
		":total": numberValue(billing.TotalAmount),
		":one":   numberValue(1),
		":count": numberValue(int64(len(billing.Details))),
	}
	adds := []string{"total_amount :total", "billing_count :one", "detail_count :count", "version :one"}
	i := 0
	for category, total := range billing.CategoryTotals() {
		i++
		name, value := fmt.Sprintf("#c%d", i), fmt.Sprintf(":c%d", i)
		names[name] = "category_total_" + category
		values[value] = numberValue(total)
		adds = append(adds, name+" "+value)
	}
	expr := "SET #type=if_not_exists(#type,:type),user_id=if_not_exists(user_id,:user),year_month=if_not_exists(year_month,:month),updated_at=:now ADD " + strings.Join(adds, ", ")
	return ddbtypes.TransactWriteItem{Update: &ddbtypes.Update{
		TableName:                 aws.String(r.MonthlySummaries.Name()),
		Key:                       map[string]ddbtypes.AttributeValue{"PK": stringValue(app.UserPK(billing.UserID)), "SK": stringValue(app.MonthSK(month))},
		UpdateExpression:          aws.String(expr),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}}
}
