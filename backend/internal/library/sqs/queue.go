package sqs

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Queue は特定のキューに束縛した Client。
// 業務コードはキュー URL を設定から 1 回だけ受け取り、以降は本文だけで送る。
// 統合テストではランダムな名前で作ったキューを Queue にして渡せるので、業務コードがキュー URL を知る必要がない。
//
//	analyzeQueue := libsqs.New(cfg, log).Queue(cfg.AnalyzeQueueURL)
//	msgID, err := analyzeQueue.SendJSON(ctx, payload)
type Queue struct {
	client *Client
	url    string
}

// Queue は url に束縛した Queue を返す。
func (c *Client) Queue(url string) *Queue {
	return &Queue{client: c, url: url}
}

// URL はキュー URL を返す。
func (q *Queue) URL() string { return q.url }

// SendMessage は Client.SendMessage をこのキューに対して行う。in.QueueUrl が空ならこのキューの URL を入れ、
// 別のキューが指定されていればエラーにする。
func (q *Queue) SendMessage(ctx context.Context, in *awssqs.SendMessageInput, optFns ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("sqs: SendMessageInput is nil")
	}
	if target := aws.ToString(in.QueueUrl); target != "" && target != q.url {
		return nil, fmt.Errorf("sqs: input targets %s but the queue is bound to %s", target, q.url)
	}
	sending := *in
	sending.QueueUrl = aws.String(q.url)
	return q.client.SendMessage(ctx, &sending, optFns...)
}

// SendJSON は Client.SendJSON をこのキューに対して行う。
func (q *Queue) SendJSON(ctx context.Context, body any) (string, error) {
	return q.client.SendJSON(ctx, q.url, body)
}
