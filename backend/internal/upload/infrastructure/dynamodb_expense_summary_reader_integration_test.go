package infrastructure_test

import (
	"context"
	"testing"
	"time"

	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	"snap_kakeibo/backend/internal/library/dynamodb/dynamodbtest"
	"snap_kakeibo/backend/internal/upload/infrastructure"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestFindExpenseSummariesAgainstDynamoDB(t *testing.T) {
	t.Parallel()
	env := dynamodbtest.Connect(t)
	expenses := env.CreateTable(t, libdynamodb.ExpensesSchema)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	for _, item := range []map[string]ddbtypes.AttributeValue{
		{"PK": &ddbtypes.AttributeValueMemberS{Value: "USER#u1"}, "SK": &ddbtypes.AttributeValueMemberS{Value: "EXPENSE#e1"}, "expense_id": &ddbtypes.AttributeValueMemberS{Value: "e1"}, "store_name": &ddbtypes.AttributeValueMemberS{Value: "スーパー"}, "recorded_amount": &ddbtypes.AttributeValueMemberN{Value: "2780"}},
		{"PK": &ddbtypes.AttributeValueMemberS{Value: "USER#u2"}, "SK": &ddbtypes.AttributeValueMemberS{Value: "EXPENSE#e2"}, "expense_id": &ddbtypes.AttributeValueMemberS{Value: "e2"}, "store_name": &ddbtypes.AttributeValueMemberS{Value: "他人の店"}, "recorded_amount": &ddbtypes.AttributeValueMemberN{Value: "999"}},
	} {
		if _, err := expenses.PutItem(ctx, &awssdk.PutItemInput{Item: item, ConditionExpression: aws.String("attribute_not_exists(PK)")}); err != nil {
			t.Fatal(err)
		}
	}

	reader := infrastructure.DynamoDBAnalysisRequestExpenseReader{Client: env.Client, Expenses: expenses}
	got, err := reader.FindSummaries(ctx, "u1", []string{"e1", "e1", "e2", "missing"})
	if err != nil {
		t.Fatalf("FindSummaries() error = %v", err)
	}
	if len(got) != 1 || got["e1"].StoreName != "スーパー" || got["e1"].RecordedAmount != 2780 {
		t.Errorf("FindSummaries() = %+v", got)
	}
}
