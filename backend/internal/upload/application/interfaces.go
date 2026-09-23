package application

import (
	"context"
	"time"

	"snap_kakeibo/backend/internal/upload/domain"
)

// AnalysisRequestRepository は analysis_requests に新しい解析依頼を登録する。
type AnalysisRequestRepository interface {
	// Save は解析依頼を新規登録する。同じ利用者・analysis_request_id の解析依頼が既にあれば ErrAnalysisRequestAlreadyExists を返す。
	Save(ctx context.Context, request domain.AnalysisRequest) error
}

// UploadForm は S3 へ直接 POST する署名済みフォーム。
type UploadForm struct {
	URL    string
	Fields map[string]string
}

// UploadFormPresigner は元画像を直接 POST するための署名済みフォームを発行する。
type UploadFormPresigner interface {
	PresignPost(ctx context.Context, key, contentType string, expires time.Duration) (UploadForm, error)
}

// IDGenerator は analysis_request_id を採番する。
type IDGenerator interface {
	NewID() (string, error)
}

// RetryAnalysisMarker は再解析できる解析依頼を ANALYZING に戻して attempt を進める。
type RetryAnalysisMarker interface {
	// MarkRetrying は status が domain.RetryableStatuses のいずれかであるときだけ ANALYZING にし、attempt を 1 進めて
	// 前回の失敗情報(error_code / error_message / failed_at)を消す。進めた後の attempt を返す。
	// 解析依頼が無い、または再解析できない status なら ErrAnalysisRequestNotRetryable を返す。
	MarkRetrying(ctx context.Context, userID, requestID string, now time.Time) (attempt int, err error)
}

// RetryAnalysisEnqueuer は再解析のジョブを analyze キューへ送る。
type RetryAnalysisEnqueuer interface {
	EnqueueRetryAnalysis(ctx context.Context, job domain.RetryAnalysisJob) error
}

// AnalysisRequestLister は利用者の解析依頼を月ごとに引く。
type AnalysisRequestLister interface {
	// ListPage は月全体へ状態フィルターを適用し、作成日時の降順で最大 PageSize 件と次のカーソルを返す。
	ListPage(ctx context.Context, query AnalysisRequestListQuery) (AnalysisRequestPage, error)
}

// AnalysisRequestExpenseReader は登録完了した解析依頼に対応する支出の一覧表示項目をまとめて取得する。
type AnalysisRequestExpenseReader interface {
	FindSummaries(ctx context.Context, userID string, expenseIDs []string) (map[string]ExpenseSummary, error)
}
