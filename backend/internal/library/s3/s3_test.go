package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"snap_kakeibo/backend/internal/library/awsconfig"
	"snap_kakeibo/backend/internal/library/awstest"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/oswrapper"
	"snap_kakeibo/backend/internal/library/stage"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeAPI はメモリ上のバケット。入力を記録する。
type fakeAPI struct {
	objects map[string][]byte
	putIn   *awss3.PutObjectInput
	getIn   *awss3.GetObjectInput
	err     error
}

type fakePresigner struct {
	getIn      *awss3.GetObjectInput
	putIn      *awss3.PutObjectInput
	getExpires time.Duration
	putExpires time.Duration
	err        error
}

func (f *fakePresigner) PresignGetObject(_ context.Context, in *awss3.GetObjectInput, optFns ...func(*awss3.PresignOptions)) (*PresignedRequest, error) {
	f.getIn = in
	opts := awss3.PresignOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}
	f.getExpires = opts.Expires
	if f.err != nil {
		return nil, f.err
	}
	return &PresignedRequest{URL: "https://example.com/get", Method: http.MethodGet}, nil
}

func (f *fakePresigner) PresignPutObject(_ context.Context, in *awss3.PutObjectInput, optFns ...func(*awss3.PresignOptions)) (*PresignedRequest, error) {
	f.putIn = in
	opts := awss3.PresignOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}
	f.putExpires = opts.Expires
	if f.err != nil {
		return nil, f.err
	}
	return &PresignedRequest{URL: "https://example.com/put", Method: http.MethodPut}, nil
}

func newFakeAPI() *fakeAPI { return &fakeAPI{objects: map[string][]byte{}} }

func (f *fakeAPI) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	f.getIn = in
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.objects[aws.ToString(in.Key)]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &awss3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String("application/octet-stream"),
	}, nil
}

func (f *fakeAPI) PutObject(_ context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	f.putIn = in
	if f.err != nil {
		return nil, f.err
	}
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.objects[aws.ToString(in.Key)] = data
	return &awss3.PutObjectOutput{}, nil
}

func TestPutBytesAndGetBytes(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	log, buf := newTestLogger()
	ctx := logger.ContextWith(context.Background(), logger.AnalysisRequestID("u1"))
	c := NewWithAPI(api, nil, log)

	if err := c.PutBytes(ctx, "b", "k.json", []byte(`{"a":1}`), "application/json"); err != nil {
		t.Fatal(err)
	}
	if aws.ToString(api.putIn.Bucket) != "b" || aws.ToString(api.putIn.ContentType) != "application/json" || aws.ToInt64(api.putIn.ContentLength) != 7 {
		t.Fatalf("put input=%+v", api.putIn)
	}

	got, err := c.GetBytes(ctx, "b", "k.json", 1024)
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("got %q err=%v", got, err)
	}

	entries := allEntries(t, buf)
	var finished []map[string]any
	for _, e := range entries {
		if e["event"] == logger.EventSpanFinished {
			finished = append(finished, e)
		}
	}
	if len(finished) != 2 {
		t.Fatalf("span_finished=%d: %s", len(finished), buf.String())
	}
	put, get := finished[0], finished[1]
	assertField(t, put, "span_name", SpanPutObject)
	assertField(t, put, "bucket", "b")
	assertField(t, put, "s3_key", "k.json")
	assertField(t, put, "content_type", "application/json")
	assertField(t, put, "content_length", float64(7))
	assertField(t, put, "status", logger.StatusOK)
	assertField(t, put, "analysis_request_id", "u1")
	assertField(t, get, "span_name", SpanGetObject)
	assertField(t, get, "content_length", float64(7))
	assertField(t, get, "content_type", "application/octet-stream")
	if strings.Contains(buf.String(), `{\"a\":1}`) || strings.Contains(buf.String(), `"a":1`) {
		t.Fatalf("body must not be logged: %s", buf.String())
	}
}

func TestGetBytesTooLarge(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.objects["big"] = bytes.Repeat([]byte("x"), 11)
	c := NewWithAPI(api, nil, nil)

	if _, err := c.GetBytes(context.Background(), "b", "big", 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err=%v want ErrTooLarge", err)
	}
	if got, err := c.GetBytes(context.Background(), "b", "big", 11); err != nil || len(got) != 11 {
		t.Fatalf("exactly at limit must succeed: len=%d err=%v", len(got), err)
	}
	if _, err := c.GetBytes(context.Background(), "b", "big", 0); err == nil {
		t.Fatal("maxBytes<=0 must be rejected")
	}
}

