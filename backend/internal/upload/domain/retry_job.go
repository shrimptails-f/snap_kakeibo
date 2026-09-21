package domain

import (
	"errors"
	"strings"
)

// TriggerRetry は retry-upload が analyze キューへ送るジョブの trigger。analyze-receipt はこの値で RETRY 形式を見分ける。
const TriggerRetry = "RETRY"

// ErrInvalidRetryJob は識別子が欠けている、または attempt が進んでいないジョブを作ろうとしたときに返す。
var ErrInvalidRetryJob = errors.New("retry job requires user_id, upload_id and an attempt after the first")

// RetryJob は解析をやり直すために analyze キューへ送る 1 件。
// Attempt は upload_histories.attempt を進めた後の値で、analyze-receipt はこれが一致するときだけ結果を書く。
type RetryJob struct {
	UserID   string
	UploadID string
	Attempt  int
}

// NewRetryJob は attempt を進めた後のジョブを作る。
// 再実行は必ず 2 回目以降なので、attempt が FirstAttempt 以下なら履歴の不整合として ErrInvalidRetryJob を返す。
func NewRetryJob(userID, uploadID string, attempt int) (RetryJob, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(uploadID) == "" || attempt <= FirstAttempt {
		return RetryJob{}, ErrInvalidRetryJob
	}
	return RetryJob{UserID: userID, UploadID: uploadID, Attempt: attempt}, nil
}
