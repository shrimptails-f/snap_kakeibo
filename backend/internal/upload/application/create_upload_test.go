package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

var now = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

type histories struct {
	saved []domain.UploadHistory
	err   error
}

func (h *histories) Save(_ context.Context, history domain.UploadHistory) error {
	if h.err != nil {
		return h.err
	}
	h.saved = append(h.saved, history)
	return nil
}

type presigner struct {
	key         string
	contentType string
	expires     time.Duration
	err         error
}

func (p *presigner) PresignPut(_ context.Context, key, contentType string, expires time.Duration) (string, error) {
	p.key, p.contentType, p.expires = key, contentType, expires
	if p.err != nil {
		return "", p.err
	}
	return "https://example.com/" + key, nil
}

type ids struct{ err error }

func (i ids) NewID() (string, error) { return "up1", i.err }

// fixture は差し替え可能な依存一式。各テストはこれを変えてから build する。
type fixture struct {
	histories *histories
	urls      *presigner
	ids       ids
}

func newFixture() *fixture { return &fixture{histories: &histories{}, urls: &presigner{}} }

func (f *fixture) build() application.CreateUploadUsecaseInterface {
	return application.NewCreateUploadUsecase(f.histories, f.urls, f.ids, timewrapper.NewFixed(now))
}

func TestCreateRegistersHistoryThenPresigns(t *testing.T) {
	t.Parallel()
	f := newFixture()
	out, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1", FileName: "a.png", ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	want := application.CreateUploadOutput{UploadID: "up1", S3Key: "receipts/u1/up1/original.jpg", PutURL: "https://example.com/receipts/u1/up1/original.jpg", ExpiresAt: now.Add(15 * time.Minute)}
	if out != want {
		t.Errorf("Create() = %+v, want %+v", out, want)
	}
	if len(f.histories.saved) != 1 {
		t.Fatalf("saved = %+v", f.histories.saved)
	}
	saved := f.histories.saved[0]
	if saved.UserID != "u1" || saved.UploadID != "up1" || saved.Status != domain.StatusUploading || saved.Attempt != 1 || saved.FileName != "a.png" || saved.ContentType != "image/png" || !saved.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("saved history = %+v", saved)
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
	saved := f.histories.saved[0]
	if saved.FileName != domain.DefaultFileName || saved.ContentType != domain.DefaultContentType || f.urls.contentType != domain.DefaultContentType {
		t.Errorf("defaults: history = %+v, presign content_type = %q", saved, f.urls.contentType)
	}
}

func TestCreateRejectsMissingUser(t *testing.T) {
	t.Parallel()
	f := newFixture()
	_, err := f.build().Create(context.Background(), application.CreateUploadInput{})
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}
	if len(f.histories.saved) != 0 || f.urls.key != "" {
		t.Errorf("no side effects expected: %+v %+v", f.histories, f.urls)
	}
}

func TestCreateDoesNotPresignWhenSaveFails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.histories.err = application.ErrUploadAlreadyExists
	_, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1"})
	if !errors.Is(err, application.ErrUploadAlreadyExists) {
		t.Fatalf("Create() error = %v, want ErrUploadAlreadyExists", err)
	}
	if f.urls.key != "" {
		t.Errorf("PresignPut should not be called, got key %q", f.urls.key)
	}
}

func TestCreateFailsWhenIDGenerationFails(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.ids = ids{err: errors.New("boom")}
	if _, err := f.build().Create(context.Background(), application.CreateUploadInput{UserID: "u1"}); err == nil || len(f.histories.saved) != 0 {
		t.Fatalf("Create() error = %v, saved = %+v", err, f.histories.saved)
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
