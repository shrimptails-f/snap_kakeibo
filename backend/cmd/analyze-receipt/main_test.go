package main

import "testing"

func TestDecodeRetryJobKeepsAttempt(t *testing.T) {
	t.Parallel()
	jobs, err := decodeJobs(`{"user_id":"u1","upload_id":"up1","attempt":3,"trigger":"RETRY"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Attempt != 3 || jobs[0].Key != "receipts/u1/up1/original.jpg" {
		t.Fatalf("jobs=%+v", jobs)
	}
}

func TestDecodeS3JobUsesAttemptOne(t *testing.T) {
	t.Parallel()
	body := `{"Records":[{"s3":{"bucket":{"name":"bucket"},"object":{"key":"receipts%2Fu1%2Fup1%2Foriginal.jpg"}}}]}`
	jobs, err := decodeJobs(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Attempt != 1 || jobs[0].UserID != "u1" {
		t.Fatalf("jobs=%+v", jobs)
	}
}

func TestResponseIDsProduceDifferentRawKeys(t *testing.T) {
	t.Parallel()
	a, b := safeID("resp_A"), safeID("resp_B")
	if a == b {
		t.Fatalf("response IDs must remain unique: %q", a)
	}
}
