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
var _ application.MonthlySummaryLister = DynamoDBMonthlySummaryRepository{}

type monthlySummaryItem struct {
	UserID              string `dynamodbav:"user_id"`
	YearMonth           string `dynamodbav:"year_month"`
	TotalRecordedAmount int64  `dynamodbav:"total_recorded_amount"`
	ExpenseCount        int64  `dynamodbav:"expense_count"`
	DetailCount         int64  `dynamodbav:"detail_count"`
	Version             int64  `dynamodbav:"version"`
	UpdatedAt           string `dynamodbav:"updated_at"`
	Food                int64  `dynamodbav:"category_total_food"`
	DailyGoods          int64  `dynamodbav:"category_total_daily_goods"`
	Medical             int64  `dynamodbav:"category_total_medical"`
	Transport           int64  `dynamodbav:"category_total_transport"`
	Utilities           int64  `dynamodbav:"category_total_utilities"`
	Entertainment       int64  `dynamodbav:"category_total_entertainment"`
	Social              int64  `dynamodbav:"category_total_social"`
	Clothing            int64  `dynamodbav:"category_total_clothing"`
	Education           int64  `dynamodbav:"category_total_education"`
	Other               int64  `dynamodbav:"category_total_other"`
	Unknown             int64  `dynamodbav:"category_total_unknown"`
}

// List は利用者の保存済み集計を全件読み、year_month 降順で返す。
func (r DynamoDBMonthlySummaryRepository) List(ctx context.Context, userID common.UserID) ([]application.MonthlySummaryItem, error) {
	result := make([]application.MonthlySummaryItem, 0)
	var cursor map[string]ddbtypes.AttributeValue
	for {
		out, err := r.Table.Query(ctx, &awssdk.QueryInput{
			KeyConditionExpression:    aws.String("PK = :pk AND begins_with(SK, :sk)"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{":pk": stringValue(UserPK(userID.String())), ":sk": stringValue("MONTH#")},
			ScanIndexForward:          aws.Bool(false), ExclusiveStartKey: cursor,
		})
		if err != nil {
			return nil, err
		}
		var items []monthlySummaryItem
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
			return nil, fmt.Errorf("unmarshal monthly summaries: %w", err)
		}
		for _, item := range items {
			month, err := common.NewYearMonth(item.YearMonth)
			if err != nil {
				return nil, fmt.Errorf("restore monthly summary month: %w", err)
			}
			updatedAt, err := time.Parse(time.RFC3339, item.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("restore monthly summary updated_at: %w", err)
			}
			totals := domain.CategoryTotals{
				common.CategoryFood: item.Food, common.CategoryDailyGoods: item.DailyGoods, common.CategoryMedical: item.Medical,
				common.CategoryTransport: item.Transport, common.CategoryUtilities: item.Utilities, common.CategoryEntertainment: item.Entertainment,
				common.CategorySocial: item.Social, common.CategoryClothing: item.Clothing, common.CategoryEducation: item.Education,
				common.CategoryOther: item.Other, common.CategoryUnknown: item.Unknown,
			}
			result = append(result, application.MonthlySummaryItem{Summary: domain.MonthlySummary{
				UserID: userID, YearMonth: domain.YearMonth(month), TotalRecordedAmount: item.TotalRecordedAmount,
				ExpenseCount: item.ExpenseCount, DetailCount: item.DetailCount, CategoryTotals: totals, Version: item.Version,
			}, UpdatedAt: updatedAt})
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		cursor = out.LastEvaluatedKey
	}
	return result, nil
}

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
