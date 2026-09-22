package infrastructure

import (
	"context"
	"time"

	common "snap_kakeibo/backend/internal/common/domain"
	"snap_kakeibo/backend/internal/ledger/application"
	libs3 "snap_kakeibo/backend/internal/library/s3"
	uploaddomain "snap_kakeibo/backend/internal/upload/domain"
)

// S3ReceiptImageURLPresigner は規定の元画像キーに対する署名付き GET URL を発行する。
type S3ReceiptImageURLPresigner struct {
	Bucket *libs3.Bucket
}

var _ application.ReceiptImageURLPresigner = S3ReceiptImageURLPresigner{}

// PresignGet は利用者と解析依頼から元画像キーを組み立て、期限付き URL を返す。
func (p S3ReceiptImageURLPresigner) PresignGet(ctx context.Context, userID common.UserID, requestID common.AnalysisRequestID, expires time.Duration) (string, error) {
	return p.Bucket.PresignGetObject(ctx, uploaddomain.ObjectKey(userID.String(), requestID.String()), expires)
}
