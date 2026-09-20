package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"snap_kakeibo/backend/internal/analysis/domain"
	libs3 "snap_kakeibo/backend/internal/library/s3"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// fakeS3 は 1 オブジェクトだけを持つメモリ上の S3。
type fakeS3 struct {
	data   []byte
	getIn  *awss3.GetObjectInput
	putIn  *awss3.PutObjectInput
	putted []byte
}

func (f *fakeS3) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	f.getIn = in
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.data)), ContentLength: aws.Int64(int64(len(f.data)))}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	f.putIn = in
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.putted = body
	return &awss3.PutObjectOutput{}, nil
}

var storageJob = domain.Job{UserID: "u1", UploadID: "up1", Attempt: 2, Bucket: "incoming", Key: "receipts/u1/up1/original.jpg"}

func TestReadImageReadsFromTheJobBucket(t *testing.T) {
	t.Parallel()
	api := &fakeS3{data: []byte("image")}
	client := libs3.NewWithAPI(api, nil, nil)
	storage := S3ReceiptStorage{Client: client, Results: client.Bucket("results")}

	got, err := storage.ReadImage(context.Background(), storageJob.Bucket, storageJob.Key)
	if err != nil || string(got) != "image" {
		t.Fatalf("ReadImage() = %q, %v", got, err)
	}
	if aws.ToString(api.getIn.Bucket) != "incoming" || aws.ToString(api.getIn.Key) != storageJob.Key {
		t.Errorf("GetObject input = %+v", api.getIn)
	}
}

func TestReadImageRejectsOversizedImage(t *testing.T) {
	t.Parallel()
	api := &fakeS3{data: []byte("0123456789")}
	client := libs3.NewWithAPI(api, nil, nil)
	storage := S3ReceiptStorage{Client: client, Results: client.Bucket("results"), MaxImageBytes: 5}
	if _, err := storage.ReadImage(context.Background(), "b", "k"); !errors.Is(err, libs3.ErrTooLarge) {
		t.Fatalf("ReadImage() error = %v, want ErrTooLarge", err)
	}
}

func TestSaveRawResultWritesJSONToResultsBucket(t *testing.T) {
	t.Parallel()
	api := &fakeS3{}
	client := libs3.NewWithAPI(api, nil, nil)
	storage := S3ReceiptStorage{Client: client, Results: client.Bucket("results")}

	key, err := storage.SaveRawResult(context.Background(), storageJob, "resp_A/../x", []byte(`{"id":"resp_A"}`))
	if err != nil {
		t.Fatalf("SaveRawResult() error = %v", err)
	}
	if key != "analysis-results/u1/up1/2/resp_Ax.json" {
		t.Errorf("key = %q", key)
	}
	if aws.ToString(api.putIn.Bucket) != "results" || aws.ToString(api.putIn.Key) != key || aws.ToString(api.putIn.ContentType) != "application/json" || string(api.putted) != `{"id":"resp_A"}` {
		t.Errorf("PutObject input = %+v body = %s", api.putIn, api.putted)
	}
}

func TestRawResultKeyKeepsResponseIDsDistinct(t *testing.T) {
	t.Parallel()
	a, b := RawResultKey(storageJob, "resp_A"), RawResultKey(storageJob, "resp_B")
	if a == b {
		t.Fatalf("response IDs must remain unique: %q", a)
	}
	if got := RawResultKey(storageJob, "///"); got != "analysis-results/u1/up1/2/response.json" {
		t.Errorf("empty safe id = %q", got)
	}
}
