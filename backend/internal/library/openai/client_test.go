package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/retry"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

const completedBody = `{
  "id": "resp_1", "status": "completed",
  "output": [{"type": "message", "content": [{"type": "output_text", "text": "{\"store_name\":\"A\"}"}]}],
  "usage": {"input_tokens": 10, "output_tokens": 5, "output_tokens_details": {"reasoning_tokens": 2}}
}`

// fakeServer は応答を順に返す httptest サーバー。受け取ったリクエストを記録する。
type fakeServer struct {
	*httptest.Server
	calls   atomic.Int32
	handler func(n int, w http.ResponseWriter)

	mu       sync.Mutex // タイムアウト後も前のハンドラが動いているので、記録は排他する
	requests []map[string]any
	headers  []http.Header
}

func newFakeServer(t *testing.T, handler func(n int, w http.ResponseWriter)) *fakeServer {
	t.Helper()
	fs := &fakeServer{handler: handler}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(fs.calls.Add(1))
		if r.Method != http.MethodPost || r.URL.Path != "/responses" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		fs.mu.Lock()
		fs.requests = append(fs.requests, parsed)
		fs.headers = append(fs.headers, r.Header.Clone())
		fs.mu.Unlock()
		w.Header().Set("x-request-id", "req_"+strings.Repeat("x", n))
		fs.handler(n, w)
	}))
	t.Cleanup(fs.Close)
	return fs
}

