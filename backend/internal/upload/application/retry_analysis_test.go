package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

type retryMarker struct {
	userID, requestID string
	now               time.Time
	attempt           int
	err               error
}

func (m *retryMarker) MarkRetrying(_ context.Context, userID, requestID string, now time.Time) (int, error) {
	m.userID, m.requestID, m.now = userID, requestID, now
	if m.err != nil {
		return 0, m.err
	}
	return m.attempt, nil
}

type enqueuer struct {
	jobs []domain.RetryAnalysisJob
	err  error
}

func (e *enqueuer) EnqueueRetryAnalysis(_ context.Context, job domain.RetryAnalysisJob) error {
	if e.err != nil {
		return e.err
	}
	e.jobs = append(e.jobs, job)
	return nil
}

// retryFixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type retryFixture struct {
	requests *retryMarker
	queue    *enqueuer
}

func newRetryFixture() *retryFixture {
	return &retryFixture{requests: &retryMarker{attempt: 2}, queue: &enqueuer{}}
}

func (f *retryFixture) build() application.RetryAnalysisUsecaseInterface {
	return application.NewRetryAnalysisUsecase(f.requests, f.queue, timewrapper.NewFixed(now))
}

func TestRetryMarksRequestThenEnqueues(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.requests.attempt = 3
	out, err := f.build().Retry(context.Background(), application.RetryAnalysisInput{UserID: "u1", AnalysisRequestID: "req1"})
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if want := (application.RetryAnalysisOutput{AnalysisRequestID: "req1", Status: analysisdomain.AnalysisStatusAnalyzing, Attempt: 3}); out != want {
		t.Errorf("Retry() = %+v, want %+v", out, want)
	}
	if f.requests.userID != "u1" || f.requests.requestID != "req1" || !f.requests.now.Equal(now) {
		t.Errorf("MarkRetrying args = %+v", f.requests)
	}
	if want := []domain.RetryAnalysisJob{{UserID: "u1", AnalysisRequestID: "req1", Attempt: 3}}; len(f.queue.jobs) != 1 || f.queue.jobs[0] != want[0] {
		t.Errorf("enqueued = %+v, want %+v", f.queue.jobs, want)
	}
}

func TestRetryRejectsMissingIDs(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.RetryAnalysisInput{
		"missing user":    {AnalysisRequestID: "req1"},
		"missing request": {UserID: "u1", AnalysisRequestID: " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newRetryFixture()
			if _, err := f.build().Retry(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("Retry() error = %v, want ErrInvalidInput", err)
			}
			if f.requests.userID != "" || len(f.queue.jobs) != 0 {
				t.Errorf("no side effects expected: %+v %+v", f.requests, f.queue)
			}
		})
	}
}

func TestRetryDoesNotEnqueueWhenNotRetryable(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.requests.err = application.ErrAnalysisRequestNotRetryable
	_, err := f.build().Retry(context.Background(), application.RetryAnalysisInput{UserID: "u1", AnalysisRequestID: "req1"})
	if !errors.Is(err, application.ErrAnalysisRequestNotRetryable) {
		t.Fatalf("Retry() error = %v, want ErrAnalysisRequestNotRetryable", err)
	}
	if len(f.queue.jobs) != 0 {
		t.Errorf("EnqueueRetryAnalysis should not be called, got %+v", f.queue.jobs)
	}
}

func TestRetryDoesNotEnqueueWhenMarkFails(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.requests.err = errors.New("boom")
	if _, err := f.build().Retry(context.Background(), application.RetryAnalysisInput{UserID: "u1", AnalysisRequestID: "req1"}); err == nil || len(f.queue.jobs) != 0 {
		t.Fatalf("Retry() error = %v, enqueued = %+v", err, f.queue.jobs)
	}
}

func TestRetryRejectsAttemptThatDidNotAdvance(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	// 解析依頼の attempt が壊れていて進めても 1 以下のとき。キューへ送ると analyze-receipt が S3 起点と区別できない
	f.requests.attempt = domain.FirstAttempt
	_, err := f.build().Retry(context.Background(), application.RetryAnalysisInput{UserID: "u1", AnalysisRequestID: "req1"})
	if !errors.Is(err, domain.ErrInvalidRetryAnalysisJob) {
		t.Fatalf("Retry() error = %v, want ErrInvalidRetryAnalysisJob", err)
	}
	if len(f.queue.jobs) != 0 {
		t.Errorf("EnqueueRetryAnalysis should not be called, got %+v", f.queue.jobs)
	}
}

func TestRetryWrapsEnqueueFailure(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.queue.err = errors.New("boom")
	if _, err := f.build().Retry(context.Background(), application.RetryAnalysisInput{UserID: "u1", AnalysisRequestID: "req1"}); err == nil {
		t.Fatal("Retry() error = nil, want enqueue failure")
	}
}
