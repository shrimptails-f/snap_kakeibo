// Package dynamodb は DynamoDB をテーブル名に束縛して扱う薄いラッパーを提供する。
// 統合テストでは Client.Table にテストごとのランダムなテーブル名を渡せる。
package dynamodb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// API は Client が使う DynamoDB 操作。*awssdk.Client が満たす。
type API interface {
	GetItem(ctx context.Context, params *awssdk.GetItemInput, optFns ...func(*awssdk.Options)) (*awssdk.GetItemOutput, error)
	PutItem(ctx context.Context, params *awssdk.PutItemInput, optFns ...func(*awssdk.Options)) (*awssdk.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *awssdk.UpdateItemInput, optFns ...func(*awssdk.Options)) (*awssdk.UpdateItemOutput, error)
}

var _ API = (*awssdk.Client)(nil)

// Client は DynamoDB 操作の起点。
type Client struct{ api API }

// New は cfg から DynamoDB client を生成する。
func New(cfg aws.Config) *Client { return NewWithAPI(awssdk.NewFromConfig(cfg)) }

// NewWithAPI はテスト用の API 差し替えを受け取って Client を生成する。
func NewWithAPI(api API) *Client { return &Client{api: api} }

// Table は name に束縛した Table を返す。
func (c *Client) Table(name string) *Table { return &Table{client: c, name: name} }

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
	return t.client.api.GetItem(ctx, &bound, optFns...)
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
	return t.client.api.PutItem(ctx, &bound, optFns...)
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
	return t.client.api.UpdateItem(ctx, &bound, optFns...)
}

func (t *Table) validateTable(target string) error {
	if target != "" && target != t.name {
		return fmt.Errorf("dynamodb: input targets %s but the table is bound to %s", target, t.name)
	}
	return nil
}
