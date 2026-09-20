package timewrapper

import "time"

// Clock は標準の time パッケージをそのまま使う実装。
type Clock struct{}

var _ Interface = (*Clock)(nil)

// NewClock は実時刻を返す Clock を生成する。
func NewClock() *Clock {
	return &Clock{}
}

// Now は現在時刻を返す。
func (c *Clock) Now() time.Time {
	return time.Now()
}

// After は d 経過後に現在時刻を送るチャネルを返す。
func (c *Clock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// Fixed は固定した時刻を返すテスト用の実装。Set で進められる。
type Fixed struct {
	now time.Time
}

var _ Interface = (*Fixed)(nil)

// NewFixed は now を返し続ける Fixed を生成する。
func NewFixed(now time.Time) *Fixed {
	return &Fixed{now: now}
}

// Now は固定した時刻を返す。
func (f *Fixed) Now() time.Time {
	return f.now
}

// After は実際には待たず、固定した時刻を d 進めた値を即座に送るチャネルを返す。
func (f *Fixed) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- f.now.Add(d)
	return ch
}

// Set は返す時刻を差し替える。
func (f *Fixed) Set(now time.Time) {
	f.now = now
}

// Advance は返す時刻を d だけ進める。
func (f *Fixed) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}
