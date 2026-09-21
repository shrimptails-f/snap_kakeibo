package infrastructure

import (
	"context"
	"fmt"

	"snap_kakeibo/backend/internal/billing/application"
	"snap_kakeibo/backend/internal/billing/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// billingDetailItem は billing_details の項目のうち画面へ返す属性。analyze-receipt が書く属性名と揃える。
type billingDetailItem struct {
	DetailID       string `dynamodbav:"detail_id"`
	Name           string `dynamodbav:"name"`
	Category       string `dynamodbav:"category"`
	CategorySource string `dynamodbav:"category_source"`
	Amount         int64  `dynamodbav:"amount"`
	Quantity       int64  `dynamodbav:"quantity"`
}

// DynamoDBBillingDetailRepository は billing_details から請求の明細を引く。
type DynamoDBBillingDetailRepository struct {
	Table *libdynamodb.Table
}

var _ application.BillingDetailLister = DynamoDBBillingDetailRepository{}

// ListByBilling は請求のパーティション(USER#{user_id}#BILLING#{billing_id})を Query し、SK(DETAIL#{detail_id})の昇順で返す。
// detail_id は ULID なので採番順になる。1 請求の明細は最大 50 件で 1 回の Query に収まるのでページネーションはしない。
func (r DynamoDBBillingDetailRepository) ListByBilling(ctx context.Context, userID, billingID string) ([]domain.BillingDetail, error) {
	out, err := r.Table.Query(ctx, &awssdk.QueryInput{
		KeyConditionExpression:    aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(DetailPK(userID, billingID))},
	})
	if err != nil {
		return nil, err
	}
	var items []billingDetailItem
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, fmt.Errorf("unmarshal billing details: %w", err)
	}
	details := make([]domain.BillingDetail, 0, len(items))
	for _, item := range items {
		details = append(details, domain.BillingDetail{
			ID:             item.DetailID,
			Name:           item.Name,
			Category:       item.Category,
			CategorySource: item.CategorySource,
			Amount:         item.Amount,
			Quantity:       item.Quantity,
		})
	}
	return details, nil
}
