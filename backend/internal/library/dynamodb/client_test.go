package dynamodb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"snap_kakeibo/backend/internal/library/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

func TestTableBindsNameToRequest(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	table := NewWithAPI(api, nil).Table("scenario-users-123")
	if _, err := table.GetItem(context.Background(), &awssdk.GetItemInput{}); err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got := aws.ToString(api.get.TableName); got != table.Name() {
		t.Errorf("table name = %q, want %q", got, table.Name())
	}
	if _, err := table.GetItem(context.Background(), &awssdk.GetItemInput{TableName: aws.String("another-table")}); err == nil {
		t.Error("GetItem() accepted a different table name")
	}
	if _, err := table.Query(context.Background(), &awssdk.QueryInput{IndexName: aws.String("gsi")}); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got := aws.ToString(api.query.TableName); got != table.Name() {
		t.Errorf("query table name = %q, want %q", got, table.Name())
	}
	if got := aws.ToString(api.query.IndexName); got != "gsi" {
		t.Errorf("query index name = %q, want gsi", got)
	}
}

// TestOperationsEmitSpans は各操作が dynamodb_* span を出し、テーブル名と件数だけを載せ、キーや属性値を載せないことを確認する。
func TestOperationsEmitSpans(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{transactErr: errors.New("boom")}
	var buf bytes.Buffer
	log := logger.New(logger.Options{Level: "debug", Service: "test", Environment: "test", Writer: &buf})
	client := NewWithAPI(api, log)
	table := client.Table("scenario-uploads-1")
	ctx := logger.ContextWith(context.Background(), logger.AnalysisRequestID("up1"))
	key := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "USER#secret-user"}}

	if _, err := table.UpdateItem(ctx, &awssdk.UpdateItemInput{Key: key, UpdateExpression: aws.String("SET a=:a")}); err != nil {
		t.Fatalf("UpdateItem() error = %v", err)
	}
	if _, err := table.Query(ctx, &awssdk.QueryInput{IndexName: aws.String("gsi")}); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if _, err := client.BatchGetItem(ctx, &awssdk.BatchGetItemInput{RequestItems: map[string]types.KeysAndAttributes{
		"expenses": {Keys: []map[string]types.AttributeValue{key}},
	}}); err != nil {
		t.Fatalf("BatchGetItem() error = %v", err)
	}
	if _, err := client.TransactWriteItems(ctx, &awssdk.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{{}, {}}}); err == nil {
		t.Fatal("TransactWriteItems() error = nil, want boom")
	}

	finished := finishedSpans(t, &buf)
	if len(finished) != 4 {
		t.Fatalf("span_finished = %d, want 4: %s", len(finished), buf.String())
	}
	update, query, batch, transact := finished[0], finished[1], finished[2], finished[3]
	assertField(t, update, "span_name", SpanUpdateItem)
	assertField(t, update, "table_name", "scenario-uploads-1")
	assertField(t, update, "status", logger.StatusOK)
	assertField(t, update, "analysis_request_id", "up1")
	assertField(t, query, "span_name", SpanQuery)
	assertField(t, query, "index_name", "gsi")
	assertField(t, query, "item_count", float64(1))
	assertField(t, batch, "span_name", SpanBatchGetItem)
	assertField(t, batch, "item_count", float64(1))
	assertField(t, batch, "table_count", float64(1))
	assertField(t, transact, "span_name", SpanTransactWrite)
	assertField(t, transact, "item_count", float64(2))
	assertField(t, transact, "status", logger.StatusError)
	if strings.Contains(buf.String(), "secret-user") {
		t.Fatalf("keys must not be logged: %s", buf.String())
	}
}

func TestNilInputs(t *testing.T) {
	t.Parallel()
	client := NewWithAPI(&fakeAPI{}, nil)
	if _, err := client.TransactWriteItems(context.Background(), nil); err == nil {
		t.Fatal("nil TransactWriteItemsInput must be an error")
	}
	if _, err := client.BatchGetItem(context.Background(), nil); err == nil {
		t.Fatal("nil BatchGetItemInput must be an error")
	}
	if _, err := client.Table("t").UpdateItem(context.Background(), nil); err == nil {
		t.Fatal("nil UpdateItemInput must be an error")
	}
}

