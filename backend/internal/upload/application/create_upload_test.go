package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

var now = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

type requests struct {
	saved []domain.AnalysisRequest
	err   error
}

func (r *requests) Save(_ context.Context, request domain.AnalysisRequest) error {
	if r.err != nil {
		return r.err
	}
	r.saved = append(r.saved, request)
	return nil
}

type presigner struct {
	key         string
	contentType string
	expires     time.Duration
	err         error
}

func (p *presigner) PresignPost(_ context.Context, key, contentType string, expires time.Duration) (application.UploadForm, error) {
	p.key, p.contentType, p.expires = key, contentType, expires
	if p.err != nil {
		return application.UploadForm{}, p.err
	}
	return application.UploadForm{URL: "https://example.com/" + key, Fields: map[string]string{"key": key}}, nil
}

type ids struct{ err error }

func (i ids) NewID() (string, error) { return "req1", i.err }

// fixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type fixture struct {
	requests *requests
	urls     *presigner
	ids      ids
}

func newFixture() *fixture { return &fixture{requests: &requests{}, urls: &presigner{}} }

func (f *fixture) build() application.CreateUploadUsecaseInterface {
	return application.NewCreateUploadUsecase(f.requests, f.urls, f.ids, timewrapper.NewFixed(now))
}

func TestCreateRegistersRequestThenPresigns(t *testing.T) {
	t.Parallel()
	f := newFixture()
	out, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1", FileName: "a.png", ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := application.CreateUploadOutput{AnalysisRequestID: "req1", S3Key: "receipts/u1/req1/original.jpg", PostForm: application.UploadForm{URL: "https://example.com/receipts/u1/req1/original.jpg", Fields: map[string]string{"key": "receipts/u1/req1/original.jpg"}}, ExpiresAt: now.Add(15 * time.Minute)}
	if out.AnalysisRequestID != want.AnalysisRequestID || out.S3Key != want.S3Key || out.PostForm.URL != want.PostForm.URL || out.PostForm.Fields["key"] != want.PostForm.Fields["key"] || !out.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("Create() = %+v, want %+v", out, want)
	}
	if len(f.requests.saved) != 1 {
		t.Fatalf("saved = %+v", f.requests.saved)
	}
	saved := f.requests.saved[0]
	if saved.UserID() != "u1" || saved.ID() != "req1" || saved.Status() != analysisdomain.AnalysisStatusUploading || saved.CurrentAttempt().Int() != 1 || saved.Image().FileName() != "a.png" || saved.Image().ContentType() != "image/png" || !saved.UploadExpiresAt().Equal(want.ExpiresAt) {
		t.Errorf("saved request = %+v", saved)
	}
	if f.urls.key != want.S3Key || f.urls.contentType != "image/png" || f.urls.expires != 15*time.Minute {
		t.Errorf("presign args = %+v", f.urls)
	}
}

func TestCreateAppliesDefaultsToOmittedFields(t *testing.T) {
	t.Parallel()
	f := newFixture()
	if _, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	saved := f.requests.saved[0]
	if saved.Image().FileName() != domain.DefaultFileName || saved.Image().ContentType() != domain.DefaultContentType || f.urls.contentType != domain.DefaultContentType {
		t.Errorf("defaults: request = %+v, presign content_type = %q", saved, f.urls.contentType)
	}
}

func TestCreateRejectsMissingUser(t *testing.T) {
	t.Parallel()
	f := newFixture()
	_, err := f.build().Create(context.Background(), application.CreateUploadInput{})
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
	if len(f.requests.saved) != 0 || f.urls.key != "" {
		t.Errorf("no side effects expected: %+v %+v", f.requests, f.urls)
	}
}

func TestCreateDoesNotPresignWhenSaveFails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.requests.err = application.ErrAnalysisRequestAlreadyExists
	_, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1"})
	if !errors.Is(err, application.ErrAnalysisRequestAlreadyExists) {
		t.Fatalf("Create() error = %v, want ErrAnalysisRequestAlreadyExists", err)
	}
	if f.urls.key != "" {
		t.Errorf("PresignPost should not be called, got key %q", f.urls.key)
	}
}

func TestCreateFailsWhenIDGenerationFails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.ids = ids{err: errors.New("boom")}
	if _, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1"}); err == nil || len(f.requests.saved) != 0 {
		t.Fatalf("Create() error = %v, saved = %+v", err, f.requests.saved)
	}
}

func TestCreateWrapsPresignFailure(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.urls.err = errors.New("boom")
	if _, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1"}); err == nil {
		t.Fatal("Create() error = nil, want presign failure")
	}
}
