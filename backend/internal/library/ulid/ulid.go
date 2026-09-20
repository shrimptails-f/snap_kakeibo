// Package ulid は時刻順に並ぶ一意な識別子(ULID)を採番する。
//
// 時刻は timewrapper.Interface から取るのでテストで固定できるが、乱数部は毎回異なるので同じ時刻でも衝突しない。
//
//	ids := ulid.New(timewrapper.NewClock())
//	id, err := ids.NewID() // "01JUSER0000000000000000000" のような 26 文字
package ulid

import (
	"crypto/rand"
	"fmt"

	"snap_kakeibo/backend/internal/library/timewrapper"

	oklog "github.com/oklog/ulid/v2"
)

// Generator は ULID を採番する。
type Generator struct {
	clock timewrapper.Interface
}

// New は clock の時刻で採番する Generator を生成する。clock が nil なら実時刻。
func New(clock timewrapper.Interface) *Generator {
	if clock == nil {
		clock = timewrapper.NewClock()
	}
	return &Generator{clock: clock}
}

// NewID は新しい ULID を返す。
func (g *Generator) NewID() (string, error) {
	id, err := oklog.New(oklog.Timestamp(g.clock.Now()), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("ulid: generate: %w", err)
	}
	return id.String(), nil
}
