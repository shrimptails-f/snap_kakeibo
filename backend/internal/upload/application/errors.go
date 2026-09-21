package application

import "errors"

var (
	// ErrUploadAlreadyExists は同じ upload_id の履歴が既にある場合にリポジトリが返す。
	// ID は ULID で採番するので通常は起きず、起きたら採番の異常として呼び出し側は失敗させる。
	ErrUploadAlreadyExists = errors.New("upload already exists")
	// ErrInvalidInput は利用者の識別子が欠けている、月が YYYY-MM でないなど、入力から履歴を作れない・引けない場合に返す。
	ErrInvalidInput = errors.New("invalid upload input")
	// ErrUploadNotRetryable は履歴が無い、または status が再実行できない(UPLOADING / SUCCEEDED)場合にリポジトリが返す。
	// 履歴の有無を区別しないのは、他人の upload_id を当てても存在が分からないようにするため。
	ErrUploadNotRetryable = errors.New("upload cannot be retried")
)
