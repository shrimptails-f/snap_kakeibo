// Package lambdawrap は Lambda ハンドラに共通の可観測性処理を被せる薄いラッパー。
//
// Handle が行うこと:
//   - request_id(AwsRequestID)と trace(_X_AMZN_TRACE_ID か新規 root)を ctx に積む
//   - 開始・終了ログ(duration_ms / cold_start)を出す
//   - panic を回収して stack_trace 付きでログを出し、error として返す
//
// SQS レコードごとの ctx 構築(送信側の trace の引き継ぎ)は library/sqs の RecordContext が担う。
//
// 使い方:
//
//	func main() {
//		lambda.Start(lambdawrap.Handle(log, handler))      // (ctx, in) -> (out, error) 形式(API Gateway など)
//		lambda.Start(lambdawrap.HandleEvent(log, handler)) // (ctx, in) -> error 形式(SQS / S3 など)
//	}
package lambdawrap

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/trace"

	"github.com/aws/aws-lambda-go/lambdacontext"
)

// xrayTraceIDEnv は Lambda ランタイムが呼び出しごとに設定する X-Ray トレースヘッダ。
const xrayTraceIDEnv = "_X_AMZN_TRACE_ID"

// Event の値のうち lambdawrap が出すもの。Logs Insights の filter に使う。
const (
	EventInvocationStarted  = "invocation_started"
	EventInvocationFinished = "invocation_finished"
	EventPanicRecovered     = "panic_recovered"
)

var coldStart atomic.Bool

func init() {
	coldStart.Store(true)
}

// Handler は aws-lambda-go が受け付ける (ctx, in) -> (out, err) 形式のハンドラ。
type Handler[In, Out any] func(ctx context.Context, in In) (Out, error)

// Handle は Lambda ハンドラに共通処理を被せたハンドラを返す。
func Handle[In, Out any](log logger.Interface, fn Handler[In, Out]) Handler[In, Out] {
	if log == nil {
		log = logger.NewNop()
	}

	return func(ctx context.Context, in In) (out Out, err error) {
		ctx = InvocationContext(ctx)
		cold := coldStart.Swap(false)
		started := time.Now()

		log.Info(ctx, "invocation started", logger.Event(EventInvocationStarted), logger.Bool("cold_start", cold))

		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
				log.Error(ctx, "panic recovered", logger.Event(EventPanicRecovered), logger.Recovered(r), logger.StackTrace())
			}

			fields := []logger.Field{
				logger.Event(EventInvocationFinished),
				logger.DurationMS(time.Since(started)),
			}
			if err != nil {
				log.Error(ctx, "invocation failed", append(fields, logger.Err(err))...)
				return
			}
			log.Info(ctx, "invocation finished", fields...)
		}()

		return fn(ctx, in)
	}
}

// EventHandler は戻り値を持たない (ctx, in) -> err 形式のハンドラ。SQS や S3 のイベント用。
type EventHandler[In any] func(ctx context.Context, in In) error

// HandleEvent は戻り値を持たないハンドラ向けの Handle。
func HandleEvent[In any](log logger.Interface, fn EventHandler[In]) EventHandler[In] {
	h := Handle(log, func(ctx context.Context, in In) (struct{}, error) {
		return struct{}{}, fn(ctx, in)
	})
	return func(ctx context.Context, in In) error {
		_, err := h(ctx, in)
		return err
	}
}

// InvocationContext は Lambda 1 回の実行に共通するフィールドを ctx に積む。
// Handle を使っていれば呼ぶ必要はない。
func InvocationContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if tc, ok := trace.ParseXRayTraceHeader(os.Getenv(xrayTraceIDEnv)); ok {
		// X-Ray が有効なら CloudWatch のログと X-Ray のトレースが同じ trace_id で紐づく。
		// ヘッダの Parent は上流(Lambda サービス)のセグメントなので、この実行は子 span にする
		ctx, _ = trace.StartFrom(ctx, tc)
	} else {
		ctx, _ = trace.Start(ctx)
	}

	if lc, ok := lambdacontext.FromContext(ctx); ok && lc.AwsRequestID != "" {
		ctx = logger.ContextWith(ctx, logger.RequestID(lc.AwsRequestID))
	}

	return ctx
}
