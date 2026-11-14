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
	ledgerdomain "snap_kakeibo/backend/internal/ledger/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

var (
	now     = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	testJob = domain.AnalysisJob{UserID: "u1", AnalysisRequestID: "req1", Attempt: 2, Trigger: domain.TriggerRetry, Bucket: "receipts", Key: "receipts/u1/req1/original.jpg"}
)

func ptr[T any](v T) *T { return &v }

func validReading() domain.ReceiptReading {
	return domain.ReceiptReading{
		StoreName:    ptr("店"),
		PurchaseDate: ptr("2026-09-18"),
		ReadAmount:   ptr(int64(150)),
		Details: []domain.ReadDetail{
			{Name: "牛乳", Amount: 100, Quantity: 1, Category: "food"},
			{Name: "会食", Amount: 50, Quantity: 2, Category: "social"},
		},
	}
}

// fixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type fixture struct {
	images   *images
	resizer  *resizer
	analyzer *analyzer
	raw      *rawResults
	requests *requests
	expenses *expenses
	buf      *bytes.Buffer
}

func newFixture() *fixture {
	return &fixture{
		images:   &images{data: []byte("image-bytes")},
		resizer:  &resizer{},
		analyzer: &analyzer{response: application.AnalyzerResponse{ResponseID: "resp_1", Raw: []byte(`{"id":"resp_1"}`), Usage: application.TokenUsage{InputTokens: 10, OutputTokens: 5, ReasoningTokens: 2}, Reading: validReading()}},
		raw:      &rawResults{},
		requests: &requests{started: true},
		expenses: &expenses{},
		buf:      &bytes.Buffer{},
	}
}

func (f *fixture) build() application.AnalyzeReceiptUsecaseInterface {
	log := logger.New(logger.Options{Level: "debug", Service: "test", Environment: "test", Writer: f.buf})
	return application.NewAnalyzeReceiptUsecase(f.images, f.resizer, f.analyzer, f.raw, f.requests, f.expenses, &ids{}, timewrapper.NewFixed(now), log)
}

func TestAnalyzeRegistersExpenseOnSuccess(t *testing.T) {
	t.Parallel()
	f := newFixture()
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if out.Outcome != domain.OutcomeSucceeded || out.ExpenseID != "id-1" || out.RawResultKey != "raw/resp_1" {
		t.Fatalf("output = %+v", out)
	}
	if f.requests.analyzing != 1 || f.requests.failed != nil || f.requests.noData {
		t.Errorf("requests = %+v", f.requests)
	}
	if f.raw.responseID != "resp_1" || string(f.raw.raw) != `{"id":"resp_1"}` {
		t.Errorf("raw result = %+v", f.raw)
	}
	e := f.expenses.registered
	if e.ID() != "id-1" || e.UserID() != "u1" || e.SourceRequestID() != "req1" || e.StoreName() != "店" || e.PurchaseDate().String() != "2026-09-18" || e.ReadAmount().Yen() != 150 || e.RecordedAmount().Yen() != 150 || e.AdjustmentAmount().Yen() != 0 || e.Edited() {
		t.Errorf("expense = %+v", e)
	}
	details := e.Details()
	if len(details) != 2 || details[0].ID() != "id-2" || details[1].ID() != "id-3" || details[1].Quantity().Int64() != 2 || details[1].Category() != "social" || details[0].CategorySource() != ledgerdomain.CategorySourceAI {
		t.Errorf("details = %+v", details)
	}
	if f.expenses.rawKey != "raw/resp_1" || !f.expenses.now.Equal(now) {
		t.Errorf("register args: rawKey=%q now=%v", f.expenses.rawKey, f.expenses.now)
	}
	span := analysisSpan(t, f.buf)
	assertField(t, span, "analysis_status", "SUCCEEDED")
	assertField(t, span, "status", logger.StatusOK)
	assertField(t, span, "analysis_request_id", "req1")
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
	assertField(t, span, "expense_id", "id-1")
	if _, ok := span["error_code"]; ok {
		t.Errorf("error_code must not be set on success: %v", span)
	}
}

