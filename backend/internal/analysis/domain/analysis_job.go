package domain

// Trigger は解析ジョブの起点。
type Trigger string

const (
	// TriggerS3 は S3 へのアップロード通知から起動したジョブ。attempt は 1。
	TriggerS3 Trigger = "S3"
	// TriggerRetry は retry-analysis から再投入されたジョブ(再解析)。attempt は 2 以上。
	TriggerRetry Trigger = "RETRY"
)

// AnalysisJob は解析する解析依頼 1 件分の解析試行。Bucket / Key は元画像の場所。
// Attempt は analysis_requests.attempt と一致するときだけ処理を進めるための楽観ロックに使う。
// キューのメッセージから復元する値なので、値オブジェクトではなく文字列のまま持つ。
type AnalysisJob struct {
	UserID            string
	AnalysisRequestID string
	Attempt           int
	Trigger           Trigger
	Bucket            string
	Key               string
}

// AnalysisOutcome は解析試行 1 件の結末。解析依頼の状態(AnalysisStatus)と同じ語彙を使うが、
// 状態に保存しない SKIPPED を含むため別の型にしている。
type AnalysisOutcome string

const (
	OutcomeSucceeded AnalysisOutcome = "SUCCEEDED"
	OutcomeFailed    AnalysisOutcome = "FAILED"
	OutcomeNoData    AnalysisOutcome = "NO_DATA"
	// OutcomeSkipped は analysis_requests の状態や attempt が合わず、処理しなかった場合。DynamoDB には書かない。
	OutcomeSkipped AnalysisOutcome = "SKIPPED"
)
