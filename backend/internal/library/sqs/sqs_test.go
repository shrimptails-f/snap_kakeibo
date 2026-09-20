package sqs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/trace"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// fakeAPI は SendMessage の入力を記録し、固定の応答か err を返す。
type fakeAPI struct {
	in  *awssqs.SendMessageInput
	err error
}

func (f *fakeAPI) SendMessage(_ context.Context, in *awssqs.SendMessageInput, _ ...func(*awssqs.Options)) (*awssqs.SendMessageOutput, error) {
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	return &awssqs.SendMessageOutput{MessageId: aws.String("msg-1")}, nil
}

func TestSendJSONInjectsTraceparentAndLogsSpan(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	log, buf := newTestLogger()
	ctx, sender := trace.Start(context.Background())

	id, err := New(api, log).SendJSON(ctx, "http://q/1", map[string]any{"upload_id": "u1"})
	if err != nil || id != "msg-1" {
		t.Fatalf("id=%q err=%v", id, err)
	}

	if got := aws.ToString(api.in.QueueUrl); got != "http://q/1" {
		t.Fatalf("queue_url=%q", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(aws.ToString(api.in.MessageBody)), &body); err != nil || body["upload_id"] != "u1" {
		t.Fatalf("body=%q err=%v", aws.ToString(api.in.MessageBody), err)
	}

	// traceparent は送信 span(sender の子)のものが入る
	attr := api.in.MessageAttributes[trace.TraceparentHeader]
	sent, ok := trace.ParseTraceparent(aws.ToString(attr.StringValue))
	if !ok || aws.ToString(attr.DataType) != "String" || sent.TraceID != sender.TraceID || sent.SpanID == sender.SpanID {
		t.Fatalf("traceparent attr=%+v parsed=%+v ok=%v", attr, sent, ok)
	}

	entries := allEntries(t, buf)
	if len(entries) != 2 {
		t.Fatalf("entries=%d: %s", len(entries), buf.String())
	}
	finished := entries[1]
	assertField(t, finished, "event", logger.EventSpanFinished)
	assertField(t, finished, "span_name", SpanSend)
	assertField(t, finished, "status", logger.StatusOK)
	assertField(t, finished, "queue_url", "http://q/1")
	assertField(t, finished, "message_id", "msg-1")
	assertField(t, finished, "span_id", sent.SpanID)
	assertField(t, finished, "parent_span_id", sender.SpanID)
}

func TestSendMessageDoesNotMutateInputAndKeepsExistingAttributes(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{}
	ctx, _ := trace.Start(context.Background())
	in := &awssqs.SendMessageInput{
		QueueUrl:    aws.String("http://q/1"),
		MessageBody: aws.String("{}"),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"kind": {DataType: aws.String("String"), StringValue: aws.String("retry")},
		},
	}

	if _, err := New(api, nil).SendMessage(ctx, in); err != nil {
		t.Fatal(err)
	}

	if _, ok := in.MessageAttributes[trace.TraceparentHeader]; ok {
		t.Fatal("input must not be mutated")
	}
	if got := aws.ToString(api.in.MessageAttributes["kind"].StringValue); got != "retry" {
		t.Fatalf("existing attribute lost: %v", api.in.MessageAttributes)
	}
	if _, ok := api.in.MessageAttributes[trace.TraceparentHeader]; !ok {
		t.Fatalf("traceparent missing: %v", api.in.MessageAttributes)
	}
}

func TestSendMessageError(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{err: errors.New("boom")}
	log, buf := newTestLogger()

	_, err := New(api, log).SendMessage(context.Background(), &awssqs.SendMessageInput{QueueUrl: aws.String("http://q/1")})
	if err == nil {
		t.Fatal("want error")
	}
	entries := allEntries(t, buf)
	finished := entries[len(entries)-1]
	assertField(t, finished, "level", "ERROR")
	assertField(t, finished, "status", logger.StatusError)
	assertField(t, finished, "error", "boom")

	if _, err := New(api, log).SendMessage(context.Background(), nil); err == nil {
		t.Fatal("nil input must be rejected")
	}
}

