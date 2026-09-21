package infrastructure

import (
	"context"

	libsqs "snap_kakeibo/backend/internal/library/sqs"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

// retryMessage は analyze キューへ送る RETRY 形式の本文。analyze-receipt の library/queue.DecodeJobs が読む形と揃える。
type retryMessage struct {
	UserID   string `json:"user_id"`
	UploadID string `json:"upload_id"`
	Attempt  int    `json:"attempt"`
	Trigger  string `json:"trigger"`
}

// SQSAnalyzeQueue は analyze キューへ再実行ジョブを送る。
// Queue.SendJSON が ctx の trace を traceparent 属性に載せるので、analyze-receipt 側のログが同じ trace_id で繋がる。
type SQSAnalyzeQueue struct {
	Queue *libsqs.Queue
}

var _ application.AnalyzeJobEnqueuer = SQSAnalyzeQueue{}

// EnqueueRetry は job を RETRY 形式の JSON で送る。
func (q SQSAnalyzeQueue) EnqueueRetry(ctx context.Context, job domain.RetryJob) error {
	_, err := q.Queue.SendJSON(ctx, retryMessage{UserID: job.UserID, UploadID: job.UploadID, Attempt: job.Attempt, Trigger: domain.TriggerRetry})
	return err
}
