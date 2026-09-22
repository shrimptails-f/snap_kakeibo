package application

import "errors"

var (
	// ErrInvalidInput は利用者または支出の識別子が欠けていて支出を引けない場合に返す。
	ErrInvalidInput = errors.New("invalid expense input")
	// ErrExpenseNotFound は利用者の支出に expense_id が無い場合にリポジトリが返す。
	// 他人の expense_id も同じエラーにし、存在の有無が分からないようにする。
	ErrExpenseNotFound = errors.New("expense not found")
	// ErrConcurrentUpdate は月次集計の version が再構築中に変化したことを表す。
	ErrConcurrentUpdate = errors.New("monthly summary was updated concurrently")
)
