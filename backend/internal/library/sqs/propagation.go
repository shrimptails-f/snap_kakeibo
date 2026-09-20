package sqs

import (
	"context"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/trace"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// xrayAttribute は X-Ray が有効なとき SQS がシステム属性として運ぶトレースヘッダのキー。
const xrayAttribute = "AWSTraceHeader"

// Inject は ctx の trace を traceparent メッセージ属性として attrs に加えた新しい map を返す。
// ctx に trace がなければ attrs をそのまま返す。既に traceparent があれば上書きしない。
func Inject(ctx context.Context, attrs map[string]types.MessageAttributeValue) map[string]types.MessageAttributeValue {
	tc, ok := trace.FromContext(ctx)
	if !ok {
		return attrs
	}
	if _, exists := attrs[trace.TraceparentHeader]; exists {
		return attrs
	}
	out := make(map[string]types.MessageAttributeValue, len(attrs)+1)
	for k, v := range attrs {
		out[k] = v
	}
	out[trace.TraceparentHeader] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(tc.Traceparent()),
	}
	return out
}

// Extract は受信メッセージから親 trace を決める。
// 送信側が付けた traceparent → X-Ray の AWSTraceHeader の順に見て、どちらも無効なら ok=false。
func Extract(traceparent, awsTraceHeader string) (trace.Context, bool) {
	if tc, ok := trace.ParseTraceparent(traceparent); ok {
		return tc, true
	}
	if tc, ok := trace.ParseXRayTraceHeader(awsTraceHeader); ok {
		return tc, true
	}
	return trace.Context{}, false
}

// RecordContext は Lambda が受け取った SQS レコード 1 件の処理用 ctx を返す。
// レコードが運んできた trace(なければ ctx の trace)の子 span を開始し、message_id を積む。
// 同じレコードの再配信(リトライ)は message_id が同じなのでログで追える。
func RecordContext(ctx context.Context, record events.SQSMessage) context.Context {
	var traceparent string
	if attr, ok := record.MessageAttributes[trace.TraceparentHeader]; ok {
		traceparent = aws.ToString(attr.StringValue)
	}
	return messageContext(ctx, traceparent, record.Attributes[xrayAttribute], record.MessageId)
}

// MessageContext は SDK の ReceiveMessage で受け取ったメッセージ 1 件の処理用 ctx を返す。動きは RecordContext と同じ。
func MessageContext(ctx context.Context, msg types.Message) context.Context {
	var traceparent string
	if attr, ok := msg.MessageAttributes[trace.TraceparentHeader]; ok {
		traceparent = aws.ToString(attr.StringValue)
	}
	return messageContext(ctx, traceparent, msg.Attributes[xrayAttribute], aws.ToString(msg.MessageId))
}

func messageContext(ctx context.Context, traceparent, awsTraceHeader, messageID string) context.Context {
	parent, ok := Extract(traceparent, awsTraceHeader)
	if !ok {
		parent, _ = trace.FromContext(ctx)
	}
	ctx, _ = trace.StartFrom(ctx, parent)
	if messageID != "" {
		ctx = logger.ContextWith(ctx, logger.String("message_id", messageID))
	}
	return ctx
}
