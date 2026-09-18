package analyze

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
)

func ResizeJPEG(data []byte, maxEdge int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	if maxEdge <= 0 {
		maxEdge = 2048
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
		resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, b, draw.Over, nil)
		dst = resized
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return out.Bytes(), nil
}
