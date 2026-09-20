package queue

import (
	"testing"

	"snap_kakeibo/backend/internal/analysis/domain"
)

func TestDecodeRetryJobKeepsAttempt(t *testing.T) {
	t.Parallel()
	jobs, err := DecodeJobs(`{"user_id":"u1","upload_id":"up1","attempt":3,"trigger":"RETRY"}`, "receipt-bucket")
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Job{UserID: "u1", UploadID: "up1", Attempt: 3, Trigger: domain.TriggerRetry, Bucket: "receipt-bucket", Key: "receipts/u1/up1/original.jpg"}
	if len(jobs) != 1 || jobs[0] != want {
		t.Fatalf("jobs=%+v want %+v", jobs, want)
	}
}

func TestDecodeS3JobUsesAttemptOne(t *testing.T) {
	t.Parallel()
	body := `{"Records":[{"s3":{"bucket":{"name":"bucket"},"object":{"key":"receipts%2Fu1%2Fup1%2Foriginal.jpg"}}}]}`
	jobs, err := DecodeJobs(body, "receipt-bucket")
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Job{UserID: "u1", UploadID: "up1", Attempt: 1, Trigger: domain.TriggerS3, Bucket: "bucket", Key: "receipts/u1/up1/original.jpg"}
	if len(jobs) != 1 || jobs[0] != want {
		t.Fatalf("jobs=%+v want %+v", jobs, want)
	}
}

func TestDecodeS3JobIgnoresOtherKeys(t *testing.T) {
	t.Parallel()
	body := `{"Records":[{"s3":{"bucket":{"name":"bucket"},"object":{"key":"analysis-results/u1/up1/1/resp.json"}}}]}`
	jobs, err := DecodeJobs(body, "receipt-bucket")
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
}

func TestDecodeRejectsInvalidBody(t *testing.T) {
	t.Parallel()
	if _, err := DecodeJobs(`not json`, "receipt-bucket"); err == nil {
		t.Fatal("DecodeJobs() accepted an invalid body")
	}
}

func TestIDsFromKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key string
		ok  bool
	}{
		{"receipts/u1/up1/original.jpg", true},
		{"receipts/u1/up1/", false},
		{"receipts/u1/original.jpg", false},
		{"uploads/u1/up1/original.jpg", false},
		{"receipts//up1/original.jpg", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			userID, uploadID, ok := IDsFromKey(tt.key)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && (userID != "u1" || uploadID != "up1") {
				t.Errorf("ids = %q %q", userID, uploadID)
			}
		})
	}
}
