// Package retry は一時的なエラーに対する有限回のリトライを提供する。
//
// 待ち時間は Policy.Backoff で明示し、試行回数は len(Backoff)+1 回。無限リトライはしない。
// Lambda の外側には SQS + DLQ(MaxReceiveCount)のリトライがあるので、Lambda 内は短く有限で良い。
//
//	r := retry.New(log, timewrapper.NewClock())
//	policy := retry.Policy{
//		Backoff:        retry.Exponential(time.Second, 16*time.Second, 6), // 1,2,4,8,16,16 秒 → 最大 7 回
//		AttemptTimeout: 30 * time.Second,                                  // 1 回あたりの上限
//		ShouldRetry:    func(err error) bool { return errors.Is(err, ErrTemporary) },
//	}
//	err := r.Do(ctx, "openai_request", policy, func(ctx context.Context) error {
//		return callAPI(ctx) // 渡された ctx を必ず使う。使わないと AttemptTimeout は効かない
//	})
//
// タイムアウトについて。Go のタイムアウトは ctx を通じた協調的なもので、op が ctx を無視すれば止められない。
// このパッケージは op を別 goroutine で走らせて見捨てることはしない(見捨てると処理は裏で続き、
// 副作用が完了した後にリトライして二重実行になる)。op が ctx を尊重することが前提。
// また、タイムアウト後のリトライは「相手側では成功していた」可能性があるので、
// 冪等でない操作(課金を伴う API 呼び出しや無条件の書き込み)に使うときは呼び出し側で冪等性を担保する。
//
// ログには各リトライ(retry_attempt)、諦めた時(retry_exhausted)、リトライ後に成功した時(retry_succeeded)を出す。
package retry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"snap_kakeibo/backend/internal/library/logger"
	"snap_kakeibo/backend/internal/library/timewrapper"
)

// Event の値のうち retry が出すもの。
const (
	EventRetryAttempt   = "retry_attempt"
	EventRetryExhausted = "retry_exhausted"
	EventRetrySucceeded = "retry_succeeded"
)

// Policy はリトライの方針。ゼロ値は 1 回だけ試す(リトライなし)。
type Policy struct {
	// Backoff は各リトライ前の待ち時間。len(Backoff)+1 回試す。空なら 1 回だけ。
	Backoff []time.Duration
	// AttemptTimeout は 1 回の試行に与える上限。0 なら親 ctx の期限だけに従う。
	AttemptTimeout time.Duration
	// ShouldRetry はエラーをリトライするかを決める。nil なら全エラーをリトライする。
	// 親 ctx が切れた場合は ShouldRetry に関係なく止まる。
	ShouldRetry func(error) bool
}

// MaxAttempts は最大試行回数。
func (p Policy) MaxAttempts() int {
	return len(p.Backoff) + 1
}

// Exponential は initial から 2 倍ずつ増え max で頭打ちになる待ち時間を n 個返す。
// Exponential(time.Second, 16*time.Second, 6) → 1s, 2s, 4s, 8s, 16s, 16s
func Exponential(initial, max time.Duration, n int) []time.Duration {
	if n <= 0 {
		return nil
	}
	out := make([]time.Duration, 0, n)
	delay := initial
	for i := 0; i < n; i++ {
		if delay > max {
			delay = max
		}
		out = append(out, delay)
		delay *= 2
	}
	return out
}

// Retrier はリトライを実行する。ログと時計を差し替えられる。
type Retrier struct {
	log   logger.Interface
	clock timewrapper.Interface
}

// New は Retrier を生成する。log が nil なら何も出力しない。clock が nil なら実時刻。
func New(log logger.Interface, clock timewrapper.Interface) *Retrier {
	if log == nil {
		log = logger.NewNop()
	}
	if clock == nil {
		clock = timewrapper.NewClock()
	}
	return &Retrier{log: log, clock: clock}
}

// Do は op を policy に従って繰り返す。name はログの retry_name に載せる識別子。
//
// 返り値:
//   - 成功: nil
//   - ShouldRetry が false を返した: そのエラー
//   - 回数を使い切った: 最後のエラー
//   - 親 ctx が切れた: ctx.Err() と最後のエラーを errors.Join したもの(どちらでも errors.Is できる)
func (r *Retrier) Do(ctx context.Context, name string, policy Policy, op func(ctx context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if op == nil {
		return fmt.Errorf("retry: op is nil")
	}
	maxAttempts := policy.MaxAttempts()
	ctx = logger.ContextWith(ctx, logger.String("retry_name", name), logger.Int("max_attempts", maxAttempts))

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			delay := policy.Backoff[attempt-2]
			r.log.Info(ctx, "retrying "+name,
				logger.Event(EventRetryAttempt),
				logger.Int("attempt", attempt),
				logger.Int64("delay_ms", delay.Milliseconds()),
				logger.Err(lastErr),
			)
			if err := r.wait(ctx, delay); err != nil {
				return errors.Join(err, lastErr)
			}
		}

		err := r.attempt(ctx, policy.AttemptTimeout, op)
		if err == nil {
			if attempt > 1 {
				r.log.Info(ctx, name+" succeeded after retry", logger.Event(EventRetrySucceeded), logger.Int("attempt", attempt))
			}
			return nil
		}
		lastErr = err

		// 親 ctx が切れていれば、このエラーはその結果なので続けない
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(ctxErr, lastErr)
		}
		if policy.ShouldRetry != nil && !policy.ShouldRetry(err) {
			return err
		}
	}

	r.log.Warn(ctx, name+" gave up", logger.Event(EventRetryExhausted), logger.Int("attempt", maxAttempts), logger.Err(lastErr))
	return lastErr
}

// attempt は 1 回の試行。timeout があれば子 ctx を切って op に渡す。
func (r *Retrier) attempt(ctx context.Context, timeout time.Duration, op func(ctx context.Context) error) error {
	if timeout <= 0 {
		return op(ctx)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return op(attemptCtx)
}

// wait は delay 待つ。親 ctx が先に切れたらその ctx.Err() を返す。
func (r *Retrier) wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.clock.After(delay):
		return nil
	}
}
