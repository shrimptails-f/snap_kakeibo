package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

var (
	now     = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	testJob = domain.Job{UserID: "u1", UploadID: "up1", Attempt: 2, Trigger: domain.TriggerRetry, Bucket: "receipts", Key: "receipts/u1/up1/original.jpg"}
)

func ptr[T any](v T) *T { return &v }

func validReceipt() domain.Receipt {
	return domain.Receipt{
		StoreName:   ptr("店"),
		PurchasedAt: ptr("2026-09-18"),
		TotalAmount: ptr(int64(150)),
		Details: []domain.Detail{
			{Name: "牛乳", Amount: 100, Quantity: 1, Category: "food"},
			{Name: "パン", Amount: 50, Quantity: 2, Category: "food"},
		},
	}
}

// fixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type fixture struct {
	images    *images
	resizer   *resizer
	analyzer  *analyzer
	raw       *rawResults
	histories *histories
	billings  *billings
	buf       *bytes.Buffer
}

func newFixture() *fixture {
	return &fixture{
		images:    &images{data: []byte("image-bytes")},
		resizer:   &resizer{},
		analyzer:  &analyzer{result: application.AnalysisResult{ResponseID: "resp_1", Raw: []byte(`{"id":"resp_1"}`), Usage: application.TokenUsage{InputTokens: 10, OutputTokens: 5, ReasoningTokens: 2}, Receipt: validReceipt()}},
		raw:       &rawResults{},
		histories: &histories{started: true},
		billings:  &billings{},
		buf:       &bytes.Buffer{},
	}
}

func (f *fixture) build() application.AnalyzeReceiptUsecaseInterface {
	log := logger.New(logger.Options{Level: "debug", Service: "test", Environment: "test", Writer: f.buf})
	return application.NewAnalyzeReceiptUsecase(f.images, f.resizer, f.analyzer, f.raw, f.histories, f.billings, &ids{}, timewrapper.NewFixed(now), log)
}

func TestAnalyzeRegistersBillingOnSuccess(t *testing.T) {
	t.Parallel()
	f := newFixture()
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if out.Status != domain.StatusSucceeded || out.BillingID != "id-1" || out.RawResultKey != "raw/resp_1" {
		t.Fatalf("output = %+v", out)
	}
	if f.histories.analyzing != 1 || f.histories.failed != nil || f.histories.noData {
		t.Errorf("histories = %+v", f.histories)
	}
	if f.raw.responseID != "resp_1" || string(f.raw.raw) != `{"id":"resp_1"}` {
		t.Errorf("raw result = %+v", f.raw)
	}
	b := f.billings.registered
	if b.ID != "id-1" || b.UserID != "u1" || b.UploadID != "up1" || b.StoreName != "店" || b.PurchasedAt != "2026-09-18" || b.TotalAmount != 150 || !b.CreatedAt.Equal(now) {
		t.Errorf("billing = %+v", b)
	}
	if len(b.Details) != 2 || b.Details[0].ID != "id-2" || b.Details[1].ID != "id-3" || b.Details[1].Quantity != 2 {
		t.Errorf("details = %+v", b.Details)
	}
	if f.billings.rawKey != "raw/resp_1" || !f.billings.now.Equal(now) {
		t.Errorf("register args: rawKey=%q now=%v", f.billings.rawKey, f.billings.now)
	}
	span := analysisSpan(t, f.buf)
	assertField(t, span, "analysis_status", "SUCCEEDED")
	assertField(t, span, "status", logger.StatusOK)
	assertField(t, span, "upload_id", "up1")
	assertField(t, span, "user_id", "u1")
	assertField(t, span, "attempt", float64(2))
	assertField(t, span, "trigger", "RETRY")
	assertField(t, span, "s3_key", testJob.Key)
	assertField(t, span, "image_bytes", float64(len("image-bytes")))
	assertField(t, span, "response_id", "resp_1")
	assertField(t, span, "raw_result_s3_key", "raw/resp_1")
	assertField(t, span, "input_tokens", float64(10))
	assertField(t, span, "output_tokens", float64(5))
	assertField(t, span, "reasoning_tokens", float64(2))
	assertField(t, span, "detail_count", float64(2))
	assertField(t, span, "billing_id", "id-1")
	if _, ok := span["error_code"]; ok {
		t.Errorf("error_code must not be set on success: %v", span)
	}
}

func TestAnalyzeSkipsWhenUploadIsNotAnalyzable(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.histories.started = false
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil || out.Status != domain.StatusSkipped {
		t.Fatalf("output = %+v err = %v", out, err)
	}
	if f.images.calls != 0 || f.analyzer.calls != 0 || f.billings.calls != 0 {
		t.Errorf("skipped job must not touch S3 / OpenAI / billing: images=%d analyzer=%d billings=%d", f.images.calls, f.analyzer.calls, f.billings.calls)
	}
	span := analysisSpan(t, f.buf)
	assertField(t, span, "analysis_status", "SKIPPED")
	assertField(t, span, "status", logger.StatusOK)
}