func respond(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func newClient(t *testing.T, srv *fakeServer, policy retry.Policy) (*Client, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	log := logger.New(logger.Options{Level: "debug", Service: "test", Environment: "test", Writer: &buf})
	c, err := New(Options{
		APIKey:  "sk-test",
		BaseURL: srv.URL + "/",
		Retry:   policy,
		Logger:  log,
		Clock:   timewrapper.NewFixed(time.Now()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, &buf
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Parallel()
	if _, err := New(Options{}); err == nil {
		t.Fatal("empty APIKey must be rejected")
	}
	c, err := New(Options{APIKey: "k"})
	if err != nil || c.baseURL != DefaultBaseURL || c.policy.MaxAttempts() != DefaultRetry.MaxAttempts() || c.policy.ShouldRetry == nil {
		t.Fatalf("defaults: c=%+v err=%v", c, err)
	}
}

func TestResponsesSuccess(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(_ int, w http.ResponseWriter) { respond(w, 200, completedBody) })
	c, buf := newClient(t, srv, retry.Policy{})

	ctx := logger.ContextWith(context.Background(), logger.UploadID("u1"))
	resp, err := c.Responses(ctx, Request{
		Model:           "gpt-5-mini",
		Instructions:    "read the receipt",
		Input:           []Input{UserImageJPEG([]byte{0xFF, 0xD8}, "high")},
		Schema:          &JSONSchema{Name: "receipt", Strict: true, Schema: map[string]any{"type": "object"}},
		ReasoningEffort: "low",
		MaxOutputTokens: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "resp_1" || resp.Status != "completed" || resp.OutputText != `{"store_name":"A"}` || resp.Refusal != "" {
		t.Fatalf("resp=%+v", resp)
	}
	if resp.Usage != (Usage{InputTokens: 10, OutputTokens: 5, ReasoningTokens: 2}) || resp.RequestID != "req_x" {
		t.Fatalf("usage=%+v request_id=%q", resp.Usage, resp.RequestID)
	}
	if !bytes.Equal(resp.Raw, []byte(completedBody)) {
		t.Fatal("Raw must be the response body as is")
	}

	// リクエスト本文
	if srv.headers[0].Get("Authorization") != "Bearer sk-test" || srv.headers[0].Get("Content-Type") != "application/json" {
		t.Fatalf("headers=%v", srv.headers[0])
	}
	req := srv.requests[0]
	if req["model"] != "gpt-5-mini" || req["instructions"] != "read the receipt" || req["store"] != false || req["max_output_tokens"] != float64(4096) {
		t.Fatalf("request=%v", req)
	}
	if req["reasoning"].(map[string]any)["effort"] != "low" {
		t.Fatalf("reasoning=%v", req["reasoning"])
	}
	format := req["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "receipt" || format["strict"] != true || format["schema"] == nil {
		t.Fatalf("format=%v", format)
	}
	content := req["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if content["type"] != "input_image" || content["detail"] != "high" || !strings.HasPrefix(content["image_url"].(string), "data:image/jpeg;base64,") {
		t.Fatalf("content=%v", content)
	}

	// ログ: span_finished にトークン数と response_id、生本文は出ない
	entries := allEntries(t, buf)
	finished := entries[len(entries)-1]
	assertField(t, finished, "event", logger.EventSpanFinished)
	assertField(t, finished, "span_name", SpanRequest)
	assertField(t, finished, "status", logger.StatusOK)
	assertField(t, finished, "model", "gpt-5-mini")
	assertField(t, finished, "reasoning_effort", "low")
	assertField(t, finished, "http_status_code", float64(200))
	assertField(t, finished, "response_id", "resp_1")
	assertField(t, finished, "openai_request_id", "req_x")
	assertField(t, finished, "input_tokens", float64(10))
	assertField(t, finished, "output_tokens", float64(5))
	assertField(t, finished, "reasoning_tokens", float64(2))
	assertField(t, finished, "upload_id", "u1")
	if strings.Contains(buf.String(), "store_name") || strings.Contains(buf.String(), "sk-test") || strings.Contains(buf.String(), "base64") {
		t.Fatalf("response body / api key / image must not be logged: %s", buf.String())
	}
}

func TestResponsesOptionalFieldsAreOmitted(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(_ int, w http.ResponseWriter) { respond(w, 200, completedBody) })
	c, _ := newClient(t, srv, retry.Policy{})

	if _, err := c.Responses(context.Background(), Request{Model: "m", Input: []Input{UserText("hi")}}); err != nil {
		t.Fatal(err)
	}
	req := srv.requests[0]
	for _, key := range []string{"instructions", "max_output_tokens", "reasoning", "text"} {
		if _, ok := req[key]; ok {
			t.Fatalf("%s must be omitted: %v", key, req)
		}
	}
	content := req["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if content["type"] != "input_text" || content["text"] != "hi" {
		t.Fatalf("content=%v", content)
	}
}

func TestResponsesRetriesTemporaryErrors(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(n int, w http.ResponseWriter) {
		switch n {
		case 1:
			respond(w, 429, `{"error":{"message":"slow down","type":"rate_limit","code":"rate_limit_exceeded"}}`)
		case 2:
			respond(w, 503, `oops`)
		case 3:
			respond(w, 200, `{"id":"resp_f","status":"failed","error":{"message":"boom","type":"server_error","code":"x"}}`)
		default:
			respond(w, 200, completedBody)
		}
	})
	c, buf := newClient(t, srv, retry.Policy{Backoff: retry.Exponential(time.Second, time.Second, 3)})

	resp, err := c.Responses(context.Background(), Request{Model: "m", Input: []Input{UserText("hi")}})
	if err != nil || resp.ID != "resp_1" || srv.calls.Load() != 4 {
		t.Fatalf("err=%v calls=%d", err, srv.calls.Load())
	}

	entries := allEntries(t, buf)
	var attempts []map[string]any
	for _, e := range entries {
		if e["event"] == retry.EventRetryAttempt {
			attempts = append(attempts, e)
		}
	}
	if len(attempts) != 3 {
		t.Fatalf("retry_attempt=%d: %s", len(attempts), buf.String())
	}
	assertField(t, attempts[0], "error_type", "openai.APIError")
	assertField(t, attempts[1], "error_type", "openai.APIError")
	assertField(t, attempts[2], "error_type", "openai.ResponseFailedError")
	if strings.Contains(buf.String(), "slow down") {
		t.Fatalf("provider message must not be in the error string: %s", buf.String())
	}
}

func TestResponsesDoesNotRetryClientErrors(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(_ int, w http.ResponseWriter) {
		respond(w, 400, `{"error":{"message":"bad schema","type":"invalid_request_error","code":"invalid_json_schema"}}`)
	})
	c, buf := newClient(t, srv, retry.Policy{Backoff: retry.Exponential(time.Second, time.Second, 3)})

	_, err := c.Responses(context.Background(), Request{Model: "m", Input: []Input{UserText("hi")}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 || apiErr.Type != "invalid_request_error" || apiErr.Code != "invalid_json_schema" || apiErr.Message != "bad schema" || apiErr.RequestID != "req_x" {
		t.Fatalf("err=%v", err)
	}
	if IsTemporary(err) || srv.calls.Load() != 1 {
		t.Fatalf("400 must not be retried: temporary=%v calls=%d", IsTemporary(err), srv.calls.Load())
	}

	finished := allEntries(t, buf)
	last := finished[len(finished)-1]
	assertField(t, last, "event", logger.EventSpanFinished)
	assertField(t, last, "status", logger.StatusError)
	assertField(t, last, "http_status_code", float64(400))
	assertField(t, last, "provider_type", "invalid_request_error")
	assertField(t, last, "provider_code", "invalid_json_schema")
	assertField(t, last, "error_type", "openai.APIError")
}

func TestResponsesReturnsLastErrorWhenExhausted(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(_ int, w http.ResponseWriter) { respond(w, 500, `{}`) })
	c, _ := newClient(t, srv, retry.Policy{Backoff: retry.Exponential(time.Second, time.Second, 2)})

	_, err := c.Responses(context.Background(), Request{Model: "m"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 500 || !IsTemporary(err) || srv.calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d", err, srv.calls.Load())
	}
}

func TestResponsesAttemptTimeoutIsRetried(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(n int, w http.ResponseWriter) {
		if n == 1 {
			time.Sleep(200 * time.Millisecond) // 1 回目は AttemptTimeout を超えさせる
		}
		respond(w, 200, completedBody)
	})
	c, _ := newClient(t, srv, retry.Policy{Backoff: []time.Duration{time.Second}, AttemptTimeout: 50 * time.Millisecond})

	resp, err := c.Responses(context.Background(), Request{Model: "m"})
	if err != nil || resp.ID != "resp_1" || srv.calls.Load() != 2 {
		t.Fatalf("err=%v calls=%d", err, srv.calls.Load())
	}
}

func TestResponsesStopsWhenParentContextIsCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	srv := newFakeServer(t, func(_ int, w http.ResponseWriter) {
		cancel()
		time.Sleep(50 * time.Millisecond)
		respond(w, 200, completedBody)
	})
	c, _ := newClient(t, srv, retry.Policy{Backoff: retry.Exponential(time.Second, time.Second, 3)})

	_, err := c.Responses(ctx, Request{Model: "m"})
	if !errors.Is(err, context.Canceled) || IsTemporary(err) || srv.calls.Load() != 1 {
		t.Fatalf("err=%v temporary=%v calls=%d", err, IsTemporary(err), srv.calls.Load())
	}
}

func TestResponsesIncompleteAndRefusal(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(n int, w http.ResponseWriter) {
		if n == 1 {
			respond(w, 200, `{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`)
			return
		}
		respond(w, 200, `{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"no"}]}]}`)
	})
	c, _ := newClient(t, srv, retry.Policy{})

	resp, err := c.Responses(context.Background(), Request{Model: "m"})
	if err != nil || resp.Status != "incomplete" || resp.IncompleteReason != "max_output_tokens" || resp.OutputText != "" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	resp, err = c.Responses(context.Background(), Request{Model: "m"})
	if err != nil || resp.Refusal != "no" || resp.OutputText != "" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestResponsesInvalidJSONBodyIsTransportError(t *testing.T) {
	t.Parallel()
	srv := newFakeServer(t, func(_ int, w http.ResponseWriter) { respond(w, 200, `not json`) })
	c, _ := newClient(t, srv, retry.Policy{})

	_, err := c.Responses(context.Background(), Request{Model: "m"})
	var transport *TransportError
	if !errors.As(err, &transport) || !IsTemporary(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestIsTemporary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"429", &APIError{StatusCode: 429}, true},
		{"503", &APIError{StatusCode: 503}, true},
		{"400", &APIError{StatusCode: 400}, false},
		{"401", &APIError{StatusCode: 401}, false},
		{"failed", &ResponseFailedError{}, true},
		{"transport", &TransportError{Err: errors.New("eof")}, true},
		{"transport wrapping cancel", &TransportError{Err: context.Canceled}, false},
		{"deadline", context.DeadlineExceeded, true},
		{"canceled", context.Canceled, false},
		{"other", errors.New("x"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsTemporary(tt.err); got != tt.want {
				t.Fatalf("IsTemporary(%v)=%v want %v", tt.err, got, tt.want)
			}
		})
	}
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
