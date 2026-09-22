package infrastructure

import (
	"context"
	"fmt"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/upload/application"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const maxBatchGetAttempts = 3

type expenseSummaryItem struct {
	ExpenseID      string `dynamodbav:"expense_id"`
	StoreName      string `dynamodbav:"store_name"`
	RecordedAmount int64  `dynamodbav:"recorded_amount"`
}

// DynamoDBAnalysisRequestExpenseReader は解析履歴に必要な支出の表示項目だけを expenses から一括取得する。
type DynamoDBAnalysisRequestExpenseReader struct {
	Client   *libdynamodb.Client
	Expenses *libdynamodb.Table
}

var _ application.AnalysisRequestExpenseReader = DynamoDBAnalysisRequestExpenseReader{}

// FindSummaries は同じ利用者の expense_id を BatchGetItem し、見つかった支出だけをIDで返す。
func (r DynamoDBAnalysisRequestExpenseReader) FindSummaries(ctx context.Context, userID string, expenseIDs []string) (map[string]application.ExpenseSummary, error) {
	result := make(map[string]application.ExpenseSummary, len(expenseIDs))
	seen := make(map[string]struct{}, len(expenseIDs))
	keys := make([]map[string]ddbtypes.AttributeValue, 0, len(expenseIDs))
	for _, expenseID := range expenseIDs {
		if expenseID == "" {
			continue
		}
		if _, ok := seen[expenseID]; ok {
			continue
		}
		seen[expenseID] = struct{}{}
		keys = append(keys, map[string]ddbtypes.AttributeValue{"PK": stringValue(UserPK(userID)), "SK": stringValue(ExpenseSK(expenseID))})
	}
	if len(keys) == 0 {
		return result, nil
	}

	requestItems := map[string]ddbtypes.KeysAndAttributes{
		r.Expenses.Name(): {Keys: keys, ProjectionExpression: aws.String("expense_id, store_name, recorded_amount")},
	}
	for attempt := 0; attempt < maxBatchGetAttempts && len(requestItems) > 0; attempt++ {
		out, err := r.Client.BatchGetItem(ctx, &awssdk.BatchGetItemInput{RequestItems: requestItems})
		if err != nil {
			return nil, err
		}
		var items []expenseSummaryItem
		if err := attributevalue.UnmarshalListOfMaps(out.Responses[r.Expenses.Name()], &items); err != nil {
			return nil, fmt.Errorf("unmarshal expense summaries: %w", err)
		}
		for _, item := range items {
			result[item.ExpenseID] = application.ExpenseSummary{ExpenseID: item.ExpenseID, StoreName: item.StoreName, RecordedAmount: item.RecordedAmount}
		}
		requestItems = out.UnprocessedKeys
	}
	if len(requestItems) > 0 {
		return nil, fmt.Errorf("batch get expense summaries: unprocessed keys remain")
	}
	return result, nil
}
