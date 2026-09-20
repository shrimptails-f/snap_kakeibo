// Package image はレシート画像を OpenAI に渡せる大きさの JPEG に変換する。
package image

import (
	"bytes"
	"fmt"
	stdimage "image"
	"image/jpeg"
	_ "image/png" // PNG のアップロードもデコードできるようにする

	"golang.org/x/image/draw"
)

// DefaultMaxEdge は MaxEdge が 0 以下のときの長辺の上限(px)。
const DefaultMaxEdge = 2048

// jpegQuality は出力 JPEG の品質。
const jpegQuality = 90

// Resizer は長辺が MaxEdge に収まるよう縮小して JPEG に再エンコードする。application.ImageResizer を満たす。
type Resizer struct {
	MaxEdge int
}

// ResizeJPEG は data(JPEG / PNG)をデコードし、長辺が MaxEdge を超えていれば縮小して JPEG で返す。
// デコードできなければ error。
func (r Resizer) ResizeJPEG(data []byte) ([]byte, error) {
	return ResizeJPEG(data, r.MaxEdge)
}

// ResizeJPEG は Resizer.ResizeJPEG の関数版。maxEdge が 0 以下なら DefaultMaxEdge。
func ResizeJPEG(data []byte, maxEdge int) ([]byte, error) {
	img, _, err := stdimage.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	dst := img
	if w > maxEdge || h > maxEdge {
		var nw, nh int
		if w >= h {
			nw = maxEdge
			nh = max(1, h*maxEdge/w)
		} else {
			nh = maxEdge
			nw = max(1, w*maxEdge/h)
		}
		resized := stdimage.NewRGBA(stdimage.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, b, draw.Over, nil)
		dst = resized
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return out.Bytes(), nil
}
