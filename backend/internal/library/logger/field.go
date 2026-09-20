package logger

import (
	"log/slog"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
)

const redacted = "[REDACTED]"

// Any は任意の値を付与する。
func Any(key string, value any) Field { return slog.Any(key, value) }

// String は文字列を付与する。
func String(key, value string) Field { return slog.String(key, value) }

// Bool は真偽値を付与する。
func Bool(key string, value bool) Field { return slog.Bool(key, value) }

// Int は整数を付与する。
func Int(key string, value int) Field { return slog.Int(key, value) }

// Int64 は 64bit 整数を付与する。
func Int64(key string, value int64) Field { return slog.Int64(key, value) }

// DurationMS は経過時間をミリ秒の整数 "duration_ms" として付与する。
// slog.Duration はナノ秒整数になり Logs Insights で扱いにくいので、ミリ秒に丸める。
func DurationMS(value time.Duration) Field { return slog.Int64("duration_ms", value.Milliseconds()) }

// Err はエラーを "error"(メッセージ)と "error_type"(原因の型名)の 2 キーで付与する。nil なら何も出力しない。
// error はメッセージなので値の種類が多く、集計には向かない。Logs Insights で
// `stats count() by error_type` のように分類したいときは error_type を使う。
func Err(err error) Field {
	if err == nil {
		return Field{}
	}
	// キーなしのグループは JSON ハンドラがトップレベルに展開する
	return slog.Attr{Value: slog.GroupValue(
		slog.String("error", err.Error()),
		slog.String("error_type", errorType(err)),
	)}
}

// errorType はエラーの分類に使う型名を返す。
// fmt.Errorf や errors.New の型はどのエラーでも同じで分類にならないので、Unwrap を辿って
// 一番内側にあるアプリ・SDK 由来の型を優先し、それがなければ一番内側の型を返す。
// 例: fmt.Errorf("x: %w", &json.SyntaxError{}) -> "json.SyntaxError"
func errorType(err error) string {
	var innermost, preferred string
	for e := err; e != nil; {
		name := strings.TrimPrefix(reflect.TypeOf(e).String(), "*")
		innermost = name
		if !strings.HasPrefix(name, "fmt.") && !strings.HasPrefix(name, "errors.") {
			preferred = name
		}
		switch u := e.(type) {
		case interface{ Unwrap() error }:
			e = u.Unwrap()
		case interface{ Unwrap() []error }:
			if errs := u.Unwrap(); len(errs) > 0 {
				e = errs[0]
			} else {
				e = nil
			}
		default:
			e = nil
		}
	}
	if preferred != "" {
		return preferred
	}
	return innermost
}

// Service はサービス名を共通スキーマのキーで付与する。
func Service(value string) Field { return String("service", normalizeFixedFieldValue(value)) }

// Environment は環境名を共通スキーマのキーで付与する。
func Environment(value string) Field { return String("environment", normalizeFixedFieldValue(value)) }

// Component はコンポーネント名(usecase / repository など)を付与する。
func Component(value string) Field { return String("component", value) }

// Event はログの種類を表す固定の識別子を付与する。
// message は人が読む文で、event は Logs Insights で filter する用途。
// 値は "<対象>_<過去形の動詞>" の snake_case(例: "openai_request_failed")で、
// 揺れを防ぐため呼び出し側パッケージで定数として宣言してから使う。
func Event(value string) Field { return String("event", value) }

// SpanName は処理区間の名前を付与する。通常は StartSpan が ctx に積むので手動で使う必要はない。
func SpanName(value string) Field { return String("span_name", value) }

// Status は処理の結果を表す固定の識別子("ok" / "error" や業務上の状態名)を付与する。
func Status(value string) Field { return String("status", value) }

// TraceID は分散トレースの trace_id を付与する。通常は ctx から自動で付くので手動で使う必要はない。
func TraceID(value string) Field { return String("trace_id", value) }

// SpanID は分散トレースの span_id を付与する。通常は ctx から自動で付く。
func SpanID(value string) Field { return String("span_id", value) }

// ParentSpanID は親 span の ID を付与する。通常は ctx から自動で付く。
func ParentSpanID(value string) Field { return String("parent_span_id", value) }

// RequestID はリクエストの相関 ID を付与する。
func RequestID(value string) Field { return String("request_id", value) }

// UserID はユーザー ID を付与する。
func UserID(value string) Field { return String("user_id", value) }

// UploadID はアップロード ID を付与する。
func UploadID(value string) Field { return String("upload_id", value) }

// BillingID は請求 ID を付与する。
func BillingID(value string) Field { return String("billing_id", value) }

// HTTPStatusCode は HTTP ステータスコードを付与する。
func HTTPStatusCode(value int) Field { return Int("http_status_code", value) }

// Recovered は recover() で拾った値を付与する。
func Recovered(value any) Field { return Any("recovered", value) }

// StackTrace は現在の goroutine のスタックトレースを付与する。
func StackTrace() Field { return String("stack_trace", string(debug.Stack())) }

func normalizeFixedFieldValue(value string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return "unknown"
}

// isSensitiveKey は値を伏せるべきキーかを判定する。
// "Access-Token" のような表記ゆれは "access_token" に正規化してから比較する。
func isSensitiveKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(key)))

	switch normalized {
	case "email",
		"mail",
		"phone",
		"address",
		"password",
		"token",
		"access_token",
		"refresh_token",
		"api_key",
		"apikey",
		"cookie",
		"set_cookie",
		"authorization",
		"session_id":
		return true
	default:
		return false
	}
}
