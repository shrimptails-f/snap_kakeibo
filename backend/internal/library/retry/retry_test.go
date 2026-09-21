package retry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

var errTemporary = errors.New("temporary")

func TestExponential(t *testing.T) {
	t.Parallel()
	got := Exponential(time.Second, 16*time.Second, 6)
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 16 * time.Second}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if Exponential(time.Second, time.Minute, 0) != nil {
		t.Fatal("n=0 must be nil")
	}
	if (Policy{}).MaxAttempts() != 1 || (Policy{Backoff: got}).MaxAttempts() != 7 {
		t.Fatal("MaxAttempts")
	}
}

func TestDoSucceedsAfterRetries(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger()
	clock := timewrapper.NewFixed(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := New(log, clock)

	calls := 0
	err := r.Do(context.Background(), "op", Policy{Backoff: []time.Duration{time.Second, 2 * time.Second}}, func(context.Context) error {
		calls++
		if calls < 3 {
			return errTemporary
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}

	entries := allEntries(t, buf)
	if len(entries) != 3 {
		t.Fatalf("entries=%d: %s", len(entries), buf.String())
	}
	assertField(t, entries[0], "event", EventRetryAttempt)
	assertField(t, entries[0], "attempt", float64(2))
	assertField(t, entries[0], "delay_ms", float64(1000))
	assertField(t, entries[0], "error", "temporary")
	assertField(t, entries[0], "error_type", "errors.errorString")
	assertField(t, entries[1], "event", EventRetryAttempt)
	assertField(t, entries[1], "attempt", float64(3))
	assertField(t, entries[1], "delay_ms", float64(2000))
	assertField(t, entries[2], "event", EventRetrySucceeded)
	assertField(t, entries[2], "attempt", float64(3))
	for _, e := range entries {
		assertField(t, e, "retry_name", "op")
		assertField(t, e, "max_attempts", float64(3))
	}
}

func TestDoExhausted(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger()
	r := New(log, timewrapper.NewFixed(time.Now()))

	calls := 0
	err := r.Do(context.Background(), "op", Policy{Backoff: []time.Duration{time.Second, time.Second}}, func(context.Context) error {
		calls++
		return errTemporary
	})
	if !errors.Is(err, errTemporary) || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}

	entries := allEntries(t, buf)
	last := entries[len(entries)-1]
	assertField(t, last, "event", EventRetryExhausted)
	assertField(t, last, "level", "WARN")
	assertField(t, last, "attempt", float64(3))
	assertField(t, last, "error", "temporary")
}

func TestDoWithoutBackoffRunsOnce(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger()
	r := New(log, timewrapper.NewFixed(time.Now()))

	calls := 0
	err := r.Do(context.Background(), "op", Policy{}, func(context.Context) error {
		calls++
		return errTemporary
	})
	if !errors.Is(err, errTemporary) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	// 1 回で諦めるときも retry_exhausted は出す(呼び出し側がリトライ無しで動いていることが分かる)
	entries := allEntries(t, buf)
	if len(entries) != 1 || entries[0]["event"] != EventRetryExhausted {
		t.Fatalf("entries=%v", entries)
	}
}

func TestDoStopsWhenShouldRetryIsFalse(t *testing.T) {
	t.Parallel()
	log, buf := newTestLogger()
	r := New(log, timewrapper.NewFixed(time.Now()))
	permanent := errors.New("permanent")

	calls := 0
	err := r.Do(context.Background(), "op", Policy{
		Backoff:     []time.Duration{time.Second, time.Second},
		ShouldRetry: func(err error) bool { return errors.Is(err, errTemporary) },
	}, func(context.Context) error {
		calls++
		if calls == 1 {
			return errTemporary
		}
		return permanent
	})
	if !errors.Is(err, permanent) || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if strings.Contains(buf.String(), EventRetryExhausted) {
		t.Fatalf("permanent error must not be reported as exhausted: %s", buf.String())
	}
}

func TestDoStopsWhenParentContextIsCanceledDuringWait(t *testing.T) {
	t.Parallel()
	// 実時計を使い、待ちの途中で親 ctx を切る
	r := New(nil, timewrapper.NewClock())
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := r.Do(ctx, "op", Policy{Backoff: []time.Duration{time.Minute}}, func(context.Context) error {
		calls++
		cancel()
		return errTemporary
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errTemporary) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestDoDoesNotRetryAfterParentDeadline(t *testing.T) {
	t.Parallel()
	r := New(nil, timewrapper.NewFixed(time.Now()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	calls := 0
	err := r.Do(ctx, "op", Policy{
		Backoff:     []time.Duration{time.Second, time.Second},
		ShouldRetry: func(error) bool { return true },
	}, func(ctx context.Context) error {
		calls++
		<-ctx.Done() // 親の期限が来るまで待つ
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("err=%v calls=%d (must not retry once the parent deadline passed)", err, calls)
	}
}

func TestDoAttemptTimeoutAppliesPerAttempt(t *testing.T) {
	t.Parallel()
	r := New(nil, timewrapper.NewFixed(time.Now()))

	var deadlines []time.Duration
	calls := 0
	err := r.Do(context.Background(), "op", Policy{
		Backoff:        []time.Duration{time.Second},
		AttemptTimeout: 50 * time.Millisecond,
	}, func(ctx context.Context) error {
		calls++
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("attempt ctx must have a deadline")
		}
		deadlines = append(deadlines, time.Until(dl))
		if calls == 1 {
			<-ctx.Done() // 1 回目はタイムアウトさせる
			return ctx.Err()
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	for i, d := range deadlines {
		if d <= 0 || d > 50*time.Millisecond {
			t.Fatalf("attempt %d: remaining %v not within AttemptTimeout", i+1, d)
		}
	}
}

func TestDoWithoutAttemptTimeoutPassesParentContext(t *testing.T) {
	t.Parallel()
	r := New(nil, timewrapper.NewFixed(time.Now()))
	parent := logger.ContextWith(context.Background(), logger.AnalysisRequestID("u1"))

	err := r.Do(parent, "op", Policy{}, func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("no AttemptTimeout -> no deadline")
		}
		fields := logger.FieldsFromContext(ctx)
		if len(fields) == 0 || fields[0].Key != "analysis_request_id" {
			t.Fatalf("parent fields must propagate: %v", fields)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoNilOpAndNilContext(t *testing.T) {
	t.Parallel()
	r := New(nil, nil)
	if err := r.Do(context.Background(), "op", Policy{}, nil); err == nil {
		t.Fatal("nil op must be an error")
	}
	var nilContext context.Context
	if err := r.Do(nilContext, "op", Policy{}, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
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
