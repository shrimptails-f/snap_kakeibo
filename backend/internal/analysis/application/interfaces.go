package application

import (
	"context"
	"time"

	"snap_kakeibo/backend/internal/analysis/domain"
)

// ReceiptImageReader はアップロードされたレシート画像を読み込む。
// 上限サイズは実装側が持ち、超えていれば error を返す。
type ReceiptImageReader interface {
	ReadImage(ctx context.Context, bucket, key string) ([]byte, error)
}

// RawResultStore は OpenAI の生レスポンスを調査用に保存し、保存先のキーを返す。
type RawResultStore interface {
	SaveRawResult(ctx context.Context, job domain.Job, responseID string, raw []byte) (key string, err error)
}

// ImageResizer は画像を OpenAI に渡せる大きさの JPEG に変換する。
// デコードできない画像は error を返し、usecase はそれを INTERNAL の失敗として記録する。
type ImageResizer interface {
	ResizeJPEG(data []byte) ([]byte, error)
}

// TokenUsage は OpenAI のトークン使用量。analysis span に載せて 1 件あたりのコストを集計する。
type TokenUsage struct {
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
}

// AnalysisResult は OpenAI から受け取った 1 回の応答。
// Failure が nil なら Receipt が有効。Failure があれば応答は受け取ったが解析結果として採用できない
// (拒否・未完了・スキーマ違反・4xx など、再試行しても解決しない)ことを表す。
// Raw は応答本文そのもので、あれば RawResultStore に保存する。4xx のように本文を持たない失敗では nil。
type AnalysisResult struct {
	ResponseID string
	Raw        []byte
	Usage      TokenUsage
	Receipt    domain.Receipt
	Failure    *domain.Failure
}

// ReceiptAnalyzer は JPEG 画像からレシートを読み取る。
// error は一時的または予期しない失敗で、usecase はジョブを失敗させてキューの再配信に任せる。
type ReceiptAnalyzer interface {
	Analyze(ctx context.Context, jpeg []byte) (AnalysisResult, error)
}

// IDGenerator は billing_id / detail_id などの識別子を採番する。
type IDGenerator interface {
	NewID() (string, error)
}

// UploadHistoryRepository は upload_histories の状態遷移を行う。
// いずれも status と attempt が期待どおりのときだけ書き込む。同じジョブが重複して配信されても、
// 先に終わった処理の結果を上書きしない。
type UploadHistoryRepository interface {
	// MarkAnalyzing は UPLOADING / ANALYZING かつ attempt が一致するときだけ ANALYZING にする。
	// 条件が合わなければ false を返し、呼び出し側はジョブをスキップする。
	MarkAnalyzing(ctx context.Context, job domain.Job, now time.Time) (bool, error)
	// MarkFailed は ANALYZING のアップロードを FAILED にし、失敗コードとメッセージを記録する。
	// 条件不一致(既に終端状態)は成功として扱う。
	MarkFailed(ctx context.Context, job domain.Job, failure domain.Failure, rawResultKey string, now time.Time) error
	// MarkNoData は ANALYZING のアップロードを NO_DATA にする。条件不一致は成功として扱う。
	MarkNoData(ctx context.Context, job domain.Job, rawResultKey string, now time.Time) error
}

// BillingRegistrar は請求・明細・月次集計の登録と upload_histories の SUCCEEDED への遷移を 1 つのトランザクションで行う。
// upload_histories の条件不一致(既に終端状態)は成功として扱い、登録内容が DynamoDB に拒否された場合は ErrBillingRejected を返す。
type BillingRegistrar interface {
	Register(ctx context.Context, job domain.Job, billing domain.Billing, rawResultKey string, now time.Time) error
}
