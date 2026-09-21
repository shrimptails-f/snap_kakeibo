package domain

import (
	"errors"
	"testing"
	"time"
)

func TestAnalysisRequestTransitionsToCompleted(t *testing.T) {
	t.Parallel()

	request := newTestAnalysisRequest(t)
	attempt, _ := NewAttempt(1)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := request.StartAnalysis(attempt, now); err != nil {
		t.Fatalf("StartAnalysis() error = %v", err)
	}
	expenseID, _ := NewExpenseID("expense-1")
	if err := request.Complete(attempt, expenseID, now.Add(time.Minute)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if request.Status() != AnalysisStatusSucceeded || request.ExpenseID() != expenseID {
		t.Errorf("completed request = status %q, expense %q", request.Status(), request.ExpenseID())
	}
}

func TestAnalysisRequestRejectsOldAttempt(t *testing.T) {
	t.Parallel()

	request := newTestAnalysisRequest(t)
	wrongAttempt, _ := NewAttempt(2)
	if err := request.StartAnalysis(wrongAttempt, time.Now()); !errors.Is(err, ErrAttemptMismatch) {
		t.Errorf("StartAnalysis() error = %v, want ErrAttemptMismatch", err)
	}
}

func TestAnalysisRequestRetryAdvancesTerminalAttempt(t *testing.T) {
	t.Parallel()

	request := newTestAnalysisRequest(t)
	attempt, _ := NewAttempt(1)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	if err := request.StartAnalysis(attempt, now); err != nil {
		t.Fatalf("StartAnalysis() error = %v", err)
	}
	if err := request.MarkNoData(attempt, now); err != nil {
		t.Fatalf("MarkNoData() error = %v", err)
	}
	if err := request.Retry(now.Add(time.Minute)); err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if request.Status() != AnalysisStatusAnalyzing || request.CurrentAttempt().Int() != 2 {
		t.Errorf("retried request = status %q, attempt %d", request.Status(), request.CurrentAttempt().Int())
	}
}

func TestAnalysisRequestDoesNotDecideAnalyzingRetryPolicy(t *testing.T) {
	t.Parallel()

	request := newTestAnalysisRequest(t)
	attempt, _ := NewAttempt(1)
	if err := request.StartAnalysis(attempt, time.Now()); err != nil {
		t.Fatalf("StartAnalysis() error = %v", err)
	}
	if err := request.Retry(time.Now()); !errors.Is(err, ErrRetryPolicyUndecided) {
		t.Errorf("Retry() error = %v, want ErrRetryPolicyUndecided", err)
	}
}

func TestAnalysisResultWithNoDetailsHasNoData(t *testing.T) {
	t.Parallel()

	date, _ := NewPurchaseDate("2026-09-21")
	amount, _ := NewReadAmount(1_000)
	result, err := NewAnalysisResult("店", date, amount, nil)
	if err != nil {
		t.Fatalf("NewAnalysisResult() error = %v", err)
	}
	if result.HasData() {
		t.Error("HasData() = true, want false")
	}
}

func newTestAnalysisRequest(t *testing.T) AnalysisRequest {
	t.Helper()
	id, _ := NewAnalysisRequestID("request-1")
	userID, _ := NewUserID("user-1")
	image, err := NewReceiptImage("receipts/user-1/request-1/original.jpg", "receipt.jpg", "image/jpeg")
	if err != nil {
		t.Fatalf("NewReceiptImage() error = %v", err)
	}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	request, err := NewAnalysisRequest(id, userID, image, now.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("NewAnalysisRequest() error = %v", err)
	}
	return request
}
