// Package logger は log/slog を薄くラップした構造化ロガーを提供する。
//
// JSON 形式で標準出力へ書き出し、context.Context に積んだフィールドと
// library/trace のトレース情報(trace_id / span_id)をログ出力時に自動で付与する。
// main.go で request_id などを ctx に積んでおけば、その ctx を受け取った下層の処理は
// 何も意識せずに同じフィールド付きでログを出せる。
//
//	log := logger.New(logger.Options{Level: "info", Service: "analyze-receipt", Environment: "dev"})
//
//	func handler(ctx context.Context, event events.SQSEvent) error {
//		ctx, _ = trace.Start(ctx)
//		ctx = logger.ContextWith(ctx, logger.RequestID(reqID))
//		return process(ctx, log)
//	}
//
//	func process(ctx context.Context, log logger.Interface) error {
//		// request_id / trace_id / span_id が自動で付く
//		log.Info(ctx, "processing started", logger.Int("count", 3))
//	}
//
// 処理の単位ごとに StartSpan を使うと、区間内のログに span_name と新しい span_id が付き、
// End で所要時間と結果をまとめた 1 行(event=span_finished)が出る。結果の集計はこの行を使う。
//
//	ctx, span := logger.StartSpan(ctx, log, "analysis")
//	err := doWork(ctx)
//	span.AddFields(logger.Int("detail_count", n)) // 途中で分かった結果を積む
//	span.End(err)
//
// 同じキーは 1 行に 1 回しか出ない。重なった場合は束縛が遅いものが勝つ:
// With(固定) < ContextWith(リクエスト単位) < 呼び出し時の fields。
// trace_id / span_id だけは例外で、手で渡した値より ctx の trace が優先される。
//
// ログ 1 行に載る相関キーは 3 層ある。追いかけるときはこの順で絞る。
//
//   - analysis_request_id / expense_id: 業務のライフサイクル全体(S3 直 PUT を挟んでも切れない)
//   - trace_id: 1 リクエストから派生する処理の連鎖(SQS 経由を含む)
//   - request_id: Lambda 1 回の実行
package logger

import (
	"context"
	"log/slog"
)

// Field はログに付与する key/value を表す。slog.Attr の別名で、呼び出し側が slog に直接依存しないようにする。
type Field = slog.Attr

// Interface はアプリ各層で利用するロガーの契約。
// 各メソッドは ctx を受け取り、ContextWith で積まれたフィールドを出力に含める。
type Interface interface {
	Debug(ctx context.Context, message string, fields ...Field)
	Info(ctx context.Context, message string, fields ...Field)
	Warn(ctx context.Context, message string, fields ...Field)
	Error(ctx context.Context, message string, fields ...Field)
	// With は指定フィールドを常に含める子ロガーを返す。
	With(fields ...Field) Interface
}
