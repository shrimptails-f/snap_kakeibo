package trace

import "strings"

// TraceparentHeader は W3C Trace Context のヘッダ名。HTTP ヘッダや SQS メッセージ属性のキーに使う。
const TraceparentHeader = "traceparent"

// XRayHeader は X-Ray のトレースヘッダ名。SQS の system attribute(AWSTraceHeader)や
// Lambda の環境変数 _X_AMZN_TRACE_ID も同じ形式。
const XRayHeader = "X-Amzn-Trace-Id"

// Traceparent は W3C traceparent 形式("00-<trace_id>-<span_id>-<flags>")を返す。
func (c Context) Traceparent() string {
	if !c.Valid() {
		return ""
	}
	flags := "00"
	if c.Sampled {
		flags = "01"
	}
	return "00-" + c.TraceID + "-" + c.SpanID + "-" + flags
}

// ParseTraceparent は W3C traceparent 形式を解析する。形式が不正なら ok=false。
func ParseTraceparent(s string) (Context, bool) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 4 || parts[0] != "00" {
		return Context{}, false
	}
	tc := Context{
		TraceID: strings.ToLower(parts[1]),
		SpanID:  strings.ToLower(parts[2]),
		Sampled: isHex(strings.ToLower(parts[3]), 2) && parts[3][1]&1 == 1,
	}
	if !tc.Valid() {
		return Context{}, false
	}
	return tc, true
}

// XRayTraceHeader は X-Ray 形式("Root=1-<8hex>-<24hex>;Parent=<span_id>;Sampled=1")を返す。
func (c Context) XRayTraceHeader() string {
	if !c.Valid() {
		return ""
	}
	sampled := "0"
	if c.Sampled {
		sampled = "1"
	}
	return "Root=1-" + c.TraceID[:8] + "-" + c.TraceID[8:] + ";Parent=" + c.SpanID + ";Sampled=" + sampled
}

// ParseXRayTraceHeader は X-Ray 形式を解析する。Root が不正なら ok=false。
// Parent がない場合(Lambda の入口など)は SpanID を新しく採番する。
func ParseXRayTraceHeader(s string) (Context, bool) {
	var tc Context
	for _, kv := range strings.Split(s, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(kv), "=")
		if !found {
			continue
		}
		switch key {
		case "Root":
			// 1-<8hex>-<24hex>
			p := strings.Split(value, "-")
			if len(p) != 3 || p[0] != "1" {
				return Context{}, false
			}
			tc.TraceID = strings.ToLower(p[1] + p[2])
		case "Parent":
			tc.SpanID = strings.ToLower(value)
		case "Sampled":
			tc.Sampled = value == "1"
		}
	}
	if !isHex(tc.TraceID, traceIDLen) {
		return Context{}, false
	}
	if !isHex(tc.SpanID, spanIDLen) {
		tc.SpanID = randomHex(spanIDLen)
	}
	if !tc.Valid() {
		return Context{}, false
	}
	return tc, true
}
