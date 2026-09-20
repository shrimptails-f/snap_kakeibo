// Package trace は分散トレース用の trace_id / span_id を生成・伝搬する軽量な仕組みを提供する。
//
// ID の形式は W3C Trace Context(traceparent)に合わせる。OpenTelemetry や X-Ray と
// 相互変換できるので、後で本物のトレーサーに載せ替えても ID の意味は変わらない。
//
//   - trace_id: 32 桁 hex。1 つのリクエストから派生する処理の連鎖(SQS 経由を含む)で共通
//   - span_id:  16 桁 hex。処理単位ごとに採番し、parent_span_id で親子関係を残す
//
// ctx に積んだ Context は logger が自動で trace_id / span_id として出力する。
//
//	ctx, _ = trace.Start(ctx)                     // 入口: 既存があれば子 span、なければ新規 root
//	if tc, ok := trace.FromContext(ctx); ok {
//		attrs["traceparent"] = tc.Traceparent()   // SQS メッセージ属性や HTTP ヘッダで下流へ渡す
//	}
package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

const (
	traceIDLen = 32
	spanIDLen  = 16
)

// Context は 1 つの span を表すトレース情報。
type Context struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Sampled      bool
}

type contextKey struct{}

// NewRoot は新しい trace_id / span_id を持つ root span を生成する。
// trace_id の先頭 8 桁は X-Ray 互換のため Unix 秒(hex)にする。
func NewRoot() Context {
	return Context{
		TraceID: hexTimestamp(time.Now()) + randomHex(traceIDLen-8),
		SpanID:  randomHex(spanIDLen),
		Sampled: true,
	}
}

// NewChild は同じ trace に属する子 span を生成する。
func (c Context) NewChild() Context {
	if !c.Valid() {
		return NewRoot()
	}
	return Context{
		TraceID:      c.TraceID,
		SpanID:       randomHex(spanIDLen),
		ParentSpanID: c.SpanID,
		Sampled:      c.Sampled,
	}
}

// Valid は trace_id / span_id が形式として正しいかを返す。
func (c Context) Valid() bool {
	return isHex(c.TraceID, traceIDLen) && isHex(c.SpanID, spanIDLen) &&
		c.TraceID != strings.Repeat("0", traceIDLen) && c.SpanID != strings.Repeat("0", spanIDLen)
}

// ContextWith は trace 情報を積んだ新しい ctx を返す。
func ContextWith(ctx context.Context, tc Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, tc)
}

// FromContext は ctx に積まれた trace 情報を返す。なければ ok=false。
func FromContext(ctx context.Context) (Context, bool) {
	if ctx == nil {
		return Context{}, false
	}
	tc, ok := ctx.Value(contextKey{}).(Context)
	if !ok || !tc.Valid() {
		return Context{}, false
	}
	return tc, true
}

// Start は新しい span を開始し、それを積んだ ctx を返す。
// ctx に既存の trace があればその子 span、なければ新規 root になる。
func Start(ctx context.Context) (context.Context, Context) {
	var next Context
	if parent, ok := FromContext(ctx); ok {
		next = parent.NewChild()
	} else {
		next = NewRoot()
	}
	return ContextWith(ctx, next), next
}

// StartFrom は外部から受け取った trace(SQS メッセージ属性や HTTP ヘッダ)の子 span を開始する。
// parent が無効なら Start と同じ動きをする。
func StartFrom(ctx context.Context, parent Context) (context.Context, Context) {
	if !parent.Valid() {
		return Start(ctx)
	}
	next := parent.NewChild()
	return ContextWith(ctx, next), next
}

func hexTimestamp(t time.Time) string {
	sec := uint32(t.Unix()) //nolint:gosec // X-Ray の trace_id 仕様に合わせて下位 32bit だけ使う
	var b [4]byte
	b[0] = byte(sec >> 24)
	b[1] = byte(sec >> 16)
	b[2] = byte(sec >> 8)
	b[3] = byte(sec)
	return hex.EncodeToString(b[:])
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand の失敗はシステム異常なので、ID を諦めるより落とす
		panic("trace: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
