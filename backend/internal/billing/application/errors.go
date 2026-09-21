package application

import "errors"

var (
	// ErrInvalidInput は利用者または請求の識別子が欠けていて請求を引けない場合に返す。
	ErrInvalidInput = errors.New("invalid billing input")
	// ErrBillingNotFound は利用者の請求に billing_id が無い場合にリポジトリが返す。
	// 他人の billing_id も同じエラーにし、存在の有無が分からないようにする。
	ErrBillingNotFound = errors.New("billing not found")
)
