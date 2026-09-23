package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToEventMatchesAPIGatewayV2Shape(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/api/months/2026-09/analysis-requests?limit=10&limit=20", strings.NewReader(""))
	r.Header.Set("Authorization", "Bearer token")
	r.Header.Set("Cookie", "refresh_token=abc; other=1")
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	r.RemoteAddr = "127.0.0.1:5173"

	rt := route{method: "GET", path: "/api/months/{month}/analysis-requests", function: "list-analysis-requests"}
	event := toEvent(r, rt, map[string]string{"month": "2026-09"}, []byte("body"))

	if event.Version != "2.0" || event.RouteKey != "GET /api/months/{month}/analysis-requests" || event.RawPath != "/api/months/2026-09/analysis-requests" {
		t.Errorf("routing fields = %q %q %q", event.Version, event.RouteKey, event.RawPath)
	}
	if event.Headers["authorization"] != "Bearer token" {
		t.Errorf("headers = %v, want lowercased authorization", event.Headers)
	}
	if _, ok := event.Headers["cookie"]; ok {
		t.Errorf("cookie header should be moved to cookies: %v", event.Headers)
	}
	if len(event.Cookies) != 2 || event.Cookies[0] != "refresh_token=abc" {
		t.Errorf("cookies = %v", event.Cookies)
	}
	if event.QueryStringParameters["limit"] != "10,20" {
		t.Errorf("query = %v, want comma-joined multi values", event.QueryStringParameters)
	}
	if event.PathParameters["month"] != "2026-09" || event.RequestContext.HTTP.SourceIP != "203.0.113.5" || event.Body != "body" {
		t.Errorf("path/source/body = %v %q %q", event.PathParameters, event.RequestContext.HTTP.SourceIP, event.Body)
	}
}

func TestRewritePostURLReplacesEndpointOnly(t *testing.T) {
	t.Parallel()
	tc := transformContext{endpoint: "http://floci:4566", publicS3URL: "http://localhost:8080/s3"}
	in := `{"analysis_request_id":"id","post_url":"http://floci:4566/bucket","post_fields":{"policy":"signed"},"s3_key":"key"}`

	out, err := rewritePostURL(tc, []byte(in))
	if err != nil {
		t.Fatalf("rewritePostURL() error = %v", err)
	}
	var res map[string]json.RawMessage
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if string(res["post_url"]) != `"http://localhost:8080/s3/bucket"` || string(res["analysis_request_id"]) != `"id"` || string(res["post_fields"]) != `{"policy":"signed"}` {
		t.Errorf("rewritten = %v", res)
	}
}

func TestRewritePostURLRejectsUnexpectedOrigin(t *testing.T) {
	t.Parallel()
	tc := transformContext{endpoint: "http://floci:4566", publicS3URL: "http://localhost:8080/s3"}
	if _, err := rewritePostURL(tc, []byte(`{"post_url":"https://s3.amazonaws.com/bucket"}`)); err == nil {
		t.Error("expected an error for a post_url outside the local endpoint")
	}
}

func TestRewriteExpenseImageURLReplacesNestedURL(t *testing.T) {
	t.Parallel()
	tc := transformContext{endpoint: "http://floci:4566", publicS3URL: "http://localhost:8080/s3"}
	in := `{"expense":{"expense_id":"e1","image_url":"http://floci:4566/bucket/receipts/u1/r1/original.jpg?X-Amz-Signature=sig"},"details":[]}`

	out, err := rewriteExpenseImageURL(tc, []byte(in))
	if err != nil {
		t.Fatalf("rewriteExpenseImageURL() error = %v", err)
	}
	var res struct {
		Expense map[string]string `json:"expense"`
		Details []any             `json:"details"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if res.Expense["image_url"] != "http://localhost:8080/s3/bucket/receipts/u1/r1/original.jpg?X-Amz-Signature=sig" || res.Expense["expense_id"] != "e1" || len(res.Details) != 0 {
		t.Errorf("rewritten = %+v", res)
	}
}

func TestRewriteExpenseImageURLRejectsUnexpectedOrigin(t *testing.T) {
	t.Parallel()
	tc := transformContext{endpoint: "http://floci:4566", publicS3URL: "http://localhost:8080/s3"}
	if _, err := rewriteExpenseImageURL(tc, []byte(`{"expense":{"image_url":"https://s3.amazonaws.com/bucket/key"},"details":[]}`)); err == nil {
		t.Error("expected an error for an image_url outside the local endpoint")
	}
}
