package domain

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 30, 23, 30, 0, 0, time.FixedZone("JST", 9*60*60))

func TestNewUploadHistoryBuildsInitialState(t *testing.T) {
	t.Parallel()
	got, err := NewUploadHistory("u1", "up1", "レシート.png", "image/png", now, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewUploadHistory() error = %v", err)
	}
	want := UploadHistory{
		UserID: "u1", UploadID: "up1", Status: StatusUploading, Attempt: 1,
		S3Key: "receipts/u1/up1/original.jpg", FileName: "レシート.png", ContentType: "image/png",
		// JST 30 日 23:30 は UTC では 30 日 14:30 なので 2026-09
		YearMonth: "2026-09",
		ExpiresAt: time.Date(2026, 9, 30, 14, 45, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 30, 14, 30, 0, 0, time.UTC),
	}
	if got != want {
		t.Errorf("NewUploadHistory() = %+v, want %+v", got, want)
	}
}

func TestNewUploadHistoryAppliesDefaults(t *testing.T) {
	t.Parallel()
	got, err := NewUploadHistory("u1", "up1", " ", "", now, time.Minute)
	if err != nil {
		t.Fatalf("NewUploadHistory() error = %v", err)
	}
	if got.FileName != DefaultFileName || got.ContentType != DefaultContentType {
		t.Errorf("defaults = %q / %q", got.FileName, got.ContentType)
	}
}

func TestNewUploadHistoryRejectsMissingIDs(t *testing.T) {
	t.Parallel()
	for name, ids := range map[string][2]string{"user": {"", "up1"}, "upload": {"u1", " "}} {
		if _, err := NewUploadHistory(ids[0], ids[1], "", "", now, time.Minute); !errors.Is(err, ErrInvalidUploadHistory) {
			t.Errorf("%s: error = %v, want ErrInvalidUploadHistory", name, err)
		}
	}
}
