package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	"snap_kakeibo/backend/internal/ledger/domain"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoDBMonthlySummaryRepository struct{ Table *libdynamodb.Table }

var _ application.MonthlySummaryRepository = DynamoDBMonthlySummaryRepository{}

func (r DynamoDBMonthlySummaryRepository) Version(ctx context.Context, userID common.UserID, month domain.YearMonth) (int64, error) {
	out, err := r.Table.GetItem(ctx, &awssdk.GetItemInput{Key: map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID.String())), "SK": stringValue(MonthSK(month.String()))}, ProjectionExpression: aws.String("version")})
	if err != nil || len(out.Item) == 0 {
		return 0, err
	}
	var item struct {
		Version int64 `dynamodbav:"version"`
	}
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return 0, fmt.Errorf("unmarshal monthly summary version: %w", err)
	}
	return item.Version, nil
}

func (r DynamoDBMonthlySummaryRepository) Save(ctx context.Context, summary domain.MonthlySummary, updatedAt time.Time) error {
	names := map[string]string{"#type": "type", "#version": "version"}
	values := map[string]ddbtypes.AttributeValue{
		":type": stringValue("MONTHLY_SUMMARY"), ":user": stringValue(summary.UserID.String()), ":month": stringValue(summary.YearMonth.String()),
		":total": numberValue(summary.TotalRecordedAmount), ":expenses": numberValue(summary.ExpenseCount), ":details": numberValue(summary.DetailCount),
		":expected": numberValue(summary.Version), ":zero": numberValue(0), ":one": numberValue(1), ":updated": stringValue(updatedAt.UTC().Format(time.RFC3339)),
	}
	sets := []string{"#type=:type", "user_id=:user", "year_month=:month", "total_recorded_amount=:total", "expense_count=:expenses", "detail_count=:details", "updated_at=:updated", "#version=if_not_exists(#version,:zero)+:one"}
	for i, category := range common.Categories() {
		name, value := fmt.Sprintf("#c%d", i), fmt.Sprintf(":c%d", i)
		names[name] = "category_total_" + category.String()
		values[value] = numberValue(summary.CategoryTotals[category])
		sets = append(sets, name+"="+value)
	}
	_, err := r.Table.UpdateItem(ctx, &awssdk.UpdateItemInput{
		Key:                 map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(summary.UserID.String())), "SK": stringValue(MonthSK(summary.YearMonth.String()))},
		UpdateExpression:    aws.String("SET " + strings.Join(sets, ",")),
		ConditionExpression: aws.String("attribute_not_exists(PK) OR #version=:expected"), ExpressionAttributeNames: names, ExpressionAttributeValues: values,
	})
	if libdynamodb.IsConditionalCheckFailed(err) {
		return application.ErrConcurrentUpdate
	}
	return err
}
