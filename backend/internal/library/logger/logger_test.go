package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/trace"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unexpected", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := parseLevel(tt.input); got != tt.want {
				t.Fatalf("parseLevel(%q)=%v want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewWritesJSONWithFixedFields(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("info")

	log.Info(context.Background(), "hello", String("name", "alice"), Int("count", 3))

	entry := singleEntry(t, buf)
	assertField(t, entry, "level", "INFO")
	assertField(t, entry, "message", "hello")
	assertField(t, entry, "service", "test-service")
	assertField(t, entry, "environment", "test")
	assertField(t, entry, "name", "alice")
	assertField(t, entry, "count", float64(3))
	if _, ok := entry["time"]; !ok {
		t.Fatal("time is missing")
	}
	caller, _ := entry["caller"].(string)
	if !strings.HasPrefix(caller, "logger/logger_test.go:") {
		t.Fatalf("caller=%q want logger/logger_test.go:<line>", caller)
	}
}

func TestNewDefaultsBlankFixedFieldsToUnknown(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := New(Options{Writer: &buf})

	log.Info(context.Background(), "hello")

	entry := singleEntry(t, &buf)
	assertField(t, entry, "service", "unknown")
	assertField(t, entry, "environment", "unknown")
}

func TestLevelFiltering(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("warn")
	ctx := context.Background()

	log.Debug(ctx, "debug")
	log.Info(ctx, "info")
	log.Warn(ctx, "warn")
	log.Error(ctx, "error")

	entries := allEntries(t, buf)
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2", len(entries))
	}
	assertField(t, entries[0], "level", "WARN")
	assertField(t, entries[1], "level", "ERROR")
}

func TestContextFieldsPropagate(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	ctx := ContextWith(context.Background(), RequestID("req-1"))
	ctx = ContextWith(ctx, UserID("user-1"), UploadID("upload-1"))

	log.Info(ctx, "hello")

	entry := singleEntry(t, buf)
	assertField(t, entry, "request_id", "req-1")
	assertField(t, entry, "user_id", "user-1")
	assertField(t, entry, "upload_id", "upload-1")
}

func TestContextWithReplacesSameKey(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	ctx := ContextWith(context.Background(), UploadID("first"))
	ctx = ContextWith(ctx, UploadID("second"))

	log.Info(ctx, "hello")

	raw := buf.String()
	if strings.Count(raw, `"upload_id"`) != 1 {
		t.Fatalf("upload_id should appear once: %s", raw)
	}
	assertField(t, singleEntry(t, buf), "upload_id", "second")
}

func TestContextWithDoesNotAffectParentContext(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	parent := ContextWith(context.Background(), RequestID("req-1"))
	child := ContextWith(parent, UploadID("upload-1"))
	_ = child

	log.Info(parent, "hello")

	entry := singleEntry(t, buf)
	assertField(t, entry, "request_id", "req-1")
	if _, ok := entry["upload_id"]; ok {
		t.Fatal("upload_id must not leak into parent context")
	}
}

func TestFieldsFromContext(t *testing.T) {
	t.Parallel()

	if got := FieldsFromContext(context.Background()); got != nil {
		t.Fatalf("got %v want nil", got)
	}
	if got := FieldsFromContext(nilContext); got != nil {
		t.Fatalf("got %v want nil", got)
	}

	ctx := ContextWith(context.Background(), RequestID("req-1"))
	got := FieldsFromContext(ctx)
	if len(got) != 1 || got[0].Key != "request_id" || got[0].Value.String() != "req-1" {
		t.Fatalf("got %v", got)
	}

	// 返り値を書き換えても ctx 側は変わらない
	got[0] = RequestID("changed")
	if FieldsFromContext(ctx)[0].Value.String() != "req-1" {
		t.Fatal("FieldsFromContext must return a copy")
	}
}

func TestTraceFieldsPropagateFromContext(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	ctx, root := trace.Start(context.Background())
	log.Info(ctx, "root")
	ctx, child := trace.Start(ctx)
	log.Info(ctx, "child")

	entries := allEntries(t, buf)
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2", len(entries))
	}
	assertField(t, entries[0], "trace_id", root.TraceID)
	assertField(t, entries[0], "span_id", root.SpanID)
	if _, ok := entries[0]["parent_span_id"]; ok {
		t.Fatal("root must not have parent_span_id")
	}
	assertField(t, entries[1], "trace_id", child.TraceID)
	assertField(t, entries[1], "span_id", child.SpanID)
	assertField(t, entries[1], "parent_span_id", root.SpanID)
}

func TestNoTraceFieldsWithoutTrace(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	log.Info(context.Background(), "hello")

	if strings.Contains(buf.String(), "trace_id") {
		t.Fatalf("trace_id must be absent: %s", buf.String())
	}
}

