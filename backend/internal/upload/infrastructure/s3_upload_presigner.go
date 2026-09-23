package infrastructure

import (
	"context"
	"time"

	libs3 "snap_kakeibo/backend/internal/library/s3"
	"snap_kakeibo/backend/internal/upload/application"
)

// S3UploadPresigner はレシートバケットへのサイズ制限付き POST フォームを発行する。
type S3UploadPresigner struct {
	Bucket *libs3.Bucket
}

// maxUploadBodyBytes は解析側の画像読込上限と同じ値。multipart 境界・フォーム項目もこの上限に含む。
const maxUploadBodyBytes = 30 << 20

var _ application.UploadFormPresigner = S3UploadPresigner{}

// PresignPost は Bucket に束縛した署名済み POST フォームを返す。
func (p S3UploadPresigner) PresignPost(ctx context.Context, key, contentType string, expires time.Duration) (application.UploadForm, error) {
	req, err := p.Bucket.PresignPostObject(ctx, key, contentType, maxUploadBodyBytes, expires)
	if err != nil {
		return application.UploadForm{}, err
	}
	return application.UploadForm{URL: req.URL, Fields: req.Values}, nil
}
