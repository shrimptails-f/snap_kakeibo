package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/openai"
)

type fakeResponses struct {
	req  openai.Request
	resp *openai.Response
	err  error
	// deadline は受け取った ctx の期限。タイムアウトが設定されていることの確認用
	deadline time.Time
}

func (f *fakeResponses) Responses(ctx context.Context, req openai.Request) (*openai.Response, error) {
	f.req = req
	f.deadline, _ = ctx.Deadline()
	return f.resp, f.err
}

func completedResponse(text string) *openai.Response {
	return &openai.Response{ID: "resp_1", Status: "completed", OutputText: text, Raw: []byte(`{"id":"resp_1"}`), Usage: openai.Usage{InputTokens: 10, OutputTokens: 5, ReasoningTokens: 2}}
}

func TestAnalyzeSendsImageWithSchema(t *testing.T) {
	t.Parallel()
	client := &fakeResponses{resp: completedResponse(`{"store_name":null,"purchased_at":"2026-09-18","total_amount":100,"details":[]}`)}
	analyzer := OpenAIReceiptAnalyzer{Client: client, Model: "gpt-5-mini", ReasoningEffort: "low"}

	before := time.Now()
	result, err := analyzer.Analyze(context.Background(), []byte("jpeg-bytes"))
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Failure != nil || result.ResponseID != "resp_1" || string(result.Raw) != `{"id":"resp_1"}` {
		t.Fatalf("result = %+v", result)
	}
	if result.Receipt.PurchasedAt == nil || *result.Receipt.PurchasedAt != "2026-09-18" || *result.Receipt.TotalAmount != 100 {
		t.Errorf("receipt = %+v", result.Receipt)
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 || result.Usage.ReasoningTokens != 2 {
		t.Errorf("usage = %+v", result.Usage)
	}
	req := client.req
	if req.Model != "gpt-5-mini" || req.ReasoningEffort != "low" || req.MaxOutputTokens != maxOutputTokens || req.Instructions != Instructions {
		t.Errorf("request = %+v", req)
	}
	if req.Schema == nil || req.Schema.Name != "receipt" || !req.Schema.Strict {
		t.Errorf("schema = %+v", req.Schema)
	}
	if len(req.Input) != 1 || len(req.Input[0].Content) != 1 || req.Input[0].Content[0].Type != "input_image" || req.Input[0].Content[0].Detail != "high" || !strings.HasPrefix(req.Input[0].Content[0].ImageURL, "data:image/jpeg;base64,") {
		t.Errorf("input = %+v", req.Input)
	}
	if client.deadline.IsZero() || client.deadline.Sub(before) > DefaultRequestTimeout+time.Second {
		t.Errorf("request deadline = %v, want within %v", client.deadline, DefaultRequestTimeout)
	}
}

func TestAnalyzeReturnsFailureForUnusableResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		resp    *openai.Response
		message string
	}{
		{"refusal", &openai.Response{ID: "r", Status: "completed", Refusal: "no", Raw: []byte(`{}`)}, "拒否"},
		{"incomplete", &openai.Response{ID: "r", Status: "incomplete", IncompleteReason: "max_output_tokens", Raw: []byte(`{}`)}, "max_output_tokens"},
		{"incomplete without reason", &openai.Response{ID: "r", Status: "incomplete", Raw: []byte(`{}`)}, "unknown"},
		{"unexpected status", &openai.Response{ID: "r", Status: "queued", Raw: []byte(`{}`)}, "状態が不正"},
		{"no output", &openai.Response{ID: "r", Status: "completed", Raw: []byte(`{}`)}, "解析結果がありません"},
		{"invalid json", completedResponse(`not json`), "JSON Schema"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			analyzer := OpenAIReceiptAnalyzer{Client: &fakeResponses{resp: tt.resp}}
			result, err := analyzer.Analyze(context.Background(), []byte("jpeg"))
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			if result.Failure == nil || result.Failure.Code != domain.FailureAnalysisFailed || !strings.Contains(result.Failure.Message, tt.message) {
				t.Fatalf("failure = %v, want %s containing %q", result.Failure, domain.FailureAnalysisFailed, tt.message)
			}
			if result.ResponseID != "r" && result.ResponseID != "resp_1" || len(result.Raw) == 0 {
				t.Errorf("response metadata must be kept for the raw result: %+v", result)
			}
		})
	}
}

func TestAnalyzeTreatsClientErrorsAsFailure(t *testing.T) {
	t.Parallel()
	analyzer := OpenAIReceiptAnalyzer{Client: &fakeResponses{err: &openai.APIError{StatusCode: 400, Type: "invalid_request_error", Code: "invalid_image", Message: "bad image"}}}
	result, err := analyzer.Analyze(context.Background(), []byte("jpeg"))
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Failure == nil || result.Failure.Code != domain.FailureAnalysisFailed || !strings.Contains(result.Failure.Message, "400") {
		t.Fatalf("failure = %v", result.Failure)
	}
	if result.ResponseID != "" || result.Raw != nil {
		t.Errorf("4xx has no response to persist: %+v", result)
	}
}

func TestAnalyzeReturnsTemporaryErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{"server error", &openai.APIError{StatusCode: 503}},
		{"rate limited", &openai.APIError{StatusCode: 429}},
		{"response failed", &openai.ResponseFailedError{ID: "r"}},
		{"transport", &openai.TransportError{Err: errors.New("reset")}},
		{"deadline", context.DeadlineExceeded},
		{"canceled", context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			analyzer := OpenAIReceiptAnalyzer{Client: &fakeResponses{err: tt.err}}
			if _, err := analyzer.Analyze(context.Background(), []byte("jpeg")); !errors.Is(err, tt.err) {
				t.Fatalf("Analyze() error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestReceiptSchemaRequiresDateOnly(t *testing.T) {
	t.Parallel()
	properties := ReceiptSchema()["properties"].(map[string]any)
	purchasedAt := properties["purchased_at"].(map[string]any)
	variants := purchasedAt["anyOf"].([]any)
	date := variants[0].(map[string]any)
	if got := date["pattern"]; got != `^\d{4}-\d{2}-\d{2}$` {
		t.Fatalf("purchased_at pattern = %v", got)
	}
	details := properties["details"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if got := details["category"].(map[string]any)["enum"]; len(got.([]string)) != len(domain.Categories) {
		t.Fatalf("category enum = %v", got)
	}
}
