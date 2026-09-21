package domain

import (
	"errors"
	"testing"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
)

var now = time.Date(2026, 9, 30, 23, 30, 0, 0, time.FixedZone("JST", 9*60*60))

func TestNewUploadRequestBuildsInitialState(t *testing.T) {
	t.Parallel()
	got, err := NewUploadRequest("u1", "req1", "レシート.png", "image/png", now, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewUploadRequest() error = %v", err)
	}
	if got.ID() != "req1" || got.UserID() != "u1" || got.Status() != analysisdomain.AnalysisStatusUploading || got.CurrentAttempt().Int() != FirstAttempt {
		t.Errorf("request = id %q user %q status %q attempt %d", got.ID(), got.UserID(), got.Status(), got.CurrentAttempt().Int())
	}
	image := got.Image()
	if image.Reference() != "receipts/u1/req1/original.jpg" || image.FileName() != "レシート.png" || image.ContentType() != "image/png" {
		t.Errorf("image = %+v", image)
	}
	// JST 30 日 23:30 は UTC では 30 日 14:30
	createdAt := time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC)
	if !got.CreatedAt().Equal(createdAt) || !got.UpdatedAt().Equal(createdAt) || !got.UploadExpiresAt().Equal(createdAt.Add(15*time.Minute)) {
		t.Errorf("times = created %v updated %v expires %v", got.CreatedAt(), got.UpdatedAt(), got.UploadExpiresAt())
	}
	if got := YearMonth(got.CreatedAt()); got != "2026-09" {
		t.Errorf("YearMonth() = %q, want 2026-09", got)
	}
}

func TestNewUploadRequestAppliesDefaults(t *testing.T) {
	t.Parallel()
	got, err := NewUploadRequest("u1", "req1", " ", "", now, time.Minute)
	if err != nil {
		t.Fatalf("NewUploadRequest() error = %v", err)
	}
	if got.Image().FileName() != DefaultFileName || got.Image().ContentType() != DefaultContentType {
		t.Errorf("defaults = %q / %q", got.Image().FileName(), got.Image().ContentType())
	}
}

func TestNewUploadRequestRejectsMissingIDs(t *testing.T) {
	t.Parallel()
	for name, ids := range map[string][2]string{"user": {"", "req1"}, "request": {"u1", " "}} {
		if _, err := NewUploadRequest(ids[0], ids[1], "", "", now, time.Minute); !errors.Is(err, ErrInvalidUpload) {
			t.Errorf("%s: error = %v, want ErrInvalidUpload", name, err)
		}
	}
}

func TestRetryable(t *testing.T) {
	t.Parallel()
	for status, want := range map[AnalysisStatus]bool{
		analysisdomain.AnalysisStatusFailed:    true,
		analysisdomain.AnalysisStatusNoData:    true,
		analysisdomain.AnalysisStatusAnalyzing: true,
		analysisdomain.AnalysisStatusUploading: false,
		analysisdomain.AnalysisStatusSucceeded: false,
	} {
		if got := Retryable(status); got != want {
			t.Errorf("Retryable(%s) = %v, want %v", status, got, want)
		}
	}
}
