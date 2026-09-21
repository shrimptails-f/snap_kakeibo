// Package infrastructure は請求参照の application が定義した interface の DynamoDB 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"

	"snap_kakeibo/backend/internal/billing/application"
	"snap_kakeibo/backend/internal/billing/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// billingItem は billings の項目のうち画面へ返す属性。analyze-receipt が書く属性名と揃える。
type billingItem struct {
	BillingID      string `dynamodbav:"billing_id"`
	UploadID       string `dynamodbav:"upload_id"`
	StoreName      string `dynamodbav:"store_name"`
	PurchasedAt    string `dynamodbav:"purchased_at"`
	YearMonth      string `dynamodbav:"year_month"`
	OriginalAmount int64  `dynamodbav:"original_amount"`
	DiscountAmount int64  `dynamodbav:"discount_amount"`
	FinalAmount    int64  `dynamodbav:"final_amount"`
}

// DynamoDBBillingRepository は billings から請求を引く。
type DynamoDBBillingRepository struct {
	Table *libdynamodb.Table
}

var _ application.BillingFinder = DynamoDBBillingRepository{}

// FindByID は利用者と billing_id のキーで GetItem する。項目が無ければ ErrBillingNotFound。
// 他人の billing_id は利用者のパーティションに無いので同じく ErrBillingNotFound になる。
func (r DynamoDBBillingRepository) FindByID(ctx context.Context, userID, billingID string) (domain.Billing, error) {
	out, err := r.Table.GetItem(ctx, &awssdk.GetItemInput{
		Key: map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID)), "SK": stringValue(BillingSK(billingID))},
	})
	if err != nil {
		return domain.Billing{}, err
	}
	if len(out.Item) == 0 {
		return domain.Billing{}, application.ErrBillingNotFound
	}
	var item billingItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Billing{}, fmt.Errorf("unmarshal billing: %w", err)
	}
	return domain.Billing{
		ID:             item.BillingID,
		UserID:         userID,
		UploadID:       item.UploadID,
		StoreName:      item.StoreName,
		PurchasedAt:    item.PurchasedAt,
		YearMonth:      item.YearMonth,
		OriginalAmount: item.OriginalAmount,
		DiscountAmount: item.DiscountAmount,
		FinalAmount:    item.FinalAmount,
	}, nil
}

func stringValue(v string) ddbtypes.AttributeValue { return &ddbtypes.AttributeValueMemberS{Value: v} }
