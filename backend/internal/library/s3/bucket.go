package s3

import (
	"context"
	"time"
)

// Bucket は特定のバケットに束縛した Client。
// 業務コードはバケット名を設定から 1 回だけ受け取り、以降はキーだけで読み書きする。
// 統合テストではランダムな名前で作ったバケットを Bucket にして渡せるので、業務コードがバケット名を知る必要がない。
//
//	receipts := libs3.New(cfg, log).Bucket(cfg.ReceiptBucket)
//	data, err := receipts.GetBytes(ctx, key, 30<<20)
type Bucket struct {
	client *Client
	name   string
}

// Bucket は name に束縛した Bucket を返す。
func (c *Client) Bucket(name string) *Bucket {
	return &Bucket{client: c, name: name}
}

// Name はバケット名を返す。
func (b *Bucket) Name() string { return b.name }

// GetBytes は Client.GetBytes をこのバケットに対して行う。
func (b *Bucket) GetBytes(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	return b.client.GetBytes(ctx, b.name, key, maxBytes)
}

// PutBytes は Client.PutBytes をこのバケットに対して行う。
func (b *Bucket) PutBytes(ctx context.Context, key string, body []byte, contentType string) error {
	return b.client.PutBytes(ctx, b.name, key, body, contentType)
}

// PresignPutObject は Client.PresignPutObject をこのバケットに対して行う。
func (b *Bucket) PresignPutObject(ctx context.Context, key, contentType string, expires time.Duration) (string, error) {
	return b.client.PresignPutObject(ctx, b.name, key, contentType, expires)
}
