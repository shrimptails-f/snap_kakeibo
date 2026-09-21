package domain

import (
	"errors"
	"testing"
)

func TestNewRetryJobBuildsJob(t *testing.T) {
	t.Parallel()
	got, err := NewRetryJob("u1", "up1", 2)
	if err != nil {
		t.Fatalf("NewRetryJob() error = %v", err)
	}
	if want := (RetryJob{UserID: "u1", UploadID: "up1", Attempt: 2}); got != want {
		t.Errorf("NewRetryJob() = %+v, want %+v", got, want)
	}
}

func TestNewRetryJobRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		userID, uploadID string
		attempt          int
	}{
		"missing user":     {"", "up1", 2},
		"missing upload":   {"u1", " ", 2},
		"first attempt":    {"u1", "up1", FirstAttempt},
		"attempt not set":  {"u1", "up1", 0},
		"negative attempt": {"u1", "up1", -1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewRetryJob(tt.userID, tt.uploadID, tt.attempt); !errors.Is(err, ErrInvalidRetryJob) {
				t.Errorf("NewRetryJob() error = %v, want ErrInvalidRetryJob", err)
			}
		})
	}
}

func TestStatusRetryable(t *testing.T) {
	t.Parallel()
	want := map[Status]bool{
		StatusFailed: true, StatusNoData: true, StatusAnalyzing: true,
		StatusUploading: false, StatusSucceeded: false, Status(""): false, Status("UNKNOWN"): false,
	}
	for status, retryable := range want {
		if got := status.Retryable(); got != retryable {
			t.Errorf("%q.Retryable() = %v, want %v", status, got, retryable)
		}
	}
}
