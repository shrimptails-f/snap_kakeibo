package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

type retryMarker struct {
	userID, uploadID string
	now              time.Time
	attempt          int
	err              error
}

func (m *retryMarker) MarkRetrying(_ context.Context, userID, uploadID string, now time.Time) (int, error) {
	m.userID, m.uploadID, m.now = userID, uploadID, now
	if m.err != nil {
		return 0, m.err
	}
	return m.attempt, nil
}

type enqueuer struct {
	jobs []domain.RetryJob
	err  error
}

func (e *enqueuer) EnqueueRetry(_ context.Context, job domain.RetryJob) error {
	if e.err != nil {
		return e.err
	}
	e.jobs = append(e.jobs, job)
	return nil
}

// retryFixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type retryFixture struct {
	histories *retryMarker
	queue     *enqueuer
}

func newRetryFixture() *retryFixture {
	return &retryFixture{histories: &retryMarker{attempt: 2}, queue: &enqueuer{}}
}

func (f *retryFixture) build() application.RetryUploadUsecaseInterface {
	return application.NewRetryUploadUsecase(f.histories, f.queue, timewrapper.NewFixed(now))
}

func TestRetryMarksHistoryThenEnqueues(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.histories.attempt = 3
	out, err := f.build().Retry(context.Background(), application.RetryUploadInput{UserID: "u1", UploadID: "up1"})
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if want := (application.RetryUploadOutput{UploadID: "up1", Status: domain.StatusAnalyzing, Attempt: 3}); out != want {
		t.Errorf("Retry() = %+v, want %+v", out, want)
	}
	if f.histories.userID != "u1" || f.histories.uploadID != "up1" || !f.histories.now.Equal(now) {
		t.Errorf("MarkRetrying args = %+v", f.histories)
	}
	if want := []domain.RetryJob{{UserID: "u1", UploadID: "up1", Attempt: 3}}; len(f.queue.jobs) != 1 || f.queue.jobs[0] != want[0] {
		t.Errorf("enqueued = %+v, want %+v", f.queue.jobs, want)
	}
}

func TestRetryRejectsMissingIDs(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.RetryUploadInput{
		"missing user":   {UploadID: "up1"},
		"missing upload": {UserID: "u1", UploadID: " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newRetryFixture()
			if _, err := f.build().Retry(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("Retry() error = %v, want ErrInvalidInput", err)
			}
			if f.histories.userID != "" || len(f.queue.jobs) != 0 {
				t.Errorf("no side effects expected: %+v %+v", f.histories, f.queue)
			}
		})
	}
}

func TestRetryDoesNotEnqueueWhenNotRetryable(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.histories.err = application.ErrUploadNotRetryable
	_, err := f.build().Retry(context.Background(), application.RetryUploadInput{UserID: "u1", UploadID: "up1"})
	if !errors.Is(err, application.ErrUploadNotRetryable) {
		t.Fatalf("Retry() error = %v, want ErrUploadNotRetryable", err)
	}
	if len(f.queue.jobs) != 0 {
		t.Errorf("EnqueueRetry should not be called, got %+v", f.queue.jobs)
	}
}

func TestRetryDoesNotEnqueueWhenMarkFails(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.histories.err = errors.New("boom")
	if _, err := f.build().Retry(context.Background(), application.RetryUploadInput{UserID: "u1", UploadID: "up1"}); err == nil || len(f.queue.jobs) != 0 {
		t.Fatalf("Retry() error = %v, enqueued = %+v", err, f.queue.jobs)
	}
}

func TestRetryRejectsAttemptThatDidNotAdvance(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	// 履歴の attempt が壊れていて進めても 1 以下のとき。キューへ送ると analyze-receipt が S3 起点と区別できない
	f.histories.attempt = domain.FirstAttempt
	_, err := f.build().Retry(context.Background(), application.RetryUploadInput{UserID: "u1", UploadID: "up1"})
	if !errors.Is(err, domain.ErrInvalidRetryJob) {
		t.Fatalf("Retry() error = %v, want ErrInvalidRetryJob", err)
	}
	if len(f.queue.jobs) != 0 {
		t.Errorf("EnqueueRetry should not be called, got %+v", f.queue.jobs)
	}
}

func TestRetryWrapsEnqueueFailure(t *testing.T) {
	t.Parallel()
	f := newRetryFixture()
	f.queue.err = errors.New("boom")
	if _, err := f.build().Retry(context.Background(), application.RetryUploadInput{UserID: "u1", UploadID: "up1"}); err == nil {
		t.Fatal("Retry() error = nil, want enqueue failure")
	}
}
