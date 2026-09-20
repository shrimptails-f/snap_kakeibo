package logger

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"snap_kakeibo/backend/internal/library/trace"
)

// Event の値のうち logger 自身が出すもの。
const (
	EventSpanStarted  = "span_started"
	EventSpanFinished = "span_finished"
)

// Status の値のうち Span.End が出すもの。
const (
	StatusOK    = "ok"
	StatusError = "error"
)

// Span は名前付きの処理区間。区間内のログには span_name と新しい span_id が付き、
// End で所要時間と結果をまとめた 1 行(span_finished)を出す。
//
// 本物のトレーサーがなくても Logs Insights で区間ごとの所要時間・成功率を集計できる:
//
//	filter event = "span_finished" | stats pct(duration_ms, 95), count() by span_name, status
//
// 区間の途中で分かった結果(件数やトークン数など)は AddFields で積んでおくと span_finished に載る。
// 「1 処理 1 行」の wide event として使うことを想定している。
type Span struct {
	log     Interface
	ctx     context.Context
	name    string
	started time.Time

	mu     sync.Mutex
	fields []Field
	ended  bool
}

// StartSpan は新しい処理区間を開始する。返る ctx を区間内の処理に渡すこと。
// fields は区間内の全ログと span_finished に付く。
func StartSpan(ctx context.Context, log Interface, name string, fields ...Field) (context.Context, *Span) {
	if log == nil {
		log = NewNop()
	}
	ctx, _ = trace.Start(ctx)
	ctx = ContextWith(ctx, append([]Field{SpanName(name)}, fields...)...)

	s := &Span{log: log, ctx: ctx, name: name, started: time.Now()}
	s.emit(slog.LevelInfo, name+" started", []Field{Event(EventSpanStarted)})
	return ctx, s
}

// AddFields は span_finished に載せるフィールドを積む。同じキーは後勝ち。
func (s *Span) AddFields(fields ...Field) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fields = mergeFields(s.fields, fields)
}

// End は区間を終了し span_finished を出す。err があれば error レベルで status=error、なければ info で status=ok。
// 業務上の失敗(err にはしないが結果としては失敗)は AddFields で別のキーに積む。2 回目以降の呼び出しは何もしない。
func (s *Span) End(err error, fields ...Field) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	out := mergeFields(append([]Field(nil), s.fields...), fields)
	s.mu.Unlock()

	out = mergeFields(out, []Field{Event(EventSpanFinished), DurationMS(time.Since(s.started))})
	if err != nil {
		out = mergeFields(out, []Field{Status(StatusError), Err(err)})
		s.emit(slog.LevelError, s.name+" failed", out)
		return
	}
	out = mergeFields(out, []Field{Status(StatusOK)})
	s.emit(slog.LevelInfo, s.name+" finished", out)
}

// emit は StartSpan / End の呼び出し元を caller にしてログを出す。
// *Logger 以外の実装(テストのモックなど)は caller を持たないので通常のメソッドで出す。
func (s *Span) emit(level slog.Level, message string, fields []Field) {
	if l, ok := s.log.(*Logger); ok {
		// logDepth から見て emit, StartSpan / End の 2 段を飛ばすと呼び出し元になる
		l.logDepth(s.ctx, level, message, 2, fields)
		return
	}
	switch level {
	case slog.LevelError:
		s.log.Error(s.ctx, message, fields...)
	case slog.LevelWarn:
		s.log.Warn(s.ctx, message, fields...)
	case slog.LevelDebug:
		s.log.Debug(s.ctx, message, fields...)
	default:
		s.log.Info(s.ctx, message, fields...)
	}
}
