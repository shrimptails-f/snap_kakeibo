package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/trace"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// TestFlociRoundTrip は実際の SQS API(ローカルの Floci)に対して送信 → 受信 → trace 取り出しを通す。
// AWS_ENDPOINT_URL が未設定(CI など)ならスキップする。devcontainer では compose が Floci を向けている。
func TestFlociRoundTrip(t *testing.T) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		t.Skip("AWS_ENDPOINT_URL is not set; skipping Floci integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-northeast-1"
	}
	// Floci は認証を検証しないので固定のダミー資格情報を使う。~/.aws の実プロファイルには依存しない
	api := awssqs.New(awssqs.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	})

	queueName := fmt.Sprintf("sqs-roundtrip-%d", time.Now().UnixNano())
	created, err := api.CreateQueue(ctx, &awssqs.CreateQueueInput{QueueName: aws.String(queueName)})
	if err != nil {
		t.Fatalf("CreateQueue against %s failed: %v", endpoint, err)
	}
	queueURL := aws.ToString(created.QueueUrl)
	t.Cleanup(func() {
		_, _ = api.DeleteQueue(context.Background(), &awssqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
	})

	// 送信: ctx に trace を積んで SendJSON
	log, buf := newTestLogger()
	sendCtx, sender := trace.Start(logger.ContextWith(ctx, logger.UploadID("upload-1")))
	msgID, err := New(api, log).SendJSON(sendCtx, queueURL, map[string]any{"upload_id": "upload-1", "trigger": "RETRY"})
	if err != nil {
		t.Fatalf("SendJSON: %v", err)
	}
	if msgID == "" {
		t.Fatal("message id is empty")
	}

	// 受信: 生の SDK で取り出し、body と traceparent 属性を確認
	received, err := api.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:              aws.String(queueURL),
		MaxNumberOfMessages:   1,
		WaitTimeSeconds:       5,
		MessageAttributeNames: []string{"All"},
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{
			types.MessageSystemAttributeNameAll,
		},
	})
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if len(received.Messages) != 1 {
		t.Fatalf("messages=%d want 1", len(received.Messages))
	}
	msg := received.Messages[0]
	if aws.ToString(msg.MessageId) != msgID {
		t.Fatalf("message id mismatch: sent %s received %s", msgID, aws.ToString(msg.MessageId))
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(aws.ToString(msg.Body)), &body); err != nil || body["upload_id"] != "upload-1" {
		t.Fatalf("body=%q err=%v", aws.ToString(msg.Body), err)
	}
	sent, ok := trace.ParseTraceparent(aws.ToString(msg.MessageAttributes[trace.TraceparentHeader].StringValue))
	if !ok || sent.TraceID != sender.TraceID {
		t.Fatalf("traceparent attr=%+v parsed=%+v ok=%v", msg.MessageAttributes, sent, ok)
	}

	// 受信側の ctx: 送信 span の子になり、message_id が付く
	recvCtx := MessageContext(context.Background(), msg)
	tc, ok := trace.FromContext(recvCtx)
	if !ok || tc.TraceID != sender.TraceID || tc.ParentSpanID != sent.SpanID {
		t.Fatalf("receiver trace=%+v ok=%v (sender=%+v)", tc, ok, sender)
	}
	fields := logger.FieldsFromContext(recvCtx)
	if len(fields) != 1 || fields[0].Key != "message_id" || fields[0].Value.String() != msgID {
		t.Fatalf("fields=%v", fields)
	}

	// 送信ログ: sqs_send span に queue_url / message_id / upload_id が載る
	entries := allEntries(t, buf)
	finished := entries[len(entries)-1]
	assertField(t, finished, "event", logger.EventSpanFinished)
	assertField(t, finished, "span_name", SpanSend)
	assertField(t, finished, "status", logger.StatusOK)
	assertField(t, finished, "queue_url", queueURL)
	assertField(t, finished, "message_id", msgID)
	assertField(t, finished, "upload_id", "upload-1")
	assertField(t, finished, "trace_id", sender.TraceID)
	t.Logf("sqs_send: %s", buf.String())
}
