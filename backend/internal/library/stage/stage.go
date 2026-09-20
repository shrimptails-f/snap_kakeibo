// Package stage は実行環境(STAGE 環境変数)の語彙を 1 か所に持つ。
//
//   - local / ci: 開発者のマシンと CI。AWS の代わりにローカルの Floci を使う
//   - dev / stg / prd: デプロイ先の AWS 環境
//
// infra 側の Stage(デプロイ対象)とは別物で、local / ci はデプロイ先にならない。
package stage

import (
	"fmt"
	"strings"

	"snap_kakeibo/backend/internal/library/oswrapper"
)

// EnvKey は stage を渡す環境変数名。
const EnvKey = "STAGE"

// Stage は実行環境。
type Stage string

const (
	Local Stage = "local"
	CI    Stage = "ci"
	Dev   Stage = "dev"
	Stg   Stage = "stg"
	Prd   Stage = "prd"
)

// Parse は文字列を Stage にする。語彙にない値はエラー。
func Parse(value string) (Stage, error) {
	switch s := Stage(strings.ToLower(strings.TrimSpace(value))); s {
	case Local, CI, Dev, Stg, Prd:
		return s, nil
	default:
		return "", fmt.Errorf("stage: unknown %s %q (want local / ci / dev / stg / prd)", EnvKey, value)
	}
}

// FromEnv は STAGE 環境変数から Stage を返す。未設定・語彙外はエラー。
func FromEnv(osw oswrapper.Interface) (Stage, error) {
	value, err := osw.GetEnv(EnvKey)
	if err != nil {
		return "", err
	}
	return Parse(value)
}

// IsLocal は AWS ではなくローカルの Floci を使う環境(local / ci)なら true。
func (s Stage) IsLocal() bool {
	return s == Local || s == CI
}

// String は環境名を返す。
func (s Stage) String() string { return string(s) }