func TestWithAndContextCompose(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")
	ctx := ContextWith(context.Background(), RequestID("req-1"))

	log.With(Component("usecase")).Info(ctx, "hello", Int("count", 1))

	entry := singleEntry(t, buf)
	assertField(t, entry, "component", "usecase")
	assertField(t, entry, "request_id", "req-1")
	assertField(t, entry, "count", float64(1))
}

func TestWithoutFieldsReturnsSameLogger(t *testing.T) {
	t.Parallel()
	log, _ := newTestLogger("debug")
	if log.With() != Interface(log) {
		t.Fatal("With() without fields should return the same logger")
	}
}

func TestSensitiveKeysAreRedacted(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")
	ctx := ContextWith(context.Background(), String("token", "ctx-secret"))

	log.Info(ctx, "hello",
		String("email", "alice@example.com"),
		String("Access-Token", "secret"),
		Any("password", "secret"),
		String("token_type", "Bearer"),
	)

	entry := singleEntry(t, buf)
	assertField(t, entry, "email", redacted)
	assertField(t, entry, "Access-Token", redacted)
	assertField(t, entry, "password", redacted)
	assertField(t, entry, "token", redacted)
	assertField(t, entry, "token_type", "Bearer")
	if strings.Contains(buf.String(), "secret") {
		t.Fatalf("secret leaked: %s", buf.String())
	}
}

func TestErr(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	log.Error(context.Background(), "failed", Err(errors.New("boom")), Err(nil))

	entry := singleEntry(t, buf)
	assertField(t, entry, "error", "boom")
	assertField(t, entry, "error_type", "errors.errorString")
	if strings.Count(buf.String(), `"error"`) != 1 {
		t.Fatalf("Err(nil) must not emit a field: %s", buf.String())
	}
}

type typedError struct{ msg string }

func (e *typedError) Error() string { return e.msg }

func TestErrorType(t *testing.T) {
	t.Parallel()
	typed := &typedError{msg: "typed"}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"plain", errors.New("x"), "errors.errorString"},
		{"typed", typed, "logger.typedError"},
		{"wrapped typed", fmt.Errorf("outer: %w", typed), "logger.typedError"},
		{"typed wrapping sentinel", fmt.Errorf("a: %w", fmt.Errorf("b: %w", typed)), "logger.typedError"},
		{"wrapped sentinel", fmt.Errorf("outer: %w", errors.New("x")), "errors.errorString"},
		{"joined", errors.Join(typed, errors.New("x")), "logger.typedError"},
		{"stdlib typed", fmt.Errorf("outer: %w", &json.SyntaxError{}), "json.SyntaxError"},
		{"context", context.DeadlineExceeded, "context.deadlineExceededError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := errorType(tt.err); got != tt.want {
				t.Fatalf("errorType()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestDuplicateKeysAreCollapsed(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")
	ctx := ContextWith(context.Background(), UploadID("from-ctx"), Component("from-ctx"), String("service", "from-ctx"))
	ctx, tc := trace.Start(ctx)

	log.With(Component("from-with"), Environment("from-with")).Info(ctx, "dup",
		UploadID("from-call"),
		TraceID("manual"),
		Err(errors.New("boom")),
		String("error", "from-call"),
	)

	raw := buf.String()
	for _, key := range []string{"upload_id", "component", "service", "environment", "trace_id", "error", "error_type"} {
		if n := strings.Count(raw, `"`+key+`"`); n != 1 {
			t.Fatalf("%s appears %d times: %s", key, n, raw)
		}
	}
	entry := singleEntry(t, buf)
	assertField(t, entry, "upload_id", "from-call")   // 呼び出し時 > ctx
	assertField(t, entry, "component", "from-ctx")    // ctx > With
	assertField(t, entry, "service", "from-ctx")      // ctx > New の固定値
	assertField(t, entry, "environment", "from-with") // With > New の固定値
	assertField(t, entry, "trace_id", tc.TraceID)     // trace は手動指定より ctx
	assertField(t, entry, "error", "from-call")       // 後に渡したものが勝つ
	assertField(t, entry, "error_type", "errors.errorString")
}

