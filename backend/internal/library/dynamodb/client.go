// Package dynamodb は DynamoDB をテーブル名に束縛して扱う薄いラッパーを提供する。
// 統合テストでは Client.Table にテストごとのランダムなテーブル名を渡せる。
//
// 各操作は 1 呼び出しを dynamodb_* span にし、テーブル名・インデックス名・件数だけを span_finished に載せる。
// キーや属性値はログに出さない。
//
//	client := libdynamodb.New(cfg, log)
//	requests := client.Table(cfg.AnalysisRequestsTable)
//	_, err := requests.UpdateItem(ctx, &awssdk.UpdateItemInput{...})   // dynamodb_update_item span
//	_, err = client.TransactWriteItems(ctx, &awssdk.TransactWriteItemsInput{...}) // dynamodb_transact_write span
package dynamodb

import (
	"context"
	"fmt"

	"snap_kakeibo/backend/internal/library/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// span 名のうち dynamodb が出すもの。
const (
	SpanBatchGetItem  = "dynamodb_batch_get_item"
	SpanGetItem       = "dynamodb_get_item"
	SpanPutItem       = "dynamodb_put_item"
	SpanUpdateItem    = "dynamodb_update_item"
	SpanQuery         = "dynamodb_query"
	SpanTransactWrite = "dynamodb_transact_write"
)

// API は Client が使う DynamoDB 操作。*awssdk.Client が満たす。
type API interface {
	GetItem(ctx context.Context, params *awssdk.GetItemInput, optFns ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error)
	PutItem(ctx context.Context, params *awssdk.PutItemInput, optFns ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *awssdk.UpdateItemInput, optFns ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error)
	Query(ctx context.Context, params *awssdk.QueryInput, optFns ...func(*awssdk.Options)) (*awssdk.QueryOutput, error)
	TransactWriteItems(ctx context.Context, params *awssdk.TransactWriteItemsInput, optFns ...func(*awssdk.Options)) (*awssdk.TransactWriteItemsOutput, error)
}

var _ API = (*awssdk.Client)(nil)

// BatchGetAPI は複数キー取得を使う機能だけが要求する追加能力。
// API と分けることで、他の操作だけを使うテストfakeへ不要なメソッドを要求しない。
type BatchGetAPI interface {
	BatchGetItem(ctx context.Context, params *awssdk.BatchGetItemInput, optFns ...func(*awssdk.Options)) (*awssdk.BatchGetItemOutput, error)
}

var _ BatchGetAPI = (*awssdk.Client)(nil)

// Client は DynamoDB 操作の起点。
type Client struct {
	api API
	log logger.Interface
}

// New は cfg から DynamoDB client を生成する。log が nil なら何も出力しない。
func New(cfg aws.Config, log logger.Interface) *Client {
	return NewWithAPI(awssdk.NewFromConfig(cfg), log)
}

// NewWithAPI はテスト用の API 差し替えを受け取って Client を生成する。log が nil なら何も出力しない。
func NewWithAPI(api API, log logger.Interface) *Client {
	if log == nil {
		log = logger.NewNop()
	}
	return &Client{api: api, log: log}
}

// Table は name に束縛した Table を返す。
func (c *Client) Table(name string) *Table { return &Table{client: c, name: name} }

// BatchGetItem は複数のキーをまとめて取得し、dynamodb_batch_get_item span を出す。
// RequestItems は複数テーブルを指定できるため、Table ではなく Client に置く。
func (c *Client) BatchGetItem(ctx context.Context, in *awssdk.BatchGetItemInput, optFns ...func(*awssdk.Options)) (*awssdk.BatchGetItemOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("dynamodb: BatchGetItemInput is nil")
	}
	itemCount := 0
	for _, request := range in.RequestItems {
		itemCount += len(request.Keys)
	}
	ctx, span := logger.StartSpan(ctx, c.log, SpanBatchGetItem, logger.Int("item_count", itemCount), logger.Int("table_count", len(in.RequestItems)))
	api, ok := c.api.(BatchGetAPI)
	if !ok {
		err := fmt.Errorf("dynamodb: API does not support BatchGetItem")
		span.End(err)
		return nil, err
	}
	out, err := api.BatchGetItem(ctx, in, optFns...)
	span.End(err)
	return out, err
}

