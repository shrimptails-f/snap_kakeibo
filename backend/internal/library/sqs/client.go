// Package sqs は aws-sdk-go-v2 の SQS クライアントを trace 伝搬とログ付きで薄くラップする。
//
// 送信側は ctx の trace を traceparent メッセージ属性として注入し、受信側はそれを取り出して
// 子 span を開始する。これで SQS を挟んでも trace_id が切れない。
//
// SDK と同名なので、呼び出し側では SDK を awssqs、このパッケージを libsqs のように別名で import する。
//
// 送信:
//
//	awsCfg, err := awsconfig.Load(ctx, oswrapper.New()) // STAGE=local / ci なら Floci を向く
//	queue := libsqs.New(awsCfg, log)
//	msgID, err := queue.SendJSON(ctx, queueURL, payload) // traceparent を注入し sqs_send span を出す
//
// 受信(Lambda):
//
//	for _, record := range event.Records {
//		rctx := libsqs.RecordContext(ctx, record) // 送信側の trace の子 span + message_id
//	}
//
// 向き先(Floci か AWS か)は aws.Config で決まる。このパッケージは STAGE を見ない。
package sqs

import (
	"context"
	"encoding/json"
	"fmt"

	"snap_kakeibo/backend/internal/library/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// span 名のうち sqs が出すもの。
const (
	// SpanSend は送信 1 回の span 名。
	SpanSend = "sqs_send"
)

// API は Client が使う SDK の操作。*awssqs.Client が満たす。テストでは差し替える。
type API interface {
	SendMessage(ctx context.Context, params *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error)
}

var _ API = (*awssqs.Client)(nil)

// Client は SQS の送信を trace 注入・ログ付きで行う。
type Client struct {
	api API
	log logger.Interface
}

// New は cfg から SDK クライアントを作って Client を生成する。log が nil なら何も出力しない。
func New(cfg aws.Config, log logger.Interface) *Client {
	return NewWithAPI(awssqs.NewFromConfig(cfg), log)
}

// NewWithAPI は SDK クライアント(またはテスト用の差し替え)を受け取って Client を生成する。
func NewWithAPI(api API, log logger.Interface) *Client {
	if log == nil {
		log = logger.NewNop()
	}
	return &Client{api: api, log: log}
}

// SendMessage は SDK の SendMessage と同じ入出力で、ctx の trace を traceparent 属性として注入し、
// 送信を sqs_send span(queue_url / message_id / duration_ms)として記録する。
// 呼び出し側が同名の属性を既に付けていればそちらを優先する。
func (c *Client) SendMessage(ctx context.Context, in *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("sqs: SendMessageInput is nil")
	}
	ctx, span := logger.StartSpan(ctx, c.log, SpanSend, logger.String("queue_url", aws.ToString(in.QueueUrl)))

	// 入力を書き換えないようコピーしてから属性を足す
	sending := *in
	sending.MessageAttributes = Inject(ctx, in.MessageAttributes)

	out, err := c.api.SendMessage(ctx, &sending, optFns...)
	if err != nil {
		span.End(err)
		return out, err
	}
	span.End(nil, logger.String("message_id", aws.ToString(out.MessageId)))
	return out, nil
}

// SendJSON は body を JSON にして queueURL へ送り、message_id を返す。
func (c *Client) SendJSON(ctx context.Context, queueURL string, body any) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("sqs: marshal body: %w", err)
	}
	out, err := c.SendMessage(ctx, &awssqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(b)),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.MessageId), nil
}
