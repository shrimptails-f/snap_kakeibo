package domain

import (
	"errors"
	"testing"
)

func TestNewRetryAnalysisJobBuildsJob(t *testing.T) {
	t.Parallel()
	got, err := NewRetryAnalysisJob("u1", "req1", 2)
	if err != nil {
		t.Fatalf("NewRetryAnalysisJob() error = %v", err)
	}
	if want := (RetryAnalysisJob{UserID: "u1", AnalysisRequestID: "req1", Attempt: 2}); got != want {
		t.Errorf("NewRetryAnalysisJob() = %+v, want %+v", got, want)
	}
}

func TestNewRetryAnalysisJobRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		userID, requestID string
		attempt           int
	}{
		"missing user":     {"", "req1", 2},
		"missing request":  {"u1", " ", 2},
		"first attempt":    {"u1", "req1", FirstAttempt},
		"attempt not set":  {"u1", "req1", 0},
		"negative attempt": {"u1", "req1", -1},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewRetryAnalysisJob(tt.userID, tt.requestID, tt.attempt); !errors.Is(err, ErrInvalidRetryAnalysisJob) {
				t.Errorf("NewRetryAnalysisJob() error = %v, want ErrInvalidRetryAnalysisJob", err)
			}
		})
	}
}
