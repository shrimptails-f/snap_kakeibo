package application

import (
	"context"
	"fmt"
	"strings"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
	"snap_kakeibo/backend/internal/upload/domain"
)

// RetryUploadInput は再実行ユースケースの入力。UserID は認証済みの利用者、UploadID はパスパラメータ。
type RetryUploadInput struct {
	UserID   string
	UploadID string
}

// RetryUploadOutput は再投入後の履歴の状態。Status は常に ANALYZING。
type RetryUploadOutput struct {
	UploadID string
	Status   domain.Status
	Attempt  int
}

// RetryUploadUsecase は FAILED / NO_DATA / 停滞した ANALYZING の履歴を ANALYZING に戻して attempt を進め、
// 新しい attempt で analyze キューへ再投入する。
// attempt を進めるのは、前回の処理がまだ動いていてもその結果を attempt 不一致で捨てさせるため(docs/backend.md「再実行」)。
type RetryUploadUsecase struct {
	Histories UploadRetryMarker
	Queue     AnalyzeJobEnqueuer
	Clock     timewrapper.Interface
}

// RetryUploadUsecaseInterface は HTTP 層が再実行ユースケースへ依存するための契約。
type RetryUploadUsecaseInterface interface {
	Retry(ctx context.Context, in RetryUploadInput) (RetryUploadOutput, error)
}

var _ RetryUploadUsecaseInterface = (*RetryUploadUsecase)(nil)

// NewRetryUploadUsecase は再実行ユースケースを生成する。
func NewRetryUploadUsecase(histories UploadRetryMarker, queue AnalyzeJobEnqueuer, clock timewrapper.Interface) RetryUploadUsecaseInterface {
	return &RetryUploadUsecase{Histories: histories, Queue: queue, Clock: clock}
}

// Retry は履歴を ANALYZING に戻してからキューへ送る。
// 履歴を更新できなければ送らない。送信に失敗した場合、履歴は ANALYZING のまま残るが、
// ANALYZING は再実行できる status なので利用者がもう一度呼べば attempt を進めて送り直せる。
func (u *RetryUploadUsecase) Retry(ctx context.Context, in RetryUploadInput) (RetryUploadOutput, error) {
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.UploadID) == "" {
		return RetryUploadOutput{}, ErrInvalidInput
	}
	// ここから先のログ(dynamodb_update_item / sqs_send span など)に user_id / upload_id が付く
	ctx = logger.ContextWith(ctx, logger.UserID(in.UserID), logger.UploadID(in.UploadID))
	attempt, err := u.Histories.MarkRetrying(ctx, in.UserID, in.UploadID, u.Clock.Now())
	if err != nil {
		return RetryUploadOutput{}, err
	}
	job, err := domain.NewRetryJob(in.UserID, in.UploadID, attempt)
	if err != nil {
		return RetryUploadOutput{}, fmt.Errorf("build retry job: %w", err)
	}
	// sqs_send span と、analyze-receipt 側で同じ trace に繋がるログに attempt が付く
	ctx = logger.ContextWith(ctx, logger.Int("attempt", job.Attempt))
	if err := u.Queue.EnqueueRetry(ctx, job); err != nil {
		return RetryUploadOutput{}, fmt.Errorf("enqueue retry job: %w", err)
	}
	return RetryUploadOutput{UploadID: job.UploadID, Status: domain.StatusAnalyzing, Attempt: job.Attempt}, nil
}