func TestSpan(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")
	ctx := ContextWith(context.Background(), RequestID("req-1"))
	ctx, parent := trace.Start(ctx)

	sctx, span := StartSpan(ctx, log, "analysis", String("s3_key", "k"))
	log.Info(sctx, "inside")
	span.AddFields(Int("detail_count", 2), String("status", "ignored"))
	span.End(nil, Int("detail_count", 3))
	span.End(errors.New("twice")) // 2 回目は無視

	entries := allEntries(t, buf)
	if len(entries) != 3 {
		t.Fatalf("entries=%d: %s", len(entries), buf.String())
	}
	started, inside, finished := entries[0], entries[1], entries[2]

	assertField(t, started, "event", EventSpanStarted)
	assertField(t, started, "message", "analysis started")
	for _, e := range entries {
		assertField(t, e, "span_name", "analysis")
		assertField(t, e, "s3_key", "k")
		assertField(t, e, "request_id", "req-1")
		assertField(t, e, "trace_id", parent.TraceID)
		assertField(t, e, "parent_span_id", parent.SpanID)
		if e["span_id"] == parent.SpanID {
			t.Fatalf("span must get a new span_id: %v", e)
		}
		caller, _ := e["caller"].(string)
		if !strings.HasPrefix(caller, "logger/logger_test.go:") {
			t.Fatalf("caller=%q want logger/logger_test.go:<line>", caller)
		}
	}
	assertField(t, inside, "message", "inside")
	assertField(t, finished, "event", EventSpanFinished)
	assertField(t, finished, "level", "INFO")
	assertField(t, finished, "status", StatusOK)
	assertField(t, finished, "detail_count", float64(3))
	if _, ok := finished["duration_ms"]; !ok {
		t.Fatal("duration_ms is missing")
	}
	if _, ok := started["duration_ms"]; ok {
		t.Fatal("span_started must not have duration_ms")
	}
}

func TestSpanEndWithError(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	_, span := StartSpan(context.Background(), log, "job")
	span.End(errors.New("boom"))

	entries := allEntries(t, buf)
	finished := entries[len(entries)-1]
	assertField(t, finished, "level", "ERROR")
	assertField(t, finished, "message", "job failed")
	assertField(t, finished, "status", StatusError)
	assertField(t, finished, "error", "boom")
	assertField(t, finished, "error_type", "errors.errorString")
}

func TestSpanWithNilLoggerAndNilSpan(t *testing.T) {
	t.Parallel()
	ctx, span := StartSpan(context.Background(), nil, "job")
	if _, ok := trace.FromContext(ctx); !ok {
		t.Fatal("span ctx must carry a trace")
	}
	span.AddFields(Int("n", 1))
	span.End(nil)

	var nilSpan *Span
	nilSpan.AddFields(Int("n", 1))
	nilSpan.End(nil)
}

func TestSchemaHelpers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		field Field
		key   string
		value string
	}{
		{Service(""), "service", "unknown"},
		{Service(" backend "), "service", "backend"},
		{Environment("dev"), "environment", "dev"},
		{Component("repo"), "component", "repo"},
		{RequestID("r"), "request_id", "r"},
		{UserID("u"), "user_id", "u"},
		{UploadID("up"), "upload_id", "up"},
		{BillingID("b"), "billing_id", "b"},
		{HTTPStatusCode(500), "http_status_code", "500"},
		{Recovered("p"), "recovered", "p"},
		{Event("e"), "event", "e"},
		{DurationMS(1500 * time.Millisecond), "duration_ms", "1500"},
		{TraceID("t"), "trace_id", "t"},
		{SpanName("n"), "span_name", "n"},
		{Status("ok"), "status", "ok"},
		{SpanID("s"), "span_id", "s"},
		{ParentSpanID("ps"), "parent_span_id", "ps"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			if tt.field.Key != tt.key || tt.field.Value.String() != tt.value {
				t.Fatalf("got %s=%s want %s=%s", tt.field.Key, tt.field.Value, tt.key, tt.value)
			}
		})
	}

	if st := StackTrace(); st.Key != "stack_trace" || !strings.Contains(st.Value.String(), "runtime/debug.Stack") {
		t.Fatalf("StackTrace()=%v", st)
	}
}

func TestNilContextDoesNotPanic(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger("debug")

	log.Info(nilContext, "hello")

	assertField(t, singleEntry(t, buf), "message", "hello")
}

func TestNewNopDoesNotPanic(t *testing.T) {
	t.Parallel()
	log := NewNop()
	ctx := ContextWith(context.Background(), RequestID("req-1"))

	log.Debug(ctx, "debug")
	log.Info(ctx, "info")
	log.Warn(ctx, "warn")
	log.Error(ctx, "error", Err(errors.New("boom")))
	log.With(Component("c")).Info(ctx, "child")
}

// nilContext は nil ctx を渡されても panic しないことを確認するためのもの。
// リテラルの nil を渡すと staticcheck(SA1012)に弾かれるので変数にしている。
var nilContext context.Context

func newTestLogger(level string) (*Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return New(Options{Level: level, Service: "test-service", Environment: "test", Writer: &buf}), &buf
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

func singleEntry(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	entries := allEntries(t, buf)
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1: %s", len(entries), buf.String())
	}
	return entries[0]
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