// TransactWriteItems は複数テーブルにまたがる書き込みをまとめて実行し、dynamodb_transact_write span を出す。
// テーブル名は各 TransactWriteItem に指定する(Table.Name を使う)。
func (c *Client) TransactWriteItems(ctx context.Context, in *awssdk.TransactWriteItemsInput, optFns ...func(*awssdk.Options)) (*awssdk.TransactWriteItemsOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("dynamodb: TransactWriteItemsInput is nil")
	}
	ctx, span := logger.StartSpan(ctx, c.log, SpanTransactWrite, logger.Int("item_count", len(in.TransactItems)))
	out, err := c.api.TransactWriteItems(ctx, in, optFns...)
	span.End(err)
	return out, err
}

// Table は特定の DynamoDB テーブルに束縛した操作を提供する。
type Table struct {
	client *Client
	name   string
}

// Name は束縛したテーブル名を返す。
func (t *Table) Name() string { return t.name }

// GetItem は入力をこのテーブルに束縛して実行する。
func (t *Table) GetItem(ctx context.Context, in *awssdk.GetItemInput, optFns ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("dynamodb: GetItemInput is nil")
	}
	if err := t.validateTable(aws.ToString(in.TableName)); err != nil {
		return nil, err
	}
	bound := *in
	bound.TableName = aws.String(t.name)
	ctx, span := t.startSpan(ctx, SpanGetItem)
	out, err := t.client.api.GetItem(ctx, &bound, optFns...)
	if err != nil {
		span.End(err)
		return nil, err
	}
	span.End(nil, logger.Bool("found", len(out.Item) > 0))
	return out, nil
}

// PutItem は入力をこのテーブルに束縛して実行する。
func (t *Table) PutItem(ctx context.Context, in *awssdk.PutItemInput, optFns ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("dynamodb: PutItemInput is nil")
	}
	if err := t.validateTable(aws.ToString(in.TableName)); err != nil {
		return nil, err
	}
	bound := *in
	bound.TableName = aws.String(t.name)
	ctx, span := t.startSpan(ctx, SpanPutItem)
	out, err := t.client.api.PutItem(ctx, &bound, optFns...)
	span.End(err)
	return out, err
}

// UpdateItem は入力をこのテーブルに束縛して実行する。
func (t *Table) UpdateItem(ctx context.Context, in *awssdk.UpdateItemInput, optFns ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("dynamodb: UpdateItemInput is nil")
	}
	if err := t.validateTable(aws.ToString(in.TableName)); err != nil {
		return nil, err
	}
	bound := *in
	bound.TableName = aws.String(t.name)
	ctx, span := t.startSpan(ctx, SpanUpdateItem)
	out, err := t.client.api.UpdateItem(ctx, &bound, optFns...)
	span.End(err)
	return out, err
}

// Query は入力をこのテーブルに束縛して実行する。IndexName はそのまま渡す。
func (t *Table) Query(ctx context.Context, in *awssdk.QueryInput, optFns ...func(*awssdk.Options)) (*awssdk.QueryOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("dynamodb: QueryInput is nil")
	}
	if err := t.validateTable(aws.ToString(in.TableName)); err != nil {
		return nil, err
	}
	bound := *in
	bound.TableName = aws.String(t.name)
	var fields []logger.Field
	if in.IndexName != nil {
		fields = append(fields, logger.String("index_name", aws.ToString(in.IndexName)))
	}
	ctx, span := t.startSpan(ctx, SpanQuery, fields...)
	out, err := t.client.api.Query(ctx, &bound, optFns...)
	if err != nil {
		span.End(err)
		return nil, err
	}
	span.End(nil, logger.Int("item_count", len(out.Items)))
	return out, nil
}

func (t *Table) startSpan(ctx context.Context, name string, fields ...logger.Field) (context.Context, *logger.Span) {
	return logger.StartSpan(ctx, t.client.log, name, append([]logger.Field{logger.String("table_name", t.name)}, fields...)...)
}

func (t *Table) validateTable(target string) error {
	if target != "" && target != t.name {
		return fmt.Errorf("dynamodb: input targets %s but the table is bound to %s", target, t.name)
	}
	return nil
}