func TestGetBytesNotFoundAndErrorSpan(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	log, buf := newTestLogger()
	c := NewWithAPI(api, nil, log)

	_, err := c.GetBytes(context.Background(), "b", "missing", 10)
	if !IsNotFound(err) {
		t.Fatalf("err=%v want not found", err)
	}
	entries := allEntries(t, buf)
	last := entries[len(entries)-1]
	assertField(t, last, "event", logger.EventSpanFinished)
	assertField(t, last, "status", logger.StatusError)
	assertField(t, last, "error_type", "types.NoSuchKey")

	if IsNotFound(errors.New("other")) || IsNotFound(nil) {
		t.Fatal("IsNotFound must be false for other errors")
	}
}

func TestBucketBindsName(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	b := NewWithAPI(api, nil, nil).Bucket("bound")

	if err := b.PutBytes(context.Background(), "k", []byte("v"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	if aws.ToString(api.putIn.Bucket) != "bound" {
		t.Fatalf("bucket=%q", aws.ToString(api.putIn.Bucket))
	}
	if got, err := b.GetBytes(context.Background(), "k", 10); err != nil || string(got) != "v" || aws.ToString(api.getIn.Bucket) != "bound" {
		t.Fatalf("got %q err=%v bucket=%q", got, err, aws.ToString(api.getIn.Bucket))
	}
}

func TestPresignedObjectURLs(t *testing.T) {
	t.Parallel()
	presigner := &fakePresigner{}
	b := NewWithAPI(newFakeAPI(), presigner, nil).Bucket("receipts")

	getURL, err := b.PresignGetObject(context.Background(), "receipts/u1/r1/original.jpg", 15*time.Minute)
	if err != nil || getURL != "https://example.com/get" {
		t.Fatalf("PresignGetObject() = %q, %v", getURL, err)
	}
	if aws.ToString(presigner.getIn.Bucket) != "receipts" || aws.ToString(presigner.getIn.Key) != "receipts/u1/r1/original.jpg" || presigner.getExpires != 15*time.Minute {
		t.Errorf("get input = %+v, expires = %v", presigner.getIn, presigner.getExpires)
	}

	putURL, err := b.PresignPutObject(context.Background(), "receipts/u1/r1/original.jpg", "image/jpeg", 10*time.Minute)
	if err != nil || putURL != "https://example.com/put" {
		t.Fatalf("PresignPutObject() = %q, %v", putURL, err)
	}
	if aws.ToString(presigner.putIn.Bucket) != "receipts" || presigner.putExpires != 10*time.Minute {
		t.Errorf("put input = %+v, expires = %v", presigner.putIn, presigner.putExpires)
	}
}

func TestNilInputs(t *testing.T) {
	t.Parallel()
	c := NewWithAPI(newFakeAPI(), nil, nil)
	if _, err := c.GetObject(context.Background(), nil); err == nil {
		t.Fatal("nil GetObjectInput")
	}
	if _, err := c.PutObject(context.Background(), nil); err == nil {
		t.Fatal("nil PutObjectInput")
	}
	if _, err := c.PresignPutObject(context.Background(), "b", "k", "image/jpeg", time.Minute); err == nil {
		t.Fatal("nil presigner must be an error")
	}
	if _, err := c.PresignGetObject(context.Background(), "b", "k", time.Minute); err == nil {
		t.Fatal("nil presigner must be an error")
	}
}

// TestFlociRoundTrip は実際の S3 API(ローカルの Floci)に対して put → get → 上限超過 → 未存在 → 署名付き URL への PUT を通す。
// STAGE が local / ci のときだけ動く(devcontainer と CI)。それ以外はスキップする。
func TestFlociRoundTrip(t *testing.T) {
	t.Parallel()
	osw := oswrapper.New()
	if st, err := stage.FromEnv(osw); err != nil || !st.IsLocal() {
		t.Skipf("STAGE is not local / ci (stage=%q err=%v); skipping Floci integration test", st, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.Load(ctx, osw)
	if err != nil {
		t.Fatal(err)
	}
	log, buf := newTestLogger()
	c := New(cfg, log)

	bucket := awstest.ResourceName("s3-roundtrip")
	raw := awss3.NewFromConfig(cfg, func(o *awss3.Options) { o.UsePathStyle = true })
	if _, err := raw.CreateBucket(ctx, &awss3.CreateBucketInput{
		Bucket:                    aws.String(bucket),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(cfg.Region)},
	}); err != nil {
		t.Fatalf("CreateBucket against %s failed: %v", aws.ToString(cfg.BaseEndpoint), err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		for _, key := range []string{"results/1.json", "receipts/1.jpg"} {
			_, _ = raw.DeleteObject(cleanup, &awss3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		}
		_, _ = raw.DeleteBucket(cleanup, &awss3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	// 業務コードに渡す形: ランダムな名前のバケットに束縛した Bucket
	bound := c.Bucket(bucket)
	if bound.Name() != bucket {
		t.Fatalf("Name()=%q", bound.Name())
	}

	// put → get
	body := []byte(`{"store_name":"A","total_amount":100}`)
	if err := bound.PutBytes(ctx, "results/1.json", body, "application/json"); err != nil {
		t.Fatalf("PutBytes: %v", err)
	}
	got, err := bound.GetBytes(ctx, "results/1.json", 1<<20)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("GetBytes: %q err=%v", got, err)
	}
	head, err := raw.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("results/1.json")})
	if err != nil || aws.ToString(head.ContentType) != "application/json" {
		t.Fatalf("HeadObject: %+v err=%v", head, err)
	}

	// 上限超過 / 未存在
	if _, err := c.GetBytes(ctx, bucket, "results/1.json", 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large: err=%v", err)
	}
	if _, err := c.GetBytes(ctx, bucket, "results/missing.json", 1<<20); !IsNotFound(err) {
		t.Fatalf("missing: err=%v", err)
	}

	// 署名付き URL に PUT → GET で読める
	url, err := bound.PresignPutObject(ctx, "receipts/1.jpg", "image/jpeg", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutObject: %v", err)
	}
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jpeg))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT to presigned URL: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("PUT to presigned URL: HTTP %d", resp.StatusCode)
	}
	got, err = c.GetBytes(ctx, bucket, "receipts/1.jpg", 1<<20)
	if err != nil || !bytes.Equal(got, jpeg) {
		t.Fatalf("GetBytes after presigned PUT: %v err=%v", got, err)
	}
	getURL, err := bound.PresignGetObject(ctx, "receipts/1.jpg", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetObject: %v", err)
	}
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()
	got, err = io.ReadAll(getResp.Body)
	if err != nil || getResp.StatusCode != http.StatusOK || !bytes.Equal(got, jpeg) {
		t.Fatalf("GET presigned URL: status=%d body=%v err=%v", getResp.StatusCode, got, err)
	}

	// ログ: 各操作が span_finished に bucket / s3_key 付きで出る
	var spans []string
	for _, e := range allEntries(t, buf) {
		if e["event"] == logger.EventSpanFinished {
			spans = append(spans, fmt.Sprintf("%s:%s:%s", e["span_name"], e["status"], e["s3_key"]))
		}
	}
	want := []string{
		SpanPutObject + ":ok:results/1.json",
		SpanGetObject + ":ok:results/1.json",
		SpanGetObject + ":ok:results/1.json", // 上限超過は本文を読んでから分かるので span 自体は ok
		SpanGetObject + ":error:results/missing.json",
		SpanGetObject + ":ok:receipts/1.jpg",
	}
	if strings.Join(spans, "\n") != strings.Join(want, "\n") {
		t.Fatalf("spans:\n%s\nwant:\n%s", strings.Join(spans, "\n"), strings.Join(want, "\n"))
	}
	t.Logf("endpoint=%s spans=%v", aws.ToString(cfg.BaseEndpoint), spans)
}

func newTestLogger() (*logger.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return logger.New(logger.Options{Level: "debug", Service: "test", Environment: "test", Writer: &buf}), &buf
}

func allEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func assertField(t *testing.T, entry map[string]any, key string, want any) {
	t.Helper()
	got, ok := entry[key]
	if !ok {
		t.Fatalf("%s is missing: %v", key, entry)
	}
	if got != want {
		t.Fatalf("%s=%v (%T) want %v (%T)", key, got, got, want, want)
	}
}
