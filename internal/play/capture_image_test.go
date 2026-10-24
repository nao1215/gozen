package play

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

func TestCaptureGIFBounds(t *testing.T) {
	for _, cfg := range []CaptureGIF{
		{FPS: -1},
		{FPS: 31},
		{Duration: -1},
		{Duration: 11 * time.Second},
		{Width: -1},
		{Width: 1281},
		{FPS: 1, Duration: time.Second},
		{FPS: 30, Duration: 10 * time.Second, Width: 1280},
		{Width: int(^uint(0) >> 1)},
	} {
		if _, err := newCaptureFrames(cfg, 1280, 720); err == nil {
			t.Errorf("accepted %+v", cfg)
		}
	}
	for _, size := range [][2]int{{0, 720}, {1280, 0}, {8193, 1}, {1, 8193}, {1280, 8192}} {
		if _, err := newCaptureFrames(CaptureGIF{}, size[0], size[1]); err == nil {
			t.Errorf("accepted dimensions %v", size)
		}
	}
}

func TestCaptureGIFDecodedFramesAndTiming(t *testing.T) {
	for _, fps := range []int{7, 10, 30} {
		f, err := newCaptureFrames(CaptureGIF{FPS: fps, Duration: 1234 * time.Millisecond, Width: 8}, 16, 12)
		if err != nil {
			t.Fatal(err)
		}
		for i := range f.count {
			if got := f.due(); got != i*60/fps {
				t.Fatalf("sample %d at tick %d", i, got)
			}
			f.add(sysSolid(16, 12, color.NRGBA{R: uint8(i * 20), B: uint8(255 - i*20), A: 255}))
		}
		var out bytes.Buffer
		if err := gif.EncodeAll(&out, &f.GIF); err != nil {
			t.Fatal(err)
		}
		decoded, err := gif.DecodeAll(&out)
		if err != nil {
			t.Fatal(err)
		}
		if len(decoded.Image) != f.count || decoded.LoopCount != 0 || decoded.Image[0].Bounds() != image.Rect(0, 0, 8, 6) {
			t.Fatalf("wrong animation: %dx%d, %d frames", decoded.Config.Width, decoded.Config.Height, len(decoded.Image))
		}
		if bytes.Equal(decoded.Image[0].Pix, decoded.Image[len(decoded.Image)-1].Pix) {
			t.Fatal("animation lost motion")
		}
		total := 0
		for _, d := range decoded.Delay {
			if d <= 0 {
				t.Fatalf("nonpositive delay: %d", d)
			}
			total += d
		}
		if total != 123 {
			t.Fatalf("duration drift at %d FPS: %d centiseconds", fps, total)
		}
	}
}
