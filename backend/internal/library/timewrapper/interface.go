// Package timewrapper は時刻の取得を差し替え可能にする薄いラッパーを提供する。
//
// 業務ロジックは time.Now() を直接呼ばず Interface を受け取る。テストでは Fixed で時刻を固定できる。
//
//	func NewUsecase(clock timewrapper.Interface) *Usecase
//
//	// 本番
//	uc := NewUsecase(timewrapper.NewClock())
//	// テスト
//	uc := NewUsecase(timewrapper.NewFixed(time.Date(2026, 1, 2, 3, 4, 5, 0, timewrapper.JST())))
package timewrapper

import "time"

// Interface は時刻に関する操作の契約。
type Interface interface {
	// Now は現在時刻を返す。
	Now() time.Time
	// After は d 経過後に現在時刻を送るチャネルを返す。
	After(d time.Duration) <-chan time.Time
}
