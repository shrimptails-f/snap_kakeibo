package application

import (
	"context"
	"fmt"
	"strings"

	analysisdomain "snap_kakeibo/backend/internal/analysis/domain"
	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/domain"
)

// RetryAnalysisInput は再解析ユースケースの入力。UserID は認証済みの利用者、AnalysisRequestID はパスパラメータ。
type RetryAnalysisInput struct {
	UserID            string
	AnalysisRequestID string
}

// RetryAnalysisOutput は再投入後の解析依頼の状態。Status は常に ANALYZING。
type RetryAnalysisOutput struct {
	AnalysisRequestID string
	Status            domain.AnalysisStatus
	Attempt           int
}

// RetryAnalysisUsecase は FAILED / NO_DATA / 停滞した ANALYZING の解析依頼を ANALYZING に戻して attempt を進め、
// 新しい attempt で analyze キューへ再投入する。
// attempt を進めるのは、前回の処理がまだ動いていてもその結果を attempt 不一致で捨てさせるため(docs/backend.md「再解析」)。
// SQS による同一試行の再配信や OpenAI の一時エラー再試行とは別の操作で、attempt を進めるのは再解析だけ。
type RetryAnalysisUsecase struct {
	Requests RetryAnalysisMarker
	Queue    RetryAnalysisEnqueuer
	Clock    timewrapper.Interface
}

// RetryAnalysisUsecaseInterface は HTTP 層が再解析ユースケースへ依存するための契約。
type RetryAnalysisUsecaseInterface interface {
	Retry(ctx context.Context, in RetryAnalysisInput) (RetryAnalysisOutput, error)
}

var _ RetryAnalysisUsecaseInterface = (*RetryAnalysisUsecase)(nil)

// NewRetryAnalysisUsecase は再解析ユースケースを生成する。
func NewRetryAnalysisUsecase(requests RetryAnalysisMarker, queue RetryAnalysisEnqueuer, clock timewrapper.Interface) RetryAnalysisUsecaseInterface {
	return &RetryAnalysisUsecase{Requests: requests, Queue: queue, Clock: clock}
}

// Retry は解析依頼を ANALYZING に戻してからキューへ送る。
// 解析依頼を更新できなければ送らない。送信に失敗した場合、解析依頼は ANALYZING のまま残るが、
// ANALYZING は再解析できる状態なので利用者がもう一度呼べば attempt を進めて送り直せる。
func (u *RetryAnalysisUsecase) Retry(ctx context.Context, in RetryAnalysisInput) (RetryAnalysisOutput, error) {
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.AnalysisRequestID) == "" {
		return RetryAnalysisOutput{}, ErrInvalidInput
	}
	// ここから先のログ(dynamodb_update_item / sqs_send span など)に user_id / analysis_request_id が付く
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.AnalysisRequestID(in.AnalysisRequestID))
	attempt, err := u.Requests.MarkRetrying(ctx, in.UserID, in.AnalysisRequestID, u.Clock.Now())
	if err != nil {
		return RetryAnalysisOutput{}, err
	}
	job, err := domain.NewRetryAnalysisJob(in.UserID, in.AnalysisRequestID, attempt)
	if err != nil {
		return RetryAnalysisOutput{}, fmt.Errorf("build retry analysis job: %w", err)
	}
	// sqs_send span と、analyze-receipt 側で同じ trace に繋がるログに attempt が付く
	ctx = logger.ContextWith(ctx, logger.Int("attempt", job.Attempt))
	if err := u.Queue.EnqueueRetryAnalysis(ctx, job); err != nil {
		return RetryAnalysisOutput{}, fmt.Errorf("enqueue retry analysis job: %w", err)
	}
	return RetryAnalysisOutput{AnalysisRequestID: job.AnalysisRequestID, Status: analysisdomain.AnalysisStatusAnalyzing, Attempt: job.Attempt}, nil
}
