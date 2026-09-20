package infrastructure

import (
	"context"
	"time"

	libs3 "snap_kakeibo/backend/internal/library/s3"
	"snap_kakeibo/backend/internal/upload/application"
)

// S3UploadPresigner はレシートバケットへの署名付き PUT URL を発行する。
type S3UploadPresigner struct {
	Bucket *libs3.Bucket
}

var _ application.UploadURLPresigner = S3UploadPresigner{}

// PresignPut は Bucket に束縛した署名付き PUT URL を返す。
func (p S3UploadPresigner) PresignPut(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	return p.Bucket.PresignPutObject(ctx, key, contentType, expires)
}
