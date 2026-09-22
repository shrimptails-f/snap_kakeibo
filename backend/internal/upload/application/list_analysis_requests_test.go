package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

type lister struct {
	query  application.AnalysisRequestListQuery
	called bool
	page   application.AnalysisRequestPage
	err    error
}

func (l *lister) ListPage(_ context.Context, query application.AnalysisRequestListQuery) (application.AnalysisRequestPage, error) {
	l.called, l.query = true, query
	return l.page, l.err
}

type expenseReader struct {
	userID, expenseID string
	summaries         map[string]application.ExpenseSummary
	err               error
}

func (r *expenseReader) FindSummaries(_ context.Context, userID string, expenseIDs []string) (map[string]application.ExpenseSummary, error) {
	r.userID = userID
	if len(expenseIDs) > 0 {
		r.expenseID = expenseIDs[0]
	}
	return r.summaries, r.err
}

func uploadRequest(t *testing.T, id string, createdAt time.Time) domain.AnalysisRequest {
	t.Helper()
	request, err := domain.NewUploadRequest("u1", id, "", "", createdAt, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewUploadRequest(%s) error = %v", id, err)
	}
	return request
}

func succeededRequest(t *testing.T, id, expenseID string) domain.AnalysisRequest {
	t.Helper()
	request := uploadRequest(t, id, now)
	attempt, _ := analysisdomain.NewAttempt(1)
	if err := request.StartAnalysis(attempt, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expense, _ := common.NewExpenseID(expenseID)
	if err := request.Complete(attempt, expense, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return request
}

func newListUsecase(requests *lister, expenses *expenseReader) application.ListAnalysisRequestsUsecaseInterface {
	return application.NewListAnalysisRequestsUsecase(requests, expenses, timewrapper.NewFixed(now))
}

func TestListReturnsFilteredPageWithExpenseSummary(t *testing.T) {
	t.Parallel()
	completed := succeededRequest(t, "req1", "expense1")
	requests := &lister{page: application.AnalysisRequestPage{Requests: []domain.AnalysisRequest{completed}, NextCursor: "next"}}
	expenses := &expenseReader{summaries: map[string]application.ExpenseSummary{"expense1": {ExpenseID: "expense1", StoreName: "スーパー", RecordedAmount: 2780}}}
	out, err := newListUsecase(requests, expenses).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09", Filter: "attention", Cursor: "current"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if requests.query.UserID != "u1" || requests.query.YearMonth != "2026-09" || requests.query.Filter != application.AnalysisRequestFilterAttention || requests.query.Cursor != "current" || requests.query.PageSize != 20 || !requests.query.Now.Equal(now) {
		t.Errorf("ListPage query = %+v", requests.query)
	}
	if out.NextCursor != "next" || len(out.Items) != 1 || out.Items[0].ExpenseSummary == nil || out.Items[0].ExpenseSummary.RecordedAmount != 2780 {
		t.Errorf("List() = %+v", out)
	}
	if expenses.userID != "u1" || expenses.expenseID != "expense1" {
		t.Errorf("FindSummaries args = %q / %q", expenses.userID, expenses.expenseID)
	}
}

func TestListDefaultsToAllAndReturnsEmpty(t *testing.T) {
	t.Parallel()
	requests := &lister{page: application.AnalysisRequestPage{Requests: []domain.AnalysisRequest{}}}
	out, err := newListUsecase(requests, &expenseReader{}).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if requests.query.Filter != application.AnalysisRequestFilterAll || len(out.Items) != 0 {
		t.Errorf("List() query/output = %+v / %+v", requests.query, out)
	}
}

func TestListRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.ListAnalysisRequestsInput{
		"missing user":       {YearMonth: "2026-09"},
		"missing month":      {UserID: "u1"},
		"month out of range": {UserID: "u1", YearMonth: "2026-13"},
		"unknown filter":     {UserID: "u1", YearMonth: "2026-09", Filter: "failed"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requests := &lister{}
			if _, err := newListUsecase(requests, &expenseReader{}).List(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("List() error = %v, want ErrInvalidInput", err)
			}
			if requests.called {
				t.Fatal("ListPage should not be called")
			}
		})
	}
}

func TestListWrapsDependencyFailures(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	if _, err := newListUsecase(&lister{err: cause}, &expenseReader{}).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09"}); !errors.Is(err, cause) {
		t.Fatalf("List() repository error = %v", err)
	}
	request := uploadRequest(t, "req1", now)
	if _, err := newListUsecase(&lister{page: application.AnalysisRequestPage{Requests: []domain.AnalysisRequest{request}}}, &expenseReader{err: cause}).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09"}); !errors.Is(err, cause) {
		t.Fatalf("List() expense error = %v", err)
	}
}
