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
	uploads           []domain.UploadHistory
	err               error
}

func (l *lister) ListByMonth(_ context.Context, userID, yearMonth string) ([]domain.UploadHistory, error) {
	l.called, l.userID, l.yearMonth = true, userID, yearMonth
	if l.err != nil {
		return nil, l.err
	}
	return l.uploads, nil
}

func TestListReturnsHistoriesForTheMonth(t *testing.T) {
	t.Parallel()
	histories := &lister{uploads: []domain.UploadHistory{
		{UserID: "u1", UploadID: "up2", Status: domain.StatusSucceeded, BillingID: "b1", YearMonth: "2026-09", CreatedAt: now.Add(time.Minute)},
		{UserID: "u1", UploadID: "up1", Status: domain.StatusFailed, ErrorCode: "NO_DATE", ErrorMessage: "date not found", YearMonth: "2026-09", CreatedAt: now},
	}}
	out, err := application.NewListUploadsUsecase(histories).List(context.Background(), application.ListUploadsInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if histories.userID != "u1" || histories.yearMonth != "2026-09" {
		t.Errorf("ListByMonth args = %q / %q", histories.userID, histories.yearMonth)
	}
	if len(out.Uploads) != 2 || out.Uploads[0] != histories.uploads[0] || out.Uploads[1] != histories.uploads[1] {
		t.Errorf("List() = %+v, want %+v", out.Uploads, histories.uploads)
	}
}

func TestListReturnsEmptyWhenNothingUploaded(t *testing.T) {
	t.Parallel()
	out, err := application.NewListUploadsUsecase(&lister{uploads: []domain.UploadHistory{}}).List(context.Background(), application.ListUploadsInput{UserID: "u1", YearMonth: "2026-09"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Uploads) != 0 {
		t.Errorf("List() = %+v, want empty", out.Uploads)
	}
}

func TestListRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]application.ListUploadsInput{
		"missing user":       {YearMonth: "2026-09"},
		"blank user":         {UserID: " ", YearMonth: "2026-09"},
		"missing month":      {UserID: "u1"},
		"month with day":     {UserID: "u1", YearMonth: "2026-09-01"},
		"month out of range": {UserID: "u1", YearMonth: "2026-13"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			histories := &lister{}
			if _, err := application.NewListUploadsUsecase(histories).List(context.Background(), in); !errors.Is(err, application.ErrInvalidInput) {
				t.Fatalf("List() error = %v, want ErrInvalidInput", err)
			}
			if histories.called {
				t.Errorf("ListByMonth should not be called for %+v", in)
			}
		})
	}
}

func TestListWrapsRepositoryFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	_, err := application.NewListUploadsUsecase(&lister{err: cause}).List(context.Background(), application.ListUploadsInput{UserID: "u1", YearMonth: "2026-09"})
	if !errors.Is(err, cause) {
		t.Fatalf("List() error = %v, want wrapped %v", err, cause)
	}
}
