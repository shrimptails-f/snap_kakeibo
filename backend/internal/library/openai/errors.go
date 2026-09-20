package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

// APIError は OpenAI が 4xx / 5xx を返したときのエラー。本文全体は持たず、error object の項目だけを持つ。
type APIError struct {
	StatusCode int
	Type       string
	Code       string
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai: HTTP %d %s (%s)", e.StatusCode, e.Type, e.Code)
}

// Temporary は 429 / 5xx なら true。
func (e *APIError) Temporary() bool {
	return e.StatusCode == 429 || (e.StatusCode >= 500 && e.StatusCode < 600)
}

// ResponseFailedError は HTTP 200 だが status が failed のときのエラー。OpenAI 側の一時的な失敗として扱う。
type ResponseFailedError struct {
	ID        string
	RequestID string
	Type      string
	Code      string
	Message   string
}

func (e *ResponseFailedError) Error() string {
	return fmt.Sprintf("openai: response %s failed %s (%s)", e.ID, e.Type, e.Code)
}

// TransportError は HTTP のやり取り自体(接続・読み取り・本文の JSON デコード)の失敗。
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "openai: transport: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// IsTemporary はリトライで解決しうるエラーなら true。
//
//   - *APIError の 429 / 5xx
//   - *ResponseFailedError
//   - *TransportError(ネットワーク・読み取り)。ただし呼び出し元の ctx が切れた結果は除く
//   - 試行タイムアウト(context.DeadlineExceeded)
//
// 4xx(認証・不正なリクエスト・スキーマ違反)は何度投げても同じなので false。
func IsTemporary(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Temporary()
	}
	var failed *ResponseFailedError
	if errors.As(err, &failed) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var transport *TransportError
	if errors.As(err, &transport) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func apiErrorFrom(status int, requestID string, raw []byte) *APIError {
	e := &APIError{StatusCode: status, RequestID: requestID}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &body) == nil {
		e.Type, e.Code, e.Message = body.Error.Type, body.Error.Code, truncate(body.Error.Message)
	}
	return e
}