func TestAnalyzeMarksNoDataWhenReceiptHasNoDetails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	receipt := validReceipt()
	receipt.Details = nil
	f.analyzer.result.Receipt = receipt
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil || out.Status != domain.StatusNoData || out.RawResultKey != "raw/resp_1" {
		t.Fatalf("output = %+v err = %v", out, err)
	}
	if !f.histories.noData || f.histories.noDataRawKey != "raw/resp_1" || f.billings.calls != 0 {
		t.Errorf("histories = %+v billings = %d", f.histories, f.billings.calls)
	}
	span := analysisSpan(t, f.buf)
	assertField(t, span, "analysis_status", "NO_DATA")
	assertField(t, span, "detail_count", float64(0))
}

func TestAnalyzeMarksFailed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		arrange    func(f *fixture)
		code       string
		rawKey     string
		event      string
		registered bool
	}{
		{
			name:    "undecodable image",
			arrange: func(f *fixture) { f.resizer.err = errors.New("decode image: bad") },
			code:    domain.FailureInternal,
			rawKey:  "",
			event:   application.EventImageDecodeFailed,
		},
		{
			name: "openai rejected the response with raw body",
			arrange: func(f *fixture) {
				f.analyzer.result = application.AnalysisResult{ResponseID: "resp_x", Raw: []byte(`{"status":"incomplete"}`), Failure: &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "未完了"}}
			},
			code:   domain.FailureAnalysisFailed,
			rawKey: "raw/resp_x",
			event:  application.EventOpenAIResponseRejected,
		},
		{
			name: "openai 4xx without raw body",
			arrange: func(f *fixture) {
				f.analyzer.result = application.AnalysisResult{Failure: &domain.Failure{Code: domain.FailureAnalysisFailed, Message: "HTTP 400"}}
			},
			code:   domain.FailureAnalysisFailed,
			rawKey: "",
			event:  application.EventOpenAIResponseRejected,
		},
		{
			name: "validation failed without date",
			arrange: func(f *fixture) {
				receipt := validReceipt()
				receipt.PurchasedAt = nil
				f.analyzer.result.Receipt = receipt
			},
			code:   domain.FailureNoDate,
			rawKey: "raw/resp_1",
			event:  application.EventAnalysisValidationFailed,
		},
		{
			name: "validation failed with invalid amount",
			arrange: func(f *fixture) {
				receipt := validReceipt()
				receipt.TotalAmount = ptr(int64(0))
				f.analyzer.result.Receipt = receipt
			},
			code:   domain.FailureInvalidAmount,
			rawKey: "raw/resp_1",
			event:  application.EventAnalysisValidationFailed,
		},
		{
			name:       "billing rejected by the store",
			arrange:    func(f *fixture) { f.billings.err = application.ErrBillingRejected },
			code:       domain.FailureInternal,
			rawKey:     "raw/resp_1",
			event:      application.EventBillingRegistrationRejected,
			registered: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			tt.arrange(f)
			out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			if out.Status != domain.StatusFailed || out.ErrorCode != tt.code || out.RawResultKey != tt.rawKey {
				t.Fatalf("output = %+v", out)
			}
			if f.histories.failed == nil || f.histories.failed.Code != tt.code || f.histories.failedRawKey != tt.rawKey {
				t.Errorf("MarkFailed = %+v rawKey=%q", f.histories.failed, f.histories.failedRawKey)
			}
			if !tt.registered && f.billings.calls != 0 {
				t.Errorf("billing must not be registered on failure")
			}
			span := analysisSpan(t, f.buf)
			assertField(t, span, "analysis_status", "FAILED")
			assertField(t, span, "error_code", tt.code)
			assertField(t, span, "status", logger.StatusOK)
			if !strings.Contains(f.buf.String(), `"event":"`+tt.event+`"`) {
				t.Errorf("event %s not logged: %s", tt.event, f.buf.String())
			}
		})
	}
}

