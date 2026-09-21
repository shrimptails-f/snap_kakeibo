package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

type lister struct {
	userID, yearMonth string
	called            bool
	requests          []domain.AnalysisRequest
	err               error
}

func (l *lister) ListByMonth(_ context.Context, userID, yearMonth string) ([]domain.AnalysisRequest, error) {
	l.called, l.userID, l.yearMonth = true, userID, yearMonth
	if l.err != nil {
		return nil, l.err
	}
	return l.requests, nil
}

func uploadRequest(t *testing.T, id string, createdAt time.Time) domain.AnalysisRequest {
	t.Helper()
	request, err := domain.NewUploadRequest("u1", id, "", "", createdAt, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewUploadRequest(%s) error = %v", id, err)
	}
	return request
}

func TestListReturnsRequestsForTheMonth(t *testing.T) {
	t.Parallel()
	requests := &lister{requests: []domain.AnalysisRequest{uploadRequest(t, "req2", now.Add(time.Minute)), uploadRequest(t, "req1", now)}}
	out, err := application.NewListAnalysisRequestsUsecase(requests).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if requests.userID != "u1" || requests.yearMonth != "2026-09" {
		t.Errorf("ListByMonth args = %q / %q", requests.userID, requests.yearMonth)
	}
	if len(out.Requests) != 2 || out.Requests[0].ID() != "req2" || out.Requests[1].ID() != "req1" {
		t.Errorf("List() = %+v, want %+v", out.Requests, requests.requests)
	}
}

func TestListReturnsEmptyWhenNothingUploaded(t *testing.T) {
	t.Parallel()
	out, err := application.NewListAnalysisRequestsUsecase(&lister{requests: []domain.AnalysisRequest{}}).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Requests) != 0 {
		t.Errorf("List() = %+v, want empty", out.Requests)
	}
}

func TestListRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.ListAnalysisRequestsInput{
		"missing user":       {YearMonth: "2026-09"},
		"blank user":         {UserID: " ", YearMonth: "2026-09"},
		"missing month":      {UserID: "u1"},
		"month with day":     {UserID: "u1", YearMonth: "2026-09-01"},
		"single digit month": {UserID: "u1", YearMonth: "2026-9"},
		"month out of range": {UserID: "u1", YearMonth: "2026-13"},
		"whitespace":         {UserID: "u1", YearMonth: " 2026-09"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requests := &lister{}
			if _, err := application.NewListAnalysisRequestsUsecase(requests).List(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("List() error = %v, want ErrInvalidInput", err)
			}
			if requests.called {
				t.Errorf("ListByMonth should not be called for %+v", in)
			}
		})
	}
}

func TestListWrapsRepositoryFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	_, err := application.NewListAnalysisRequestsUsecase(&lister{err: cause}).List(context.Background(), application.ListAnalysisRequestsInput{UserID: "u1", YearMonth: "2026-09"})
	if !errors.Is(err, cause) {
		t.Fatalf("List() error = %v, want wrapped %v", err, cause)
	}
}
