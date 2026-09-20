package domain

// Trigger は解析ジョブの起点。
type Trigger string

const (
	// TriggerS3 は S3 へのアップロード通知から起動したジョブ。attempt は 1。
	TriggerS3 Trigger = "S3"
	// TriggerRetry は retry-upload から再投入されたジョブ。attempt は 2 以上。
	TriggerRetry Trigger = "RETRY"
)

// Job は解析するアップロード 1 件。Bucket / Key は元画像の場所。
// Attempt は upload_histories.attempt と一致するときだけ処理を進めるための楽観ロックに使う。
type Job struct {
	UserID   string
	UploadID string
	Attempt  int
	Trigger  Trigger
	Bucket   string
	Key      string
}

// Status は解析 1 件の結末。upload_histories.status と同じ語彙を使う。
type Status string

const (
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
	StatusNoData    Status = "NO_DATA"
	// StatusSkipped は upload_histories の状態や attempt が合わず、処理しなかった場合。DynamoDB には書かない。
	StatusSkipped Status = "SKIPPED"
)