func TestAnalyzePersistsSelectedTotalEvidence(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.analyzer.response.Reading.AmountCandidates = []domain.AmountCandidate{{Amount: 8, Label: "8%税額", Role: "tax", Position: 5}, {Amount: 108, Label: "合計", Role: "final_total", Position: 9}}
	f.analyzer.response.Reading.TaxBreakdown = []domain.TaxBreakdown{{Rate: ptr(int64(8)), TaxableAmount: ptr(int64(100)), TaxAmount: ptr(int64(8)), Mode: "external"}}
	_, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil {
		t.Fatalf("Analyze() error=%v", err)
	}
	if f.expenses.registered.ReadAmount().Yen() != 108 {
		t.Fatalf("read amount=%d", f.expenses.registered.ReadAmount().Yen())
	}
	var evidence domain.AmountEvidence
	if err := json.Unmarshal([]byte(f.expenses.registered.AnalysisEvidence()), &evidence); err != nil {
		t.Fatalf("evidence JSON: %v", err)
	}
	if evidence.Selected == nil || evidence.Selected.Amount != 108 || len(evidence.Taxes) != 1 || evidence.Status != "strong" {
		t.Fatalf("evidence=%+v", evidence)
	}
}

func TestAnalyzeStoresConfirmedAndUnknownDetailAmounts(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.analyzer.response.Reading = domain.ReceiptReading{
		PurchaseDate:     ptr("2026-09-18"),
		AmountCandidates: []domain.AmountCandidate{{Amount: 2206, Label: "お支払合計", Role: "final_total", Position: 9}},
		TaxBreakdown:     []domain.TaxBreakdown{{Rate: ptr(int64(10)), TaxableAmount: ptr(int64(2006)), TaxAmount: ptr(int64(200)), Mode: "external"}},
		Details:          []domain.ReadDetail{{Name: "商品", Amount: 2006, Quantity: 1, Category: "food", TaxRate: ptr(int64(10)), TaxMode: "external"}},
	}
	if _, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob}); err != nil {
		t.Fatal(err)
	}
	detail := f.expenses.registered.Details()[0]
	if detail.Amount().Yen() != 2006 || detail.TaxIncludedAmount() == nil || *detail.TaxIncludedAmount() != 2206 || detail.TaxAllocation() != "receipt_tax_proportional_v1" {
		t.Errorf("detail = %+v", detail)
	}
	f = newFixture()
	f.analyzer.response.Reading.Details[0].TaxMode = "unknown"
	f.analyzer.response.Reading.TaxBreakdown = []domain.TaxBreakdown{{Rate: ptr(int64(8)), TaxableAmount: ptr(int64(100)), TaxAmount: ptr(int64(8)), Mode: "external"}}
	if _, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob}); err != nil {
		t.Fatal(err)
	}
	if f.expenses.registered.Details()[0].TaxIncludedAmount() != nil {
		t.Error("unknown detail was converted")
	}
}

func TestAnalyzeSkipsWhenRequestIsNotAnalyzable(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.requests.started = false
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil || out.Outcome != domain.OutcomeSkipped {
		t.Fatalf("output = %+v err = %v", out, err)
	}
	if f.images.calls != 0 || f.analyzer.calls != 0 || f.expenses.calls != 0 {
		t.Errorf("skipped job must not touch S3 / OpenAI / expense: images=%d analyzer=%d expenses=%d", f.images.calls, f.analyzer.calls, f.expenses.calls)
	}
	span := analysisSpan(t, f.buf)
	assertField(t, span, "analysis_status", "SKIPPED")
	assertField(t, span, "status", logger.StatusOK)
}

