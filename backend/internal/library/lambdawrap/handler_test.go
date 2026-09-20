package lambdawrap

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
	"github.com/aws/aws-lambda-go/lambdacontext"
)

func TestHandleSuccess(t *testing.T) {
	coldStart.Store(true)
	log, buf := newTestLogger()
	ctx := lambdacontext.NewContext(context.Background(), &lambdacontext.LambdaContext{AwsRequestID: "req-1"})

	var seen context.Context
	h := Handle(log, func(ctx context.Context, in string) (string, error) {
		seen = ctx
		log.Info(ctx, "inside")
		return in + "!", nil
	})

	out, err := h(ctx, "hi")
	if err != nil || out != "hi!" {
		t.Fatalf("out=%q err=%v", out, err)
	}

	if _, ok := trace.FromContext(seen); !ok {
		t.Fatal("handler ctx must carry a trace")
	}

	entries := allEntries(t, buf)
	if len(entries) != 3 {
		t.Fatalf("entries=%d: %s", len(entries), buf.String())
	}
	assertField(t, entries[0], "event", "invocation_started")
	assertField(t, entries[0], "cold_start", true)
	assertField(t, entries[1], "message", "inside")
	assertField(t, entries[2], "event", "invocation_finished")
	assertField(t, entries[2], "level", "INFO")

	for _, e := range entries {
		assertField(t, e, "request_id", "req-1")
		assertField(t, e, "trace_id", entries[0]["trace_id"])
		if _, ok := e["duration_ms"]; ok != (e["event"] == "invocation_finished") {
			t.Fatalf("duration_ms presence mismatch: %v", e)
		}
	}

	// 2 回目以降は cold_start=false
	buf.Reset()
	if _, err := h(ctx, "again"); err != nil {
		t.Fatal(err)
	}
	assertField(t, allEntries(t, buf)[0], "cold_start", false)
}

func TestHandleError(t *testing.T) {
	log, buf := newTestLogger()
	want := errors.New("boom")

	h := Handle(log, func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, want
	})

	if _, err := h(context.Background(), struct{}{}); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}

	entries := allEntries(t, buf)
	last := entries[len(entries)-1]
	assertField(t, last, "level", "ERROR")
	assertField(t, last, "event", "invocation_finished")
	assertField(t, last, "error", "boom")
}

func TestHandleRecoversPanic(t *testing.T) {
	log, buf := newTestLogger()

	h := Handle(log, func(ctx context.Context, in struct{}) (struct{}, error) {
		panic("kaboom")
	})

	_, err := h(context.Background(), struct{}{})
	if err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("err=%v", err)
	}

	entries := allEntries(t, buf)
	var recovered map[string]any
	for _, e := range entries {
		if e["event"] == "panic_recovered" {
			recovered = e
		}
	}
	if recovered == nil {
		t.Fatalf("panic_recovered log is missing: %s", buf.String())
	}
	assertField(t, recovered, "recovered", "kaboom")
	if st, _ := recovered["stack_trace"].(string); !strings.Contains(st, "handler_test.go") {
		t.Fatalf("stack_trace=%q", st)
	}
	assertField(t, entries[len(entries)-1], "event", "invocation_finished")
	assertField(t, entries[len(entries)-1], "level", "ERROR")
}

func TestHandleEvent(t *testing.T) {
	log, buf := newTestLogger()
	want := errors.New("boom")

	h := HandleEvent(log, func(ctx context.Context, in events.SQSEvent) error {
		if _, ok := trace.FromContext(ctx); !ok {
			t.Fatal("handler ctx must carry a trace")
		}
		return want
	})

	if err := h(context.Background(), events.SQSEvent{}); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	entries := allEntries(t, buf)
	assertField(t, entries[len(entries)-1], "event", "invocation_finished")
	assertField(t, entries[len(entries)-1], "error", "boom")
}

func TestHandleWithNilLoggerDoesNotPanic(t *testing.T) {
	h := Handle(nil, func(ctx context.Context, in int) (int, error) { return in * 2, nil })
	if out, err := h(context.Background(), 2); err != nil || out != 4 {
		t.Fatalf("out=%d err=%v", out, err)
	}
}

func TestInvocationContextUsesXRayHeader(t *testing.T) {
	t.Setenv(xrayTraceIDEnv, "Root=1-5759e988-bd862e3fe1be46a994272793;Parent=53995c3f42cd8ad8;Sampled=1")

	ctx := InvocationContext(context.Background())

	tc, ok := trace.FromContext(ctx)
	if !ok || tc.TraceID != "5759e988bd862e3fe1be46a994272793" || tc.ParentSpanID != "53995c3f42cd8ad8" || tc.SpanID == "53995c3f42cd8ad8" {
		t.Fatalf("tc=%+v ok=%v", tc, ok)
	}
}

func TestInvocationContextWithoutXRayStartsNewTrace(t *testing.T) {
	t.Setenv(xrayTraceIDEnv, "")

	ctx := InvocationContext(context.Background())

	if tc, ok := trace.FromContext(ctx); !ok || tc.ParentSpanID != "" {
		t.Fatalf("tc=%+v ok=%v", tc, ok)
	}
	if fields := logger.FieldsFromContext(ctx); len(fields) != 0 {
		t.Fatalf("no lambdacontext -> no request_id, got %v", fields)
	}
}

func TestSQSRecordContext(t *testing.T) {
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
				MessageAttributes: map[string]events.SQSMessageAttribute{"traceparent": {DataType: "String", StringValue: strPtr(remote.Traceparent())}},
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
			ctx := SQSRecordContext(base, tt.record)

			tc, ok := trace.FromContext(ctx)
			if !ok || tc.TraceID != tt.wantTrace || tc.ParentSpanID != tt.wantParent {
				t.Fatalf("tc=%+v ok=%v", tc, ok)
			}
			fields := logger.FieldsFromContext(ctx)
			if len(fields) != 1 || fields[0].Key != "message_id" || fields[0].Value.String() != tt.record.MessageId {
				t.Fatalf("fields=%v", fields)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

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
