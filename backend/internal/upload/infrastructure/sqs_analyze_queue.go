package infrastructure

import (
	"context"

	libsqs "snap_kakeibo/backend/internal/library/sqs"
	"snap_kakeibo/backend/internal/upload/application"
	"snap_kakeibo/backend/internal/upload/domain"
)

// retryMessage は analyze キューへ送る RETRY 形式の本文。analyze-receipt の library/queue.DecodeJobs が読む形と揃える。
type retryMessage struct {
	UserID            string `json:"user_id"`
	AnalysisRequestID string `json:"analysis_request_id"`
	Attempt           int    `json:"attempt"`
	Trigger           string `json:"trigger"`
}

// SQSAnalyzeQueue は analyze キューへ再解析ジョブを送る。
// Queue.SendJSON が ctx の trace を traceparent 属性に載せるので、analyze-receipt 側のログが同じ trace_id で繋がる。
type SQSAnalyzeQueue struct {
	Queue *libsqs.Queue
}

var _ application.RetryAnalysisEnqueuer = SQSAnalyzeQueue{}

// EnqueueRetryAnalysis は job を RETRY 形式の JSON で送る。
func (q SQSAnalyzeQueue) EnqueueRetryAnalysis(ctx context.Context, job domain.RetryAnalysisJob) error {
	_, err := q.Queue.SendJSON(ctx, retryMessage{UserID: job.UserID, AnalysisRequestID: job.AnalysisRequestID, Attempt: job.Attempt, Trigger: domain.TriggerRetry})
	return err
}
