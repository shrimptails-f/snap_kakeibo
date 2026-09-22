package infrastructure

import (
	"context"
	"net/http"
	"testing"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	libs3 "snap_kakeibo/backend/internal/library/s3"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

type receiptURLPresigner struct {
	in      *awss3.GetObjectInput
	expires time.Duration
}

func (p *receiptURLPresigner) PresignGetObject(_ context.Context, in *awss3.GetObjectInput, optFns ...func(*awss3.PresignOptions)) (*libs3.PresignedRequest, error) {
	p.in = in
	opts := awss3.PresignOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}
	p.expires = opts.Expires
	return &libs3.PresignedRequest{URL: "https://example.com/receipt", Method: http.MethodGet}, nil
}

func (p *receiptURLPresigner) PresignPutObject(_ context.Context, _ *awss3.PutObjectInput, _ ...func(*awss3.PresignOptions)) (*libs3.PresignedRequest, error) {
	return &libs3.PresignedRequest{URL: "https://example.com/upload", Method: http.MethodPut}, nil
}

func TestS3ReceiptImageURLPresignerUsesOwnedReceiptKey(t *testing.T) {
	t.Parallel()
	presigner := &receiptURLPresigner{}
	client := libs3.NewWithAPI(nil, presigner, nil)
	userID, _ := common.NewUserID("u1")
	requestID, _ := common.NewAnalysisRequestID("r1")

	url, err := (S3ReceiptImageURLPresigner{Bucket: client.Bucket("receipts")}).PresignGet(context.Background(), userID, requestID, 15*time.Minute)
	if err != nil || url != "https://example.com/receipt" {
		t.Fatalf("PresignGet() = %q, %v", url, err)
	}
	if aws.ToString(presigner.in.Bucket) != "receipts" || aws.ToString(presigner.in.Key) != "receipts/u1/r1/original.jpg" || presigner.expires != 15*time.Minute {
		t.Errorf("input = %+v, expires = %v", presigner.in, presigner.expires)
	}
}