func TestInject(t *testing.T) {
	t.Parallel()

	if got := Inject(context.Background(), nil); got != nil {
		t.Fatalf("no trace -> unchanged, got %v", got)
	}

	ctx, tc := trace.Start(context.Background())
	got := Inject(ctx, nil)
	if aws.ToString(got[trace.TraceparentHeader].StringValue) != tc.Traceparent() {
		t.Fatalf("got %v", got)
	}

	existing := map[string]types.MessageAttributeValue{
		trace.TraceparentHeader: {DataType: aws.String("String"), StringValue: aws.String("00-" + strings.Repeat("a", 32) + "-" + strings.Repeat("b", 16) + "-01")},
	}
	if got := Inject(ctx, existing); aws.ToString(got[trace.TraceparentHeader].StringValue) == tc.Traceparent() {
		t.Fatal("existing traceparent must be kept")
	}
}

func TestExtract(t *testing.T) {
	t.Parallel()
	remote := trace.Context{TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16), Sampled: true}
	xray := trace.Context{TraceID: "5759e988bd862e3fe1be46a994272793", SpanID: "53995c3f42cd8ad8", Sampled: true}

	if tc, ok := Extract(remote.Traceparent(), xray.XRayTraceHeader()); !ok || tc.TraceID != remote.TraceID {
		t.Fatalf("traceparent must win: %+v %v", tc, ok)
	}
	if tc, ok := Extract("garbage", xray.XRayTraceHeader()); !ok || tc.TraceID != xray.TraceID {
		t.Fatalf("x-ray fallback: %+v %v", tc, ok)
	}
	if _, ok := Extract("", "garbage"); ok {
		t.Fatal("both invalid -> false")
	}
}

func TestRecordContextAndMessageContext(t *testing.T) {
	t.Parallel()
	remote := trace.Context{TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16), Sampled: true}
	xray := trace.Context{TraceID: "5759e988bd862e3fe1be46a994272793", SpanID: "53995c3f42cd8ad8", Sampled: true}
	base, invocation := trace.Start(context.Background())

	tests := []struct {
		name       string
		record     events.SQSMessage
		wantTrace  string
		wantParent string
	}{
		{
			name: "traceparent attribute wins",
			record: events.SQSMessage{
				MessageId:         "m1",
				MessageAttributes: map[string]events.SQSMessageAttribute{"traceparent": {DataType: "String", StringValue: aws.String(remote.Traceparent())}},
				Attributes:        map[string]string{"AWSTraceHeader": xray.XRayTraceHeader()},
			},
			wantTrace:  remote.TraceID,
			wantParent: remote.SpanID,
		},
		{
			name:       "x-ray header",
			record:     events.SQSMessage{MessageId: "m2", Attributes: map[string]string{"AWSTraceHeader": xray.XRayTraceHeader()}},
			wantTrace:  xray.TraceID,
			wantParent: xray.SpanID,
		},
		{
			name:       "falls back to invocation trace",
			record:     events.SQSMessage{MessageId: "m3"},
			wantTrace:  invocation.TraceID,
			wantParent: invocation.SpanID,
		},
		{
			name:       "invalid header falls back",
			record:     events.SQSMessage{MessageId: "m4", Attributes: map[string]string{"AWSTraceHeader": "garbage"}},
			wantTrace:  invocation.TraceID,
			wantParent: invocation.SpanID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertCtx := func(ctx context.Context) {
				t.Helper()
				tc, ok := trace.FromContext(ctx)
				if !ok || tc.TraceID != tt.wantTrace || tc.ParentSpanID != tt.wantParent {
					t.Fatalf("tc=%+v ok=%v", tc, ok)
				}
				fields := logger.FieldsFromContext(ctx)
				if len(fields) != 1 || fields[0].Key != "message_id" || fields[0].Value.String() != tt.record.MessageId {
					t.Fatalf("fields=%v", fields)
				}
			}

			assertCtx(RecordContext(base, tt.record))

			// SDK の Message でも同じ結果になる
			msg := types.Message{MessageId: aws.String(tt.record.MessageId), Attributes: tt.record.Attributes}
			if attr, ok := tt.record.MessageAttributes["traceparent"]; ok {
				msg.MessageAttributes = map[string]types.MessageAttributeValue{"traceparent": {DataType: aws.String("String"), StringValue: attr.StringValue}}
			}
			assertCtx(MessageContext(base, msg))
		})
	}
}

func newTestLogger() (*logger.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return logger.New(logger.Options{Level: "debug", Service: "test", Environment: "test", Writer: &buf}), &buf
}

func allEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func assertField(t *testing.T, entry map[string]any, key string, want any) {
	t.Helper()
	got, ok := entry[key]
	if !ok {
		t.Fatalf("%s is missing: %v", key, entry)
	}
	if got != want {
		t.Fatalf("%s=%v (%T) want %v (%T)", key, got, got, want, want)
	}
}
