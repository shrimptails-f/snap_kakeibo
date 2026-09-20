// Package infrastructure はレシート解析の application が定義した interface の S3 / OpenAI / DynamoDB 実装を提供する。
package infrastructure

import (
	"context"
	"fmt"
	"strings"

	"snap_kakeibo/backend/internal/analysis/application"
	"snap_kakeibo/backend/internal/analysis/domain"
	libs3 "snap_kakeibo/backend/internal/library/s3"
)

// DefaultMaxImageBytes は S3 から読む画像の上限。
const DefaultMaxImageBytes = 30 << 20

// rawResultPrefix は生レスポンスの保存先。infra の PutObject 権限(analysis-results/*)と揃える。
const rawResultPrefix = "analysis-results"

// S3ReceiptStorage は元画像の読み込みと生レスポンスの保存を S3 で行う。
// 元画像は S3 通知が運んできたバケットから読み、生レスポンスは Results に束縛したバケットへ書く。
type S3ReceiptStorage struct {
	Client  *libs3.Client
	Results *libs3.Bucket
	// MaxImageBytes は元画像の上限。0 なら DefaultMaxImageBytes
	MaxImageBytes int64
}

var (
	_ application.ReceiptImageReader = S3ReceiptStorage{}
	_ application.RawResultStore     = S3ReceiptStorage{}
)

// ReadImage は元画像を最大 MaxImageBytes まで読み込む。超えていれば error。
func (s S3ReceiptStorage) ReadImage(ctx context.Context, bucket, key string) ([]byte, error) {
	maxBytes := s.MaxImageBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxImageBytes
	}
	return s.Client.GetBytes(ctx, bucket, key, maxBytes)
}

// SaveRawResult は生レスポンスを analysis-results/<user_id>/<upload_id>/<attempt>/<response_id>.json に保存する。
func (s S3ReceiptStorage) SaveRawResult(ctx context.Context, job domain.Job, responseID string, raw []byte) (string, error) {
	key := RawResultKey(job, responseID)
	if err := s.Results.PutBytes(ctx, key, raw, "application/json"); err != nil {
		return "", err
	}
	return key, nil
}

// RawResultKey は生レスポンスの保存キーを返す。responseID はキーに使える文字だけに落とす。
func RawResultKey(job domain.Job, responseID string) string {
	return fmt.Sprintf("%s/%s/%s/%d/%s.json", rawResultPrefix, job.UserID, job.UploadID, job.Attempt, safeID(responseID))
}

// safeID は英数字と - _ 以外を取り除く。空になれば "response"。
func safeID(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "response"
	}
	return b.String()
}
