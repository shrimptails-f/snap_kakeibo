// Package s3 は aws-sdk-go-v2 の S3 クライアントをログ付きで薄くラップする。
//
// SDK と同名なので、呼び出し側では SDK を awss3、このパッケージを libs3 のように別名で import する。
// 向き先(Floci か AWS か)は aws.Config で決まる。カスタムエンドポイントのときはパススタイル
// (http://host/bucket/key)にする。Floci はバケット名をホスト名に含める仮想ホスト形式を解決できない。
//
//	awsCfg, err := awsconfig.Load(ctx, oswrapper.New())
//	receipts := libs3.New(awsCfg, log).Bucket(cfg.ReceiptBucket)   // バケットに束縛(Bucket 参照)
//	data, err := receipts.GetBytes(ctx, key, 30<<20)                // s3_get_object span
//	err = receipts.PutBytes(ctx, key, raw, "application/json")      // s3_put_object span
//	url, err := receipts.PresignPutObject(ctx, key, "image/jpeg", 15*time.Minute)
//
// バケットを呼び出しごとに指定する Client.GetBytes などもある(S3 イベントが運んできたバケットを使う場合など)。
//
// 本文はログに出さない。span には bucket / s3_key / content_type / content_length だけを載せる。
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"snap_kakeibo/backend/internal/library/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// span 名のうち s3 が出すもの。
const (
	SpanGetObject = "s3_get_object"
	SpanPutObject = "s3_put_object"
)

// API は Client が使う SDK の操作。*awss3.Client が満たす。テストでは差し替える。
type API interface {
	GetObject(ctx context.Context, params *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *awss3.PutObjectInput, optFns ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
}

// Presigner は署名付き URL を作る操作。*awss3.PresignClient が満たす。
type Presigner interface {
	PresignPutObject(ctx context.Context, params *awss3.PutObjectInput, optFns ...func(*awss3.PresignOptions)) (*PresignedRequest, error)
}

// PresignedRequest は SDK の署名付きリクエスト型の別名。呼び出し側が signer パッケージに依存しないようにする。
type PresignedRequest = v4.PresignedHTTPRequest

var (
	_ API       = (*awss3.Client)(nil)
	_ Presigner = (*awss3.PresignClient)(nil)
)

// ErrTooLarge は GetBytes の上限を超えたときのエラー。
var ErrTooLarge = errors.New("s3: object exceeds the size limit")

// Client は S3 の読み書きをログ付きで行う。
type Client struct {
	api       API
	presigner Presigner
	log       logger.Interface
}

// New は cfg から SDK クライアントを作って Client を生成する。log が nil なら何も出力しない。
func New(cfg aws.Config, log logger.Interface) *Client {
	api := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		// Floci などのカスタムエンドポイントは仮想ホスト形式を解決できないのでパススタイルにする
		if cfg.BaseEndpoint != nil {
			o.UsePathStyle = true
		}
	})
	return NewWithAPI(api, awss3.NewPresignClient(api), log)
}

// NewWithAPI は SDK クライアント(またはテスト用の差し替え)を受け取って Client を生成する。presigner は nil でもよい。
func NewWithAPI(api API, presigner Presigner, log logger.Interface) *Client {
	if log == nil {
		log = logger.NewNop()
	}
	return &Client{api: api, presigner: presigner, log: log}
}

// GetObject は SDK の GetObject と同じ入出力で、s3_get_object span を出す。
// 返る Body は呼び出し側が Close する。span はレスポンスヘッダを受けた時点で終わる(本文の読み取りは含まない)。
func (c *Client) GetObject(ctx context.Context, in *awss3.GetObjectInput, optFns ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("s3: GetObjectInput is nil")
	}
	ctx, span := logger.StartSpan(ctx, c.log, SpanGetObject, objectFields(in.Bucket, in.Key)...)
	out, err := c.api.GetObject(ctx, in, optFns...)
	if err != nil {
		span.End(err)
		return nil, err
	}
	span.End(nil, logger.String("content_type", aws.ToString(out.ContentType)), logger.Int64("content_length", aws.ToInt64(out.ContentLength)))
	return out, nil
}

// PutObject は SDK の PutObject と同じ入出力で、s3_put_object span を出す。
func (c *Client) PutObject(ctx context.Context, in *awss3.PutObjectInput, optFns ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	if in == nil {
		return nil, fmt.Errorf("s3: PutObjectInput is nil")
	}
	fields := append(objectFields(in.Bucket, in.Key), logger.String("content_type", aws.ToString(in.ContentType)))
	if in.ContentLength != nil {
		fields = append(fields, logger.Int64("content_length", aws.ToInt64(in.ContentLength)))
	}
	ctx, span := logger.StartSpan(ctx, c.log, SpanPutObject, fields...)
	out, err := c.api.PutObject(ctx, in, optFns...)
	span.End(err)
	return out, err
}

// GetBytes はオブジェクトを最大 maxBytes まで読み込んで返す。それより大きければ ErrTooLarge。
func (c *Client) GetBytes(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("s3: maxBytes must be positive")
	}
	out, err := c.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()

	// 上限 +1 まで読み、超えていれば切り詰めず error にする
	data, err := io.ReadAll(io.LimitReader(out.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("s3: read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: %s/%s > %d bytes", ErrTooLarge, bucket, key, maxBytes)
	}
	return data, nil
}

// PutBytes はバイト列を contentType で書き込む。
func (c *Client) PutBytes(ctx context.Context, bucket, key string, body []byte, contentType string) error {
	_, err := c.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(body))),
	})
	return err
}

// PresignPutObject はブラウザなどから直接 PUT するための署名付き URL を返す。
// PUT する側は同じ Content-Type ヘッダを付ける必要がある。ネットワークには出ないので span は出さない。
func (c *Client) PresignPutObject(ctx context.Context, bucket, key, contentType string, expires time.Duration) (string, error) {
	if c.presigner == nil {
		return "", fmt.Errorf("s3: presigner is not configured")
	}
	req, err := c.presigner.PresignPutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(o *awss3.PresignOptions) {
		o.Expires = expires
	})
	if err != nil {
		return "", err
	}
	if req.Method != http.MethodPut {
		return "", fmt.Errorf("s3: unexpected presigned method %s", req.Method)
	}
	return req.URL, nil
}

// IsNotFound はオブジェクトまたはバケットが存在しないエラーなら true。
func IsNotFound(err error) bool {
	var noKey *types.NoSuchKey
	var noBucket *types.NoSuchBucket
	var notFound *types.NotFound
	if errors.As(err, &noKey) || errors.As(err, &noBucket) || errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NoSuchBucket", "NotFound":
			return true
		}
	}
	return false
}

func objectFields(bucket, key *string) []logger.Field {
	return []logger.Field{logger.String("bucket", aws.ToString(bucket)), logger.String("s3_key", aws.ToString(key))}
}