func TestIsConditionalCheckFailed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed exception", &types.ConditionalCheckFailedException{}, true},
		{"wrapped typed exception", fmt.Errorf("update: %w", &types.ConditionalCheckFailedException{}), true},
		{"transaction canceled by condition", &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("None")}, {Code: aws.String("ConditionalCheckFailed")}}}, true},
		{"transaction canceled by validation", &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("ValidationError")}}}, false},
		{"generic api error", &smithy.GenericAPIError{Code: "ConditionalCheckFailedException"}, true},
		{"generic transaction error with reason in message", &smithy.GenericAPIError{Code: "TransactionCanceledException", Message: "Transaction cancelled, please refer cancellation reasons for specific reasons [None, ConditionalCheckFailed]"}, true},
		{"generic transaction error without condition", &smithy.GenericAPIError{Code: "TransactionCanceledException", Message: "[ValidationError]"}, false},
		{"other error", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsConditionalCheckFailed(tt.err); got != tt.want {
				t.Errorf("IsConditionalCheckFailed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransactionValidationFailed(t *testing.T) {
	t.Parallel()
	validation := &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("None")}, {Code: aws.String("ValidationError")}}}
	if !IsTransactionValidationFailed(fmt.Errorf("register: %w", validation)) {
		t.Error("IsTransactionValidationFailed() = false for a ValidationError cancellation")
	}
	conditional := &types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}}}
	if IsTransactionValidationFailed(conditional) {
		t.Error("IsTransactionValidationFailed() = true for a ConditionalCheckFailed cancellation")
	}
	if IsTransactionValidationFailed(errors.New("boom")) {
		t.Error("IsTransactionValidationFailed() = true for an unrelated error")
	}
}

type fakeAPI struct {
	batchGet    *awssdk.BatchGetItemInput
	get         *awssdk.GetItemInput
	query       *awssdk.QueryInput
	transactErr error
}

func (f *fakeAPI) BatchGetItem(_ context.Context, in *awssdk.BatchGetItemInput, _ ...func(*awssdk.Options)) (*awssdk.BatchGetItemOutput, error) {
	f.batchGet = in
	return &awssdk.BatchGetItemOutput{}, nil
}

func (f *fakeAPI) GetItem(_ context.Context, in *awssdk.GetItemInput, _ ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error) {
	f.get = in
	return &awssdk.GetItemOutput{}, nil
}

func (f *fakeAPI) PutItem(context.Context, *awssdk.PutItemInput, ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error) {
	return &awssdk.PutItemOutput{}, nil
}

func (f *fakeAPI) UpdateItem(context.Context, *awssdk.UpdateItemInput, ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error) {
	return &awssdk.UpdateItemOutput{}, nil
}

func (f *fakeAPI) Query(_ context.Context, in *awssdk.QueryInput, _ ...func(*awssdk.Options)) (*awssdk.QueryOutput, error) {
	f.query = in
	return &awssdk.QueryOutput{Items: []map[string]types.AttributeValue{{}}}, nil
}

func (f *fakeAPI) TransactWriteItems(context.Context, *awssdk.TransactWriteItemsInput, ...func(*awssdk.Options)) (*awssdk.TransactWriteItemsOutput, error) {
	return &awssdk.TransactWriteItemsOutput{}, f.transactErr
}

func finishedSpans(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var finished []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		if entry["event"] == logger.EventSpanFinished {
			finished = append(finished, entry)
		}
	}
	return finished
}

func assertField(t *testing.T, entry map[string]any, key string, want any) {
	t.Helper()
	if got := entry[key]; got != want {
		t.Errorf("%s = %v, want %v (entry=%v)", key, got, want, entry)
	}
}
