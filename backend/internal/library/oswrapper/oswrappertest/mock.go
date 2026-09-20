// Package oswrappertest は oswrapper.Interface のテスト用実装を提供する。
package oswrappertest

import (
	"fmt"
	"strings"

	"snap_kakeibo/backend/internal/library/oswrapper"
)

// Mock は map から環境変数とファイル内容を返す oswrapper.Interface 実装。
type Mock struct {
	Env   map[string]string
	Files map[string]string
}

var _ oswrapper.Interface = (*Mock)(nil)

// New は env をコピーして環境変数だけを持つ Mock を生成する。
func New(env map[string]string) *Mock {
	values := make(map[string]string, len(env))
	for key, value := range env {
		values[key] = value
	}
	return &Mock{Env: values, Files: map[string]string{}}
}

// GetEnv は Env の値を返す。未設定または空白のみならエラーを返す。
func (m *Mock) GetEnv(key string) (string, error) {
	value, ok := m.Env[key]
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("oswrappertest: environment variable %s is not set", key)
	}
	return value, nil
}

// ReadFile は Files の値を返す。未設定ならエラーを返す。
func (m *Mock) ReadFile(path string) (string, error) {
	value, ok := m.Files[path]
	if !ok {
		return "", fmt.Errorf("oswrappertest: file %s is not set", path)
	}
	return value, nil
}