func TestAnalyzeMarksNoDataWhenReceiptHasNoDetails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	reading := validReading()
	reading.Details = nil
	f.analyzer.response.Reading = reading
	out, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
	if err != nil || out.Outcome != domain.OutcomeNoData || out.RawResultKey != "raw/resp_1" {
		t.Fatalf("output = %+v err = %v", out, err)
	}
	if !f.requests.noData || f.requests.noDataRawKey != "raw/resp_1" || f.expenses.calls != 0 {
		t.Errorf("requests = %+v expenses = %d", f.requests, f.expenses.calls)
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
				f.analyzer.response = application.AnalyzerResponse{ResponseID: "resp_x", Raw: []byte(`{"status":"incomplete"}`), Failure: failurePtr(domain.AnalysisFailed("未完了"))}
			},
			code:   domain.FailureAnalysisFailed,
			rawKey: "raw/resp_x",
			event:  application.EventOpenAIResponseRejected,
		},
		{
			name: "openai 4xx without raw body",
			arrange: func(f *fixture) {
				f.analyzer.response = application.AnalyzerResponse{Failure: failurePtr(domain.AnalysisFailed("HTTP 400"))}
			},
			code:   domain.FailureAnalysisFailed,
			rawKey: "",
			event:  application.EventOpenAIResponseRejected,
		},
		{
			name: "validation failed without date",
			arrange: func(f *fixture) {
				reading := validReading()
				reading.PurchaseDate = nil
				f.analyzer.response.Reading = reading
			},
			code:   domain.FailureNoDate,
			rawKey: "raw/resp_1",
			event:  application.EventAnalysisValidationFailed,
		},
		{
			name: "validation failed with invalid amount",
			arrange: func(f *fixture) {
				reading := validReading()
				reading.ReadAmount = ptr(int64(0))
				f.analyzer.response.Reading = reading
			},
			code:   domain.FailureInvalidAmount,
			rawKey: "raw/resp_1",
			event:  application.EventAnalysisValidationFailed,
		},
		{
			name:       "expense rejected by the store",
			arrange:    func(f *fixture) { f.expenses.err = application.ErrExpenseRejected },
			code:       domain.FailureInternal,
			rawKey:     "raw/resp_1",
			event:      application.EventExpenseRegistrationRejected,
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
			if out.Outcome != domain.OutcomeFailed || out.ErrorCode != tt.code || out.RawResultKey != tt.rawKey {
				t.Fatalf("output = %+v", out)
			}
			if f.requests.failed == nil || f.requests.failed.Code() != tt.code || f.requests.failedRawKey != tt.rawKey {
				t.Errorf("MarkFailed = %+v rawKey=%q", f.requests.failed, f.requests.failedRawKey)
			}
			if !tt.registered && f.expenses.calls != 0 {
				t.Errorf("expense must not be registered on failure")
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
		{name: "mark analyzing failed", arrange: func(f *fixture) { f.requests.analyzingErr = boom }},
		{name: "image read failed", arrange: func(f *fixture) { f.images.err = boom }},
		{name: "openai temporary failure", arrange: func(f *fixture) { f.analyzer.err = boom }},
		{name: "raw result save failed", arrange: func(f *fixture) { f.raw.err = boom }},
		{name: "expense register failed", arrange: func(f *fixture) { f.expenses.err = boom }},
		{name: "mark failed failed", arrange: func(f *fixture) { f.resizer.err = errors.New("bad"); f.requests.failedErr = boom }, markingFailed: true},
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
	f.analyzer.response.ResponseID = ""
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
	f.analyzer.response.Raw = nil
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
	response application.AnalyzerResponse
	err      error
	calls    int
}

func (a *analyzer) Analyze(_ context.Context, jpeg []byte) (application.AnalyzerResponse, error) {
	a.calls++
	if !bytes.HasPrefix(jpeg, []byte("jpeg:")) {
		return application.AnalyzerResponse{}, errors.New("analyzer received an image that was not resized")
	}
	return a.response, a.err
}

func failurePtr(reason domain.FailureReason) *domain.FailureReason { return &reason }

type rawResults struct {
	responseID string
	raw        []byte
	err        error
	calls      int
}

func (s *rawResults) SaveRawResult(_ context.Context, _ domain.AnalysisJob, responseID string, raw []byte) (string, error) {
	s.calls++
	s.responseID, s.raw = responseID, raw
	if s.err != nil {
		return "", s.err
	}
	return "raw/" + responseID, nil
}

type requests struct {
	started      bool
	analyzingErr error
	analyzing    int
	failed       *domain.FailureReason
	failedRawKey string
	failedErr    error
	noData       bool
	noDataRawKey string
}

func (h *requests) MarkAnalyzing(_ context.Context, _ domain.AnalysisJob, _ time.Time) (bool, error) {
	h.analyzing++
	return h.started, h.analyzingErr
}

func (h *requests) MarkFailed(_ context.Context, _ domain.AnalysisJob, reason domain.FailureReason, rawKey string, _ time.Time) error {
	h.failed, h.failedRawKey = &reason, rawKey
	return h.failedErr
}

func (h *requests) MarkNoData(_ context.Context, _ domain.AnalysisJob, rawKey string, _ time.Time) error {
	h.noData, h.noDataRawKey = true, rawKey
	return nil
}

type expenses struct {
	registered ledgerdomain.Expense
	rawKey     string
	now        time.Time
	err        error
	calls      int
}

func (e *expenses) Register(_ context.Context, _ domain.AnalysisJob, expense ledgerdomain.Expense, rawKey string, now time.Time) error {
	e.calls++
	e.registered, e.rawKey, e.now = expense, rawKey, now
	return e.err
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

func TestAnalyzePersistsReconciledAndEstimatedTaxes(t *testing.T) {
	t.Parallel()
	for _, estimated := range []bool{false, true} {
		t.Run(fmt.Sprintf("estimated=%t", estimated), func(t *testing.T) {
			t.Parallel()
			f := newFixture()
			r := validReading()
			second := int64(200)
			total := int64(328)
			tax := int64(20)
			if estimated {
				second, total, tax = 100, 218, 10
			}
			r.Details = []domain.ReadDetail{{Name: "パン", Amount: 100, Quantity: 1, Category: "food"}, {Name: "洗剤", Amount: second, Quantity: 1, Category: "daily_goods"}}
			r.AmountCandidates = []domain.AmountCandidate{{Amount: total, Label: "合計", Role: "final_total"}}
			r.TaxBreakdown = []domain.TaxBreakdown{{Rate: ptr(int64(8)), TaxableAmount: ptr(int64(100)), TaxAmount: ptr(int64(8)), Mode: "external"}, {Rate: ptr(int64(10)), TaxableAmount: &second, TaxAmount: &tax, Mode: "external"}}
			f.analyzer.response.Reading = r
			_, err := f.build().Analyze(context.Background(), application.AnalyzeReceiptInput{Job: testJob})
			if err != nil {
				t.Fatal(err)
			}
			e := f.expenses.registered
			var evidence domain.AmountEvidence
			if err := json.Unmarshal([]byte(e.AnalysisEvidence()), &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.Inference == nil {
				t.Fatal("missing persisted inference")
			}
			for i, d := range e.Details() {
				wantRate := int64(8 + i*2)
				if estimated {
					if d.TaxStatus() != "estimated" || d.TaxRate() != nil || d.TaxIncludedAmount() != nil || *d.SuggestedTaxRate() != wantRate || d.ReportingAmount() != d.Amount().Yen() {
						t.Fatalf("estimated detail=%+v", d)
					}
				} else if d.TaxStatus() != "reconciled" || *d.TaxRate() != wantRate || d.TaxIncludedAmount() == nil {
					t.Fatalf("reconciled detail=%+v", d)
				}
			}
		})
	}
}
