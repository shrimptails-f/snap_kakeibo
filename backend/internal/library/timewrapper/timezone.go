package timewrapper

import "time"

var jst = time.FixedZone("JST", 9*60*60)

// JST はアプリ共通で使う日本標準時の Location を返す。
// time.LoadLocation と違い tzdata に依存しないので、Lambda のコンテナでも確実に使える。
func JST() *time.Location {
	return jst
}

// InJST は時刻を JST に変換する。ゼロ値はそのまま返す。
func InJST(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.In(jst)
}