// TestAnalyzeReturnsTemporaryErrors は一時的な失敗を FAILED として記録せず error で返し、キューの再配信に任せることを確認する。
func TestAnalyzeReturnsTemporaryErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tests := []struct {
		name    string
		arrange func(f *fixture)
		// markingFailed は業務上の失敗を記録する途中で落ちたケース。analysis_status は FAILED のまま status=error になる
		markingFailed bool
	}{
		{name: "mark analyzing failed", arrange: func(f *fixture) { f.histories.analyzingErr = boom }},
		{name: "image read failed", arrange: func(f *fixture) { f.images.err = boom }},
		{name: "openai temporary failure", arrange: func(f *fixture) { f.analyzer.err = boom }},
		{name: "raw result save failed", arrange: func(f *fixture) { f.raw.err = boom }},
		{name: "billing register failed", arrange: func(f *fixture) { f.billings.err = boom }},
		{name: "mark failed failed", arrange: func(f *fixture) { f.resizer.err = errors.New("bad"); f.histories.failedErr = boom }, markingFailed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			tt.arrange(f)
			_, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
			if !errors.Is(err, boom) {
				t.Fatalf("Analyze() error = %v, want boom", err)
			}
			span := analysisSpan(t, f.buf)
			assertField(t, span, "status", logger.StatusError)
			if got := span["analysis_status"]; !tt.markingFailed && got != nil {
				t.Errorf("analysis_status = %v on a temporary error", got)
			}
		})
	}
}

func TestAnalyzeGeneratesResponseIDWhenMissing(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.analyzer.result.ResponseID = ""
	if _, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob}); err != nil {
		t.Fatal(err)
	}
	if f.raw.responseID != "id-1" {
		t.Errorf("response id = %q, want generated id-1", f.raw.responseID)
	}
	assertField(t, analysisSpan(t, f.buf), "response_id", "id-1")
}

func TestAnalyzeSkipsRawResultWithoutBody(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.analyzer.result.Raw = nil
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil || out.RawResultKey != "" || f.raw.calls != 0 {
		t.Fatalf("output = %+v err = %v raw calls = %d", out, err, f.raw.calls)
	}
	assertField(t, analysisSpan(t, f.buf), "raw_result_s3_key", "")
}

type images struct {
	data  []byte
	err   error
	calls int
}

func (s *images) ReadImage(_ context.Context, bucket, key string) ([]byte, error) {
	s.calls++
	if bucket != testJob.Bucket || key != testJob.Key {
		return nil, fmt.Errorf("unexpected object %s/%s", bucket, key)
	}
	return s.data, s.err
}

type resizer struct{ err error }

func (r *resizer) ResizeJPEG(data []byte) ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte("jpeg:"), data...), nil
}

type analyzer struct {
	result application.AnalysisResult
	err    error
	calls  int
}

func (a *analyzer) Analyze(_ context.Context, jpeg []byte) (application.AnalysisResult, error) {
	a.calls++
	if !bytes.HasPrefix(jpeg, []byte("jpeg:")) {
		return application.AnalysisResult{}, errors.New("analyzer received an image that was not resized")
	}
	return a.result, a.err
}

type rawResults struct {
	responseID string
	raw        []byte
	err        error
	calls      int
}

func (s *rawResults) SaveRawResult(_ context.Context, _ domain.Job, responseID string, raw []byte) (string, error) {
	s.calls++
	s.responseID, s.raw = responseID, raw
	if s.err != nil {
		return "", s.err
	}
	return "raw/" + responseID, nil
}

type histories struct {
	started      bool
	analyzingErr error
	analyzing    int
	failed       *domain.Failure
	failedRawKey string
	failedErr    error
	noData       bool
	noDataRawKey string
}

func (h *histories) MarkAnalyzing(_ context.Context, _ domain.Job, _ time.Time) (bool, error) {
	h.analyzing++
	return h.started, h.analyzingErr
}

func (h *histories) MarkFailed(_ context.Context, _ domain.Job, failure domain.Failure, rawKey string, _ time.Time) error {
	h.failed, h.failedRawKey = &failure, rawKey
	return h.failedErr
}

func (h *histories) MarkNoData(_ context.Context, _ domain.Job, rawKey string, _ time.Time) error {
	h.noData, h.noDataRawKey = true, rawKey
	return nil
}

type billings struct {
	registered domain.Billing
	rawKey     string
	now        time.Time
	err        error
	calls      int
}

func (b *billings) Register(_ context.Context, _ domain.Job, billing domain.Billing, rawKey string, now time.Time) error {
	b.calls++
	b.registered, b.rawKey, b.now = billing, rawKey, now
	return b.err
}

// ids は id-1, id-2, ... を順に返す。
type ids struct{ n int }

func (g *ids) NewID() (string, error) {
	g.n++
	return fmt.Sprintf("id-%d", g.n), nil
}

func analysisSpan(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var found map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		if entry["event"] == logger.EventSpanFinished && entry["span_name"] == application.SpanAnalysis {
			if found != nil {
				t.Fatalf("analysis span finished twice: %s", buf.String())
			}
			found = entry
		}
	}
	if found == nil {
		t.Fatalf("analysis span_finished not found: %s", buf.String())
	}
	return found
}

func assertField(t *testing.T, entry map[string]any, key string, want any) {
	t.Helper()
	if got := entry[key]; got != want {
		t.Errorf("%s = %v, want %v", key, got, want)
	}
}
