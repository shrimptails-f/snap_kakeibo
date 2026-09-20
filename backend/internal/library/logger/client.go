package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"snap_kakeibo/backend/internal/library/trace"
)

// Options は New に渡す設定。ゼロ値でも動作する。
type Options struct {
	Level       string    // Level は debug / info / warn / error。空や未知の値は info。
	Service     string    // Service はサービス名(Lambda 関数名など)。空なら "unknown"。
	Environment string    // Environment は環境名(dev / prod など)。空なら "unknown"。
	Writer      io.Writer // Writer は出力先。nil なら os.Stdout。
}

// Logger は slog.Handler をラップして Interface を満たす。
type Logger struct {
	handler slog.Handler
}

var _ Interface = (*Logger)(nil)

// New は JSON 形式で出力するロガーを生成する。
func New(opts Options) *Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}

	base := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       parseLevel(opts.Level),
		AddSource:   true,
		ReplaceAttr: replaceAttr,
	})

	h := contextHandler{inner: base}.WithAttrs([]slog.Attr{
		Service(opts.Service),
		Environment(opts.Environment),
	})

	return &Logger{handler: h}
}

// NewNop は何も出力しないロガーを返す。テストや未注入時のフォールバック用。
func NewNop() *Logger {
	return &Logger{handler: contextHandler{inner: slog.DiscardHandler}}
}

// Debug は debug レベルのログを出力する。
func (l *Logger) Debug(ctx context.Context, message string, fields ...Field) {
	l.logDepth(ctx, slog.LevelDebug, message, 1, fields)
}

// Info は info レベルのログを出力する。
func (l *Logger) Info(ctx context.Context, message string, fields ...Field) {
	l.logDepth(ctx, slog.LevelInfo, message, 1, fields)
}

// Warn は warn レベルのログを出力する。
func (l *Logger) Warn(ctx context.Context, message string, fields ...Field) {
	l.logDepth(ctx, slog.LevelWarn, message, 1, fields)
}

// Error は error レベルのログを出力する。
func (l *Logger) Error(ctx context.Context, message string, fields ...Field) {
	l.logDepth(ctx, slog.LevelError, message, 1, fields)
}

// With は指定フィールドを常に含める子ロガーを返す。
func (l *Logger) With(fields ...Field) Interface {
	if len(fields) == 0 {
		return l
	}
	return &Logger{handler: l.handler.WithAttrs(fields)}
}

// logDepth はログを 1 行出す。depth は呼び出し元の位置(caller)を取るために飛ばすフレーム数で、
// Info などから直接呼ぶなら 1、その 1 段外側を呼び出し元にしたいなら 2。
func (l *Logger) logDepth(ctx context.Context, level slog.Level, message string, depth int, fields []Field) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !l.handler.Enabled(ctx, level) {
		return
	}

	// slog.Logger を経由せず Record を組み立てるので、呼び出し元の位置を自分で取る。
	// 0: Callers, 1: logDepth, 2..: depth 分のラッパー, その次が呼び出し元
	var pcs [1]uintptr
	runtime.Callers(2+depth, pcs[:])

	r := slog.NewRecord(time.Now(), level, message, pcs[0])
	r.AddAttrs(fields...)
	_ = l.handler.Handle(ctx, r)
}

// contextHandler は With で積まれた固定フィールド・ctx のフィールド・trace 情報を Record にまとめてから内側のハンドラへ渡す。
//
// 同じキーは 1 行に 1 回しか出さない。優先順位は束縛が遅いものほど強い:
//
//	With(固定) < ContextWith(リクエスト単位) < 呼び出し時の fields < trace(ctx から自動)
//
// trace_id / span_id は手で渡された値より ctx の値を採る。ログとトレースの相関が壊れないようにするため。
// 固定フィールドを内側の WithAttrs に渡さず自前で持つのは、この重複排除のため。
type contextHandler struct {
	inner slog.Handler
	attrs []slog.Attr
}

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	merged := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs()+8)
	merged = mergeFields(merged, h.attrs)
	merged = mergeFields(merged, FieldsFromContext(ctx))
	r.Attrs(func(a slog.Attr) bool {
		merged = mergeFields(merged, []slog.Attr{a})
		return true
	})
	if tc, ok := trace.FromContext(ctx); ok {
		merged = mergeFields(merged, []slog.Attr{TraceID(tc.TraceID), SpanID(tc.SpanID)})
		if tc.ParentSpanID != "" {
			merged = mergeFields(merged, []slog.Attr{ParentSpanID(tc.ParentSpanID)})
		}
	}

	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(merged...)
	return h.inner.Handle(ctx, out)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{inner: h.inner, attrs: mergeFields(append([]slog.Attr(nil), h.attrs...), attrs)}
}

// WithGroup は Interface からは到達しない。slog.Handler の契約を満たすためだけに内側へ委譲する。
func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{inner: h.inner.WithGroup(name), attrs: h.attrs}
}

// mergeFields は src を dst に追加する。同じキーが dst にあれば src の値で置き換える。
// 空の Attr(Err(nil) など)は捨て、キーなしのグループ(Err が返すもの)は展開して個別に扱う。
func mergeFields(dst []slog.Attr, src []slog.Attr) []slog.Attr {
	for _, f := range src {
		if f.Equal(slog.Attr{}) {
			continue
		}
		if f.Key == "" && f.Value.Kind() == slog.KindGroup {
			dst = mergeFields(dst, f.Value.Group())
			continue
		}
		replaced := false
		for i := range dst {
			if dst[i].Key == f.Key {
				dst[i] = f
				replaced = true
				break
			}
		}
		if !replaced {
			dst = append(dst, f)
		}
	}
	return dst
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// replaceAttr は出力キーを共通スキーマに揃え、機微な値を伏せる。
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 {
		switch a.Key {
		case slog.MessageKey:
			a.Key = "message"
		case slog.SourceKey:
			if src, ok := a.Value.Any().(*slog.Source); ok {
				return slog.String("caller", shortCaller(src))
			}
		}
	}

	if isSensitiveKey(a.Key) {
		a.Value = slog.StringValue(redacted)
	}
	return a
}

// shortCaller は zap の ShortCaller 相当に "dir/file.go:line" へ縮める。
func shortCaller(src *slog.Source) string {
	if src == nil || src.File == "" {
		return ""
	}
	dir, file := filepath.Split(src.File)
	short := filepath.Join(filepath.Base(dir), file)
	return short + ":" + strconv.Itoa(src.Line)
}
