package application

import "errors"

var (
	// ErrAnalysisRequestAlreadyExists は同じ analysis_request_id の解析依頼が既にある場合にリポジトリが返す。
	// ID は ULID で採番するので通常は起きず、起きたら採番の異常として呼び出し側は失敗させる。
	ErrAnalysisRequestAlreadyExists = errors.New("analysis request already exists")
	// ErrInvalidInput は利用者の識別子が欠けている、月が YYYY-MM でないなど、入力から解析依頼を作れない・引けない場合に返す。
	ErrInvalidInput = errors.New("invalid upload input")
	// ErrAnalysisRequestNotRetryable は解析依頼が無い、または状態が再解析できない(UPLOADING / SUCCEEDED)場合にリポジトリが返す。
	// 解析依頼の有無を区別しないのは、他人の analysis_request_id を当てても存在が分からないようにするため。
	ErrAnalysisRequestNotRetryable = errors.New("analysis request cannot be retried")
)
