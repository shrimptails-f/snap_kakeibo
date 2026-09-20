package oswrapper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OsWrapper は標準の os パッケージをそのまま使う実装。
type OsWrapper struct{}

var _ Interface = (*OsWrapper)(nil)

// New は OsWrapper を生成する。
func New() *OsWrapper {
	return &OsWrapper{}
}

// ReadFile はファイルを読み込み文字列として返す。相対パスは絶対パスに解決してから読む。
func (o *OsWrapper) ReadFile(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("oswrapper: resolve path %q: %w", path, err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("oswrapper: read file: %w", err)
	}
	return string(data), nil
}

// GetEnv は環境変数を返す。未設定または空白のみならエラー。
func (o *OsWrapper) GetEnv(key string) (string, error) {
	value := os.Getenv(key)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("oswrapper: environment variable %s is not set", key)
	}
	return value, nil
}
