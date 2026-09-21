package domain

import (
	"errors"
	"strings"
)

// TriggerRetry は retry-analysis が analyze キューへ送るジョブの trigger。analyze-receipt はこの値で RETRY 形式を見分ける。
const TriggerRetry = "RETRY"

// ErrInvalidRetryAnalysisJob は識別子が欠けている、または attempt が進んでいないジョブを作ろうとしたときに返す。
var ErrInvalidRetryAnalysisJob = errors.New("retry analysis job requires user_id, analysis_request_id and an attempt after the first")

// RetryAnalysisJob は再解析のために analyze キューへ送る 1 件。
// Attempt は analysis_requests.attempt を進めた後の値で、analyze-receipt はこれが一致するときだけ結果を書く。
type RetryAnalysisJob struct {
	UserID            string
	AnalysisRequestID string
	Attempt           int
}

// NewRetryAnalysisJob は attempt を進めた後のジョブを作る。
// 再解析は必ず 2 回目以降なので、attempt が FirstAttempt 以下なら解析依頼の不整合として ErrInvalidRetryAnalysisJob を返す。
func NewRetryAnalysisJob(userID, requestID string, attempt int) (RetryAnalysisJob, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(requestID) == "" || attempt <= FirstAttempt {
		return RetryAnalysisJob{}, ErrInvalidRetryAnalysisJob
	}
	return RetryAnalysisJob{UserID: userID, AnalysisRequestID: requestID, Attempt: attempt}, nil
}
