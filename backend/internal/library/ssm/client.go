// Package ssm は SSM Parameter Store をパラメータ名に束縛して扱う薄いラッパーを提供する。
// 統合テストでは Client.Parameter にテストごとのランダムな名前を渡せる。
package ssm

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdk "github.com/aws/aws-sdk-go-v2/service/ssm"
)

// API は Client が使う SSM 操作。*awssdk.Client が満たす。
type API interface {
	GetParameter(ctx context.Context, params *awssdk.GetParameterInput, optFns ...func(*awssdk.Options)) (*awssdk.GetParameterOutput, error)
}

var _ API = (*awssdk.Client)(nil)

// Client は SSM Parameter Store 操作の起点。
type Client struct{ api API }

// New は cfg から SSM client を生成する。
func New(cfg aws.Config) *Client { return NewWithAPI(awssdk.NewFromConfig(cfg)) }

// NewWithAPI はテスト用の API 差し替えを受け取って Client を生成する。
func NewWithAPI(api API) *Client { return &Client{api: api} }

// Parameter は name に束縛した Parameter を返す。
func (c *Client) Parameter(name string) *Parameter { return &Parameter{client: c, name: name} }

// Parameter は特定の SSM パラメータに束縛した操作を提供する。
type Parameter struct {
	client *Client
	name   string
}

// Name は束縛したパラメータ名を返す。
func (p *Parameter) Name() string { return p.name }

// Get は復号済みのパラメータ値を返す。
func (p *Parameter) Get(ctx context.Context) (string, error) {
	out, err := p.client.api.GetParameter(ctx, &awssdk.GetParameterInput{Name: aws.String(p.name), WithDecryption: aws.Bool(true)})
	if err != nil {
		return "", err
	}
	if out.Parameter == nil || aws.ToString(out.Parameter.Value) == "" {
		return "", fmt.Errorf("ssm: parameter %s is empty", p.name)
	}
	return aws.ToString(out.Parameter.Value), nil
}

// Reader はパラメータ値を読み取る認証などの利用側の契約。
type Reader interface {
	Get(ctx context.Context) (string, error)
}

var _ Reader = (*Parameter)(nil)
