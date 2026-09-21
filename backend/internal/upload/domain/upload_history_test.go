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

func TestValidateYearMonth(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in    string
		valid bool
	}{
		"valid":              {"2026-09", true},
		"december":           {"2026-12", true},
		"month out of range": {"2026-13", false},
		"month zero":         {"2026-00", false},
		"single digit month": {"2026-9", false},
		"with day":           {"2026-09-01", false},
		"slash":              {"2026/09", false},
		"empty":              {"", false},
		"whitespace":         {" 2026-09", false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := ValidateYearMonth(tt.in)
			if tt.valid && err != nil {
				t.Fatalf("ValidateYearMonth(%q) error = %v, want nil", tt.in, err)
			}
			if !tt.valid && !errors.Is(err, ErrInvalidYearMonth) {
				t.Fatalf("ValidateYearMonth(%q) error = %v, want ErrInvalidYearMonth", tt.in, err)
			}
		})
	}
}

func TestYearMonthRoundTripsThroughValidate(t *testing.T) {
	t.Parallel()
	if err := ValidateYearMonth(YearMonth(now)); err != nil {
		t.Fatalf("ValidateYearMonth(YearMonth(now)) error = %v", err)
	}
}
