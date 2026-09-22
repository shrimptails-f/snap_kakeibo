package infrastructure

import (
	"context"
	"net/http"
	"testing"
	"time"

	libs3 "snap_kakeibo/backend/internal/library/s3"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// fakePresigner は受け取った入力を記録し、固定の URL を返す。
type fakePresigner struct {
	in      *awss3.PutObjectInput
	expires time.Duration
}

func (f *fakePresigner) PresignGetObject(_ context.Context, _ *awss3.GetObjectInput, _ ...func(*awss3.PresignOptions)) (*libs3.PresignedRequest, error) {
	return &libs3.PresignedRequest{URL: "https://example.com/signed", Method: http.MethodGet}, nil
}

func (f *fakePresigner) PresignPutObject(_ context.Context, in *awss3.PutObjectInput, optFns ...func(*awss3.PresignOptions)) (*libs3.PresignedRequest, error) {
	f.in = in
	opts := awss3.PresignOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}
	f.expires = opts.Expires
	return &libs3.PresignedRequest{URL: "https://example.com/signed", Method: http.MethodPut}, nil
}

func TestPresignPutUsesTheBoundBucket(t *testing.T) {
	t.Parallel()
	api := &fakePresigner{}
	client := libs3.NewWithAPI(nil, api, nil)
	presigner := S3UploadPresigner{Bucket: client.Bucket("receipts")}

	got, err := presigner.PresignPut(context.Background(), "receipts/u1/up1/original.jpg", "image/png", 15*time.Minute)
	if err != nil || got != "https://example.com/signed" {
		t.Fatalf("PresignPut() = %q, %v", got, err)
	}
	if aws.ToString(api.in.Bucket) != "receipts" || aws.ToString(api.in.Key) != "receipts/u1/up1/original.jpg" || aws.ToString(api.in.ContentType) != "image/png" || api.expires != 15*time.Minute {
		t.Errorf("PresignPutObject input = %+v, expires = %v", api.in, api.expires)
	}
}
