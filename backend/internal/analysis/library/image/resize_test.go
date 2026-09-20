package image

import (
	"bytes"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestResizeJPEGShrinksLongEdge(t *testing.T) {
	t.Parallel()
	out, err := Resizer{MaxEdge: 64}.ResizeJPEG(encodePNG(t, 200, 100))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not JPEG: %v", err)
	}
	if cfg.Width != 64 || cfg.Height != 32 {
		t.Errorf("size = %dx%d, want 64x32", cfg.Width, cfg.Height)
	}
}

func TestResizeJPEGKeepsSmallImage(t *testing.T) {
	t.Parallel()
	out, err := Resizer{}.ResizeJPEG(encodePNG(t, 30, 40))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 30 || cfg.Height != 40 {
		t.Errorf("size = %dx%d err = %v, want 30x40", cfg.Width, cfg.Height, err)
	}
}

func TestResizeJPEGRejectsUndecodableData(t *testing.T) {
	t.Parallel()
	if _, err := (Resizer{MaxEdge: 64}).ResizeJPEG([]byte("not an image")); err == nil {
		t.Fatal("ResizeJPEG() accepted invalid data")
	}
}
